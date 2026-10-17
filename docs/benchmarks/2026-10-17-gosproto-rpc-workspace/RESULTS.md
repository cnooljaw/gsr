# RPC 编码共享 Pack 工作区

2026-10-17。上一轮已本地提交为 gosproto `56f5c21` / GSR `0f0f779`，提交后两库干净，未推送。本轮从该版本继续，保留 RPC 编码组帧优化到工作树，尚未提交/发布；正式 GSR 依赖仍为 v0.4.0，Decode 阶段冻结。

## 修改与边界

原 RequestEncode/ResponseEncode 先编码 body，再编码 header，Append 两者后 Pack。本轮保留 body 编码及原有 session 处理顺序，只把 header 直接编码到私有 Pack 工作区尾部，复制已编码 body，再从前部压缩，省去 header 字节缓冲与 Append 拼接缓冲。生产代码只改 rpc.go，复用上一提交的 packedBufferSize/packInto。

- body 仍先独立预检/编码，header 也经原 schema、预算及编码执行层；没有手写另一套整数 wire。
- 请求 body 校验发生在 session 校验之前；等待登记、1024 上限、锁范围和取消方式保持原样。没有把 body 编码移进 session 锁。
- 重复 session 错误原本返回已编码的 body，这个既有行为也保持；本轮不借性能修改错误契约。
- header 与 body 保留独立默认预算。body 恰好 8 MiB 时，不能因为叠加 header 而新增编码拒绝；组合帧仍可能超过默认 Unpack 上限，这是原有行为，后续若调整应单独裁决。
- 返回 wire、长度、cap、输入与跨调用输出独占保持不变。没有池、全局临时对象、新 API 或生成器；Dispatch/Decode/Unpack 和 TS 源码未改。

官方 sproto.lua 的 gen_response/host:attach 使用 encode(header)、encode(content) 后 concat+pack。这里沿用其 wire 组合方式，并使用 Go 调用内独占工作区；没有照搬 Lua 的 header_tmp 共享临时表。固定官方源码及核心动态库指纹见 validation.json。

## 测试增量

在已有 RPC 用例上补充：6 种 type × 9 种二进制长度 × 4 种请求/响应 body 组合 × 4 种 session，共 **864 组、1728 个输出**，比较分步 Encode+Pack 的精确 wire/cap、旧输出与输入独占。覆盖负 type、inline 上下边界、大整数 header、2048 B 邻域和 65537 B 长 FF。另检查 body 默认预算恰到/超出上限、错误优先级及重复 session 返回 body。

同构建的 RPC 组帧与手动分步路径比较分配数；新测试在旧实现按预期失败（RPC=6，分步=5），新实现普通及 race/checkptr 构建通过。新 wire/错误/预算测试在旧实现通过，确认它们锁定的是已有契约。全部证据见 acceptance.json。

## 固定真实 RPC 配对

新增探针只位于本报告与隔离 harness，不改正式 Benchmark 工具或旧口径。5 种既有代表消息各测 notification、request_cancel、response；4 种 Echo 只测 notification，共19项。request_cancel 两边均计入 CancelSession，重复使用同一 session 并保持等待表有界；不把每次构造 Rpc 算进编码。响应直接测 ResponseEncode，不包含 Dispatch。计时前核对旧/新 RPC 精确 wire/cap，并继续验证固定 body 夹具。

self、clone 各一次控制，固定候选两次；每次六轮、每轮四个 ABBA/BAAB 四元组、10 ms 目标块。另做一次原19项 body/Pack/手工帧对照。总计 **9120 块、2280 四元组、570 配对轮次**，全部原始块独立复算到 1e-12 相对容差一致；无重试、改参数或挑选轮次。Apple M2、Go 1.24.13、GOMAXPROCS=1、GC 开启。

下表百分比是配对轮次比值的中位数，负数代表耗时减少；B/op 为第一轮采样中位数，第二轮完整值见 joined.json。

| 真实 RPC 路径 | 第一轮 | 第二轮 | B/op 前 → 后 | allocs/op 前 → 后 |
| --- | ---: | ---: | ---: | ---: |
| addressbook/notification | -10.01% | -9.02% | 600 → 308 | 6 → 4 |
| addressbook/request_cancel | -8.72% | -8.14% | 624 → 324 | 6 → 4 |
| addressbook/response | -10.00% | -9.63% | 624 → 324 | 6 → 4 |
| game_small/notification | -12.59% | -11.93% | 1432 → 724 | 6 → 4 |
| game_small/request_cancel | -11.49% | -10.24% | 1440 → 724 | 6 → 4 |
| game_small/response | -11.52% | -12.05% | 1440 → 724 | 6 → 4 |
| game_large/notification | -9.46% | -7.89% | 14232 → 7444 | 7 → 5 |
| game_large/request_cancel | -10.29% | -8.71% | 14240 → 7444 | 7 → 5 |
| game_large/response | -10.24% | -9.36% | 14240 → 7444 | 7 → 5 |
| int32_edges/notification | -17.72% | -20.22% | 16408 → 8212 | 6 → 4 |
| int32_edges/request_cancel | -17.97% | -18.62% | 16416 → 8212 | 6 → 4 |
| int32_edges/response | -17.97% | -19.40% | 16416 → 8212 | 6 → 4 |
| echo_ascii4k/notification | -38.15% | -38.31% | 19224 → 9748.125 | 6 → 4 |
| echo_ascii4k/request_cancel | -37.49% | -37.26% | 19232 → 9748 | 6 → 4 |
| echo_ascii4k/response | -38.37% | -38.25% | 19232 → 9748.125 | 6 → 4 |
| ascii32/notification | -15.74% | -15.01% | 232 → 132 | 6 → 4 |
| utf8_1k/notification | -37.76% | -37.48% | 4120 → 2068 | 6 → 4 |
| ascii4k/notification | -39.09% | -38.38% | 19224.125 → 9748 | 6 → 4 |
| zeros4k/notification | -28.15% | -28.12% | 19224.125 → 9748 | 6 → 4 |

所有19项在两轮都更快、少两次分配。大型消息保留布局记录和 body 缓冲；本轮没有声称合并整个 body 编码。少数 B/op 存在 0.125 B 的窗口尾差，原始三组128次分配记录与 GC 周期保留；不把这种窗口差算成一个新对象。

## 控制、代价与结论

self 原工具总控制通过 13/19；未通过：game_small/notification（中位差 +0.09%，IQR 1.67 pp）、game_large/notification（中位差 +0.72%，IQR 0.80 pp）、int32_edges/notification（中位差 -0.31%，IQR 1.68 pp）、echo_ascii4k/notification（中位差 +0.02%，IQR 1.78 pp）、echo_ascii4k/request_cancel（中位差 -1.09%，IQR 10.37 pp）、ascii4k/notification（中位差 -1.01%，IQR 1.78 pp）。

clone 原工具总控制通过 14/19；未通过：addressbook/request_cancel（中位差 -0.07%，IQR 4.84 pp）、game_small/response（中位差 -0.54%，IQR 8.15 pp）、game_large/notification（中位差 +0.86%，IQR 0.89 pp）、echo_ascii4k/request_cancel（中位差 -0.04%，IQR 9.14 pp）、ascii4k/notification（中位差 +0.74%，IQR 2.68 pp）。

原19项未修改路径的中位差范围为 -1.89%～+0.89%，没有观察到明显回退。它们仍受独立二进制布局、GC 和采样波动影响，不能把小幅正负值归为本轮算法收益。全部逐轮、分配窗口、GC 计数和失败布尔标记原样公开。

保留本轮实现：两次真实 RPC 改善方向一致，约8%～39%的幅度配合确定的两次分配消除，维护成本只增加一个私有组帧函数。控制未全过意味着各项精确百分比仍有不确定性；不将旧固定门槛当作自动否决，也不将结果扩展为网络吞吐、延迟、浏览器或相对 Protobuf 的新结论。

本轮只减少 header/拼接缓冲，body 编码结果仍需一次复制。若以后继续减少 RPC body 缓冲，必须先处理 body 预检、错误优先级和 session 锁的关系，不能直接把整个 body 编码塞进锁内。当前收益已通过现有验收，后续应先测真实应用是否仍由该处主导。

## 验收与复现

- 独立库普通全测、race + checkptr=2、固定官方 C/Lua 集合/packing 对照及 vet 通过。
- 原 Pack/Codec fuzz 各10秒，分别执行143169/230271次；这两项保护复用的 body/Pack，不冒充新增 RPC fuzz。
- 临时 modfile 下 GSR protocol/sproto、debugserver、benchmarks 三包通过；Go/TS Plain/MAC 联调7/7、零跳过。
- 全部候选 Go 源码与采样及归档重建一致；Decode/Unpack/EncodePacked/Pack 生产源码及正式依赖未改。未运行 GSR 全仓/race或跨平台性能。

```sh
python3 -B docs/benchmarks/2026-10-17-gosproto-rpc-workspace/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/rpc-workspace-new
python3 -B docs/benchmarks/2026-10-17-gosproto-rpc-workspace/analyze.py tmp/rpc-workspace-new
```

输出目录必须不存在；可加 --prepare-only 只重建四份源码/探针，不再次计时。candidate.patch 以56f5c21为基线，包含生产与测试变更；CHANGES 只记录发布事项。报告只收录补丁、输入指纹、原始数据、命令和结论，归档副本、二进制及临时输出留在忽略目录。
