# RPC 通知的单缓冲工作区

2026-10-18。上一切片已本地提交为 gosproto `b626637` / GSR `b2825cf`，提交后两库干净、未推送。本轮仅融合有 Request、无 Response 的通知，新增代码和记录未提交；正式依赖仍为 v0.4.0，Decode 冻结。

## 修改与契约

RequestEncode 完成协议查找和 Request 类型检查后，仅对无响应协议准备私有 type-only header，复用上一轮的 encodeWithLimits 前缀入口。body 完整预检后直接写到最终独占 Pack 工作区尾部，再从前部压缩，省掉中间 body 分配及复制。生产代码仅 rpc.go 增加八行，通用编码器、ResponseEncode、Decode/Dispatch、Pack/Unpack 均未改；没有新 API、pool、cache、unsafe、generator 或依赖。

header 至多12 B，位于调用内22 B栈缓冲；固定 int32/default 预算保证准备成功。header 和 body 预算仍各自计算，body 8 MiB 边界不因 header 被拒绝。总尺寸、原 Pack 容量、FF 增长间隔与输出独占沿用已验证的共享写出；输入与旧输出不别名。

通知仍忽略传入 session（包括零/负数），不登记 pending，不获取 session 锁。typed nil 仍返回 ErrNil 和 nil 输出，body 预算失败不暴露部分帧。没有 Request 的协议继续走原 packRpcFrame，保持忽略 req 的行为。有响应请求继续先独立 Encode body，再校验 session、加锁和登记；重复 session 返回已编码 raw body 的既有行为保持。不能将通知分支直接推广到这条路径。

## 测试增量

扩展既有 TestRpcFrameAssembly：四种忽略 session 边界、精确 wire/cap、通知不登记 pending、typed nil 错误；扩展既有 body 预算用例，保护通知8 MiB恰到边界与超限。上述语义在旧实现先通过。分配回归将通知与同构建 EncodePacked 比较：旧 notification2 / response1 / packedBody1 按预期失败，候选普通及 race 构建通过。已有1008组RPC场景/2016输出与39官方C/Lua头向量继续运行；未重复搭建底层 Pack 夹具。

首次目录创建误用库 cwd，测试完成但报告写入失败；只清理自己的空目录并修正路径，随后捕获基线结果，记录在 checks.json。没有因此修改生产行为。

## 固定两轮测量

沿用同一19项真实 RPC API 探针：9项通知是目标，10项请求编码加 CancelSession/响应是非目标控制。self/clone各一次，固定候选比较两次，不调参或重试。Go1.24.13 darwin/arm64、Apple M2、GOMAXPROCS=1、GC开启，六轮×四组ABBA/BAAB、10 ms目标块。

**7296块、1824四元组、456配对轮次**独立复算到1e-12一致。负数为耗时下降，表内是六轮配对比值中位数，不是置信区间。

| 通知 | 第一轮 | 第二轮 | B/op 前→后 | allocs/op 前→后 |
| --- | ---: | ---: | ---: | ---: |
| addressbook/notification | -7.20% | -7.00% | 288→144 | 2→1 |
| game_small/notification | -8.76% | -7.40% | 704→352 | 2→1 |
| game_large/notification | -6.53% | -7.34% | 7424→3968 | 3→2 |
| int32_edges/notification | -13.46% | -11.71% | 8192→4096 | 2→1 |
| echo_ascii4k/notification | -43.38% | -44.79% | 9728→4864 | 2→1 |
| ascii32/notification | -11.91% | -16.91% | 112→64 | 2→1 |
| utf8_1k/notification | -35.90% | -34.59% | 2048→1024 | 2→1 |
| ascii4k/notification | -42.73% | -44.03% | 9728→4864 | 2→1 |
| zeros4k/notification | -25.53% | -27.19% | 9728→4864 | 2→1 |

9项通知两轮全部改善且各少一次分配。大型消息仍有一次布局记录分配，不能宣称所有通知都只有一次总分配；这些数据不含网络、Dispatch、Decode或业务Handler。

| 非目标路径 | 第一轮 | 第二轮 | 两轮配对耗时差 ns |
| --- | ---: | ---: | ---: |
| addressbook/request_cancel | -0.21% | +0.38% | -1.5 / +2.7 |
| addressbook/response | -0.61% | -0.45% | -4.1 / -2.8 |
| game_small/request_cancel | -0.50% | -1.82% | -5.2 / -19.1 |
| game_small/response | -0.80% | -0.28% | -7.6 / -2.6 |
| game_large/request_cancel | -1.63% | -3.36% | -146.9 / -297.9 |
| game_large/response | -1.29% | -2.82% | -111.1 / -230.4 |
| int32_edges/request_cancel | +0.43% | +0.32% | +20.1 / +14.4 |
| int32_edges/response | -1.19% | -0.93% | -50.1 / -38.5 |
| echo_ascii4k/request_cancel | +0.01% | -0.23% | -0.3 / -3.5 |
| echo_ascii4k/response | -0.78% | +1.07% | -6.5 / +8.9 |

10项非目标分配保持不变。整数数组请求两轮约+0.43%/+0.32%（20.1/14.4 ns）代价保留，ASCII响应第二轮+1.07%（8.9 ns），第一轮−0.78%；大型请求/响应也出现−1%～−3%的差值。并非这些方法的算法被优化，不能将其波动归为确定收益。

self **13/19**、clone **7/19** 满足原 controlAccepted 标记；失败项、全部轮次、IQR、GC和分配计数均保留。A/A未全通过限制了微小百分比的解释；不把失败当作无收益，也不把近零差值当作确定回退。按明确少一次分配/复制、9项通知两轮改善、八行局部分支及已披露小额代价保留，不设最低CPU收益线。

## 验收与复现

- 库全测、固定官方C/Lua对照、race/checkptr、vet通过；官方源码及core.so指纹与既定e93fd5a归档一致。
- 未更换探针，3078固定RPC种子/12312帧旧新wire/cap/输入及旧输出独占差分通过。本轮没有重新跑随机fuzz或通用Pack/Codec fuzz；它们的生产源码未变，不能将上一轮次数计作本轮覆盖。
- 临时modfile下GSR三包、TS构建及Go/TS Plain/MAC **7/7、零跳过**通过。没有GSR全仓/race、跨平台运行或新的PB/TS性能排名。
- 四份归档prepare-only重建的源码、harness、工具、夹具、probe、patch和参数逐项相同；最终库全部Go源码与采样candidate一致。正式go.mod仍v0.4.0。

从GSR根目录复现（输出目录必须不存在）：

```sh
python3 -B docs/benchmarks/2026-10-18-gosproto-notification-workspace/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/gosproto-notification-workspace-new
python3 -B docs/benchmarks/2026-10-18-gosproto-notification-workspace/analyze.py \
  tmp/gosproto-notification-workspace-new
```

库须含基线b626637；candidate.patch固定本轮Go改动，驱动从归档构建，不依赖未提交工作树。仅重建可加--prepare-only。提交报告保留命令、原始块、摘要、输入指纹与差分探针，不提交二进制或归档源码目录。

下一步如优化有响应请求，应先处理“完整body在锁外校验、重复session仍返回raw body”这两个约束；不要为了省缓冲扩大锁范围或改API错误返回。Decode继续冻结。
