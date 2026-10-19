# 当前编码路径 CPU 与分配诊断

前轮已本地提交：gosproto `d2f6027`（固定 len/cap 视图）、GSR `175028b`（报告），均未推送。本轮只从该已提交版本归档诊断，生产代码、Decode 和正式 v0.4.0 依赖没有改动；新报告与诊断工具留在 GSR 工作树供审查。

## 方法与边界

计时前固定[计划](plan.json)、[探针](profile-probe.go.txt)与[驱动](reproduce.py)。复用原 70 项构造器及相同输入/schema 缓存，每个独立进程只保留一个所选操作，清除输出引用、GC，再预热 50 ms。共选 16 个入口：大型/896 整数数组的 Encode、Pack、PEncode 和三种实际 RPC，加 4KiB ASCII Pack/请求、4KiB 零字节手工帧和 32B 通知。只保活所选闭包，不保留整组操作列表。

CPU 每项一次 2 秒，手动在构造/选择/GC/预热后启动 pprof，循环结束后停止；包括循环、闭包、runtime 与 profiler 的影响。分配每项另起进程固定 20,000 次、memprofilerate=1，共 320,000 次。后者的完整 profile 还包含进程初始化、70 项构造、预热和收尾，不能把其总字节除以 20,000 当作 B/op。B/op、allocs/op 以 benchmark 循环计数为主，pprof 用于定位来源。CPU 与逐次分配插桩的耗时分别保留，不能混作正常运行基线。

GOMAXPROCS=1、GOGC=100、GOMEMLIMIT=MaxInt64；未关闭 GC，没有每调用强制 GC。32 个采集进程一次完成，无重试。96 个边界快照的 limiter 全为零，最大堆对象快照 4,038,088 B（约 3.85 MiB），快照不能代表峰值。CPU 采样后才渲染 pprof，不并行构建或运行重检查。

## 当前观测

下表是带 profiler 的诊断耗时（ns/op），不是新旧配对比较，也不是 PB/SP 排名。CPU 进程与逐次分配进程的 B/op、allocs/op 在所有 16 项一致；完整值见[测量](measurements.json)，包含采样次数。

| 入口 | CPU profile 进程 ns/op | 逐次分配进程 ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `body/game_large/sproto_encode` | 5888.0 | 9488.0 | 3968 | 2 |
| `body/game_large/sproto_pack` | 2249.0 | 2827.0 | 3456 | 1 |
| `body/game_large/sproto_pencode` | 7430.0 | 12668.0 | 3968 | 2 |
| `rpc/game_large/notification` | 7756.0 | 11083.0 | 3968 | 2 |
| `rpc/game_large/request_cancel` | 7458.0 | 11987.0 | 3968 | 2 |
| `rpc/game_large/response` | 7532.0 | 11100.0 | 3968 | 2 |
| `body/int32_edges/sproto_encode` | 1891.0 | 2846.0 | 4096 | 1 |
| `body/int32_edges/sproto_pack` | 2799.0 | 2672.0 | 4096 | 1 |
| `body/int32_edges/sproto_pencode` | 3684.0 | 4234.0 | 4096 | 1 |
| `rpc/int32_edges/notification` | 3467.0 | 4743.0 | 4096 | 1 |
| `rpc/int32_edges/request_cancel` | 3624.0 | 4433.0 | 4096 | 1 |
| `rpc/int32_edges/response` | 3411.0 | 4440.0 | 4096 | 1 |
| `body/echo_ascii4k/sproto_pack` | 774.1 | 1576.0 | 4864 | 1 |
| `rpc/echo_ascii4k/request_cancel` | 1096.0 | 1889.0 | 4864 | 1 |
| `body/zeros4k/frame` | 3723.0 | 28555.0 | 14616 | 5 |
| `rpc/ascii32/notification` | 162.5 | 1471.0 | 64 | 1 |

`request_cancel` 通过公开 RequestEncode 编码并 CancelSession，包含真实 session 登记与取消；不是只测私有 Pack。`zeros4k/frame` 保持原手工 body/header/拼接/Pack 路径，不能与融合 RPC 当作同一 API。

## 热点与解释

以下为总 CPU 样本的累计百分比，父子函数重叠，不能相加；`runtime.kevent` 列为 flat。没有剔除 runtime 样本或以其占比校正耗时。单次 2 秒、约 10 ms 粒度的采样不足以证明逐行细微差异。

| 入口 | Pack | 记录预检 | 记录写出 | runtime.kevent |
| --- | ---: | ---: | ---: | ---: |
| 大型 PEncode | 17.70% | 12.83% | 15.93% | 23.45% |
| 大型通知 | 15.58% | 14.72% | 16.88% | 20.35% |
| 大型请求+取消 | 9.44% | 15.45% | 16.74% | 24.03% |
| 大型响应 | 17.47% | 16.59% | 16.59% | 21.83% |
| 整数 PEncode | 29.39% | — | — | 35.96% |
| 整数通知 | 31.74% | — | — | 39.57% |
| 整数请求+取消 | 26.58% | — | — | 35.86% |
| 整数响应 | 26.72% | — | — | 36.64% |

大型纯 Encode 的 `encodedSize`/`encodeMessageInto` 累计为 31.58%/37.28%，其中整数记录 helper 为 18.86%/19.74%。PEncode 中对应的通用入口是 24.78%/26.55%，不可再与其子 helper 相加。记录 helper 已经通过受验证的 offset 读取 *int64，没有逐字段 safe reflect 取值；这里不能再笼统归咎于反射。数组入口仍有 Value.Index/UnsafePointer 等成本，和记录内部成本分别考虑。

[大型记录写出源码采样](body-game_large-sproto_pencode-record-source.txt)落点包含字段/tag/presence 遍历、头部写出与游标定位。它只给出候选方向，不证明其中某条指令必然是瓶颈。请求对应的[独立源码视图](rpc-game_large-request_cancel-record-source.txt)一并保留。

整数 PEncode 的 `encodeInt64Slice` 累计 16.67%，Pack 为 29.39%；实际 RPC 也仍能看到 Pack。普通 word 的 mask/前缀位置写出值得继续研究，但之前“目标游标”和“连续非零段批量写出”的负面实测仍有效，不因新 profile 重新包装为已证实方案。

大型消息四种组合/纯 Encode 均为 3968 B、2 次：3456 B 输出尺寸级别，加 512 B 布局外溢。分配 profile 的主要站点分别是 encodeBufferWithLimits 的输出 make 和 encodeLayout.add 的外溢 make；完整 profile 中旧副本/夹具的构造分配不属于计时循环。整数、ASCII4KiB、32B 通知各一次最终输出分配。头部和 session 已不是这些成功路径的新增独立对象来源。

4KiB ASCII 纯 Pack、实际请求及零字节手工帧的 `runtime.kevent` 分别为 72.91%、69.96%、75.33%，沿 netpoll/startTheWorldWithSema 等 runtime 栈出现；limiter=0 不代表没有 GC 或 runtime 开销。当前没有 syscall/调度跟踪确认成因，不把这些占比全归因于协议、GC 或实际阻塞，也不据此估算算法收益上限。其原始 profile 与耗时完整保留。

## 下一步单一实验

先只检查 `encodeIntegerRecordInto` 头部/内容写出的动态切片定位与边界检查：先看现有二进制的 BCE/汇编，再尝试保持原写入地址和顺序的局部视图，确认有实际减少的重复工作后才计时。只改写出 helper，不同时修改记录预检、Pack、布局增长或扩大 unsafe；字段值每阶段仍重读，保留 nil/tag/元信息替换和所有预算/失败契约。

不重复已做过的 Fields 切片缓存或 nil 分支前移，不用新增布局记录缓存字段值，不放大小包栈、不引入池。若预检已经消除了目标工作或候选只是增加检查，保留静态证据，不为凑候选启动完整采样。若进入测量，继续固定原 70 项逐项进程 self/clone 与两轮比较，按绝对纳秒、真实 API、分配、累计方向及复杂度判断，小而可靠的收益同样计入，不设最低百分比。

## 验证、复现与文件

harness 的统计、选择、GC 计数、代表夹具、默认运行指标及完整 70 项顺序检查通过，go vet 和一次构建通过。32 次采集、96 份常规 pprof 摘要和 2 份补充记录源码视图均成功。[独立分析](analyze.py)复核所有选择/次数/输出、96 个快照、源/二进制/32 份 profile 哈希及 fresh prepare-only 归档；只规范化不同输出目录后比较 go.mod，原哈希分别保留。结果见[分析](analysis.json)、[输入清单](inputs.json)、[命令](commands.json)及[补充渲染命令](analysis-commands.json)。补充渲染复用现有 profile，没有新增采样。

本轮没有改生产行为，不重复库全测、官方 oracle、race/checkptr、fuzz 或 GSR/GoTS 集成；这些证据沿用前轮已提交验收，不冒称本轮重跑。库工作树干净。没有新 A/B、PB 或 TS 计时，不能用本次绝对值推导相对前轮加速。

原始二进制 profile、可执行文件与归档留在忽略目录 `tmp/gosproto-current-profile/`；可审查的 JSON、pprof 文本和复现程序入报告目录。文本只去掉行尾空白，[curation.json](curation.json)同时记录原文和整理后哈希，命令保留原输出。复现时使用新输出目录：

```sh
python3 -B docs/benchmarks/2026-10-19-gosproto-current-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/current-profile-repro
python3 -B docs/benchmarks/2026-10-19-gosproto-current-profile/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/current-profile-repro-fresh --prepare-only
python3 -B docs/benchmarks/2026-10-19-gosproto-current-profile/analyze.py \
  --library /private/tmp/gsr-gosproto-fork \
  --output tmp/current-profile-repro --fresh tmp/current-profile-repro-fresh
```
