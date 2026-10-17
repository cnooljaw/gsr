# RPC 通知路径的 session 逃逸

2026-10-17。上一轮 RPC 组帧已本地提交为 gosproto `dc4948d` / GSR `c4622bf`，提交后两库干净，未推送。本轮只将 RequestEncode 的 session 副本放入有响应分支；新修改与记录未提交/发布。GSR 正式依赖仍为 v0.4.0，Decode 冻结。

## 问题与改动

原 RequestEncode 在有响应分支执行 header.Session = &session。Go 1.24.13 的逃逸分析将函数参数 session 移到堆上，导致不使用 session 的通知也付出一次逻辑分配。现在只在同一个有响应分支中创建 requestSession 副本并取地址。生产变更为一个局部变量与指针赋值，没有新增函数、全局临时对象、池、缓存或 public API。

```go
requestSession := session
header.Session = &requestSession
```

保存副本的位置仍在原 session 锁和重复/容量检查之间。body 校验、错误返回、等待表登记及锁范围、header presence/wire/cap、输入与输出独占不变。Decode/Dispatch/Pack/Unpack/body Encode/TS 生产源码未变。修改前后完整 rpc.go 逃逸条目分别保存在 escape-before.txt / escape-after.txt；变更后 RequestEncode 的参数不再整体 moved to heap，只有有响应分支中的 requestSession 需要堆存储。错误格式化的 session 值仍可能在重复请求分支装箱，这与参数地址在入口分配不同。

## 固定测量

复用上一轮未改动的 RPC 探针及原 sampler，19 项中9个通知为目标，10个 request_cancel/response 作为未改逻辑路径控制。self/clone 各一次，固定候选两次；每项六轮、每轮四个 ABBA/BAAB 四元组、10 ms 目标块，GC 开启。没有调参、重试或筛选轮次。四次共 **7296 块、1824 四元组、456 配对轮次**，独立复算原始块到1e-12相对容差一致。Apple M2、Go 1.24.13、GOMAXPROCS=1。

负百分比表示耗时减少。百分比取六轮配对比值中位数；绝对 ns 先对每轮两臂分别取块 ns/op 的几何均值、相减，再取六轮中位数。它们是描述量，不是置信区间，也不能与整场两臂算术均值之差混用。

| 通知路径 | 第一轮耗时差 | 第二轮耗时差 | 第一轮 / 第二轮绝对差 | B/op 前 → 后（首轮） | allocs/op 前 → 后 |
| --- | ---: | ---: | ---: | ---: | ---: |
| addressbook/notification | -1.92% | -2.13% | -15.9 / -17.4 ns | 308 → 304 | 4 → 3 |
| game_small/notification | -0.07% | -0.56% | -0.8 / -6.7 ns | 724 → 720 | 4 → 3 |
| game_large/notification | +0.73% | +0.51% | +68.3 / +48.0 ns | 7444 → 7440 | 5 → 4 |
| int32_edges/notification | -0.66% | -1.41% | -33.9 / -72.4 ns | 8212 → 8208 | 4 → 3 |
| echo_ascii4k/notification | +0.34% | -1.42% | +5.7 / -23.6 ns | 9748 → 9744 | 4 → 3 |
| ascii32/notification | -2.71% | -3.38% | -7.6 / -9.8 ns | 132 → 128 | 4 → 3 |
| utf8_1k/notification | -1.20% | -1.65% | -7.6 / -10.5 ns | 2068 → 2064 | 4 → 3 |
| ascii4k/notification | +0.16% | -0.29% | +2.8 / -4.8 ns | 9748.125 → 9744 | 4 → 3 |
| zeros4k/notification | +0.47% | -0.08% | +13.5 / -2.2 ns | 9748 → 9744 | 4 → 3 |

两轮9个通知都少一次逻辑分配、约4 B/op；大型消息仍保留布局记录的额外分配。32 B 和 AddressBook 通知两轮均有小幅计时改善，其余大消息计时多接近波动范围。大型通知 +0.73%/+0.51%（约68/48 ns）的代价明确保留，不概括为所有路径变快。

Go 的 tiny allocator 将无指针小值组合在16 B块中，MemStats.Mallocs 还单列统计 tiny 请求。因此这里的“少一次 allocs/op”不等于每条消息少一个独立 GC 扫描对象或16 B物理块；减少的是一次不需要的 int32 存储请求。0.125 B 的采样尾差及GC计数完整保留，不将其解释为新对象。工具链 malloc.go:1114–1139 和 mstats.go:420–428 的本地源码指纹见 validation.json。

| 未改逻辑路径 | 第一轮耗时差 | 第二轮耗时差 |
| --- | ---: | ---: |
| addressbook/request_cancel | -1.86% | -2.24% |
| addressbook/response | -0.49% | -0.78% |
| game_small/request_cancel | -0.06% | +0.84% |
| game_small/response | +0.10% | -0.77% |
| game_large/request_cancel | -0.63% | -1.77% |
| game_large/response | -0.57% | -0.61% |
| int32_edges/request_cancel | -0.08% | -0.22% |
| int32_edges/response | -0.17% | -0.21% |
| echo_ascii4k/request_cancel | +1.06% | +1.20% |
| echo_ascii4k/response | -0.37% | -0.11% |

这10项分配数不变。echo_ascii4k/request_cancel 两轮约慢1.06%/1.20%（约18/20 ns）；另有未改路径约快2%，均原样披露。请求和通知共用 RequestEncode，编译布局和分支位置变化可能影响计时，但没有据此单独归因。

## 控制与保留判断

self 原工具总控制通过 14/19；未通过：int32_edges/notification（中位差 -0.84%，IQR 1.55pp，分配中位数相同=False）、int32_edges/response（中位差 +0.61%，IQR 1.62pp，分配中位数相同=False）、echo_ascii4k/request_cancel（中位差 +0.53%，IQR 3.08pp，分配中位数相同=False）、echo_ascii4k/response（中位差 -0.08%，IQR 1.52pp，分配中位数相同=False）、zeros4k/notification（中位差 -0.41%，IQR 0.45pp，分配中位数相同=False）。

clone 原工具总控制通过 10/19；未通过：game_small/response（中位差 -2.03%，IQR 0.84pp，分配中位数相同=True）、game_large/notification（中位差 -2.51%，IQR 0.45pp，分配中位数相同=True）、game_large/request_cancel（中位差 -2.55%，IQR 0.99pp，分配中位数相同=True）、game_large/response（中位差 -1.75%，IQR 1.19pp，分配中位数相同=False）、int32_edges/notification（中位差 -0.94%，IQR 0.83pp，分配中位数相同=False）、int32_edges/response（中位差 -1.22%，IQR 2.04pp，分配中位数相同=False）、echo_ascii4k/notification（中位差 +0.17%，IQR 15.56pp，分配中位数相同=True）、ascii4k/notification（中位差 +1.07%，IQR 0.72pp，分配中位数相同=False）、zeros4k/notification（中位差 +1.56%，IQR 6.16pp，分配中位数相同=True）。

clone 的大型通知在相同生产源码下仍有 -2.51% 偏差，多个失败仅因 tiny 分配窗口的字节中位数尾差；所有标记保留，未通过控制的精确 CPU 降幅不作强结论。

**保留实现**：通知稳定少一次不必要分配，两个小包两轮均有约2%～3%的计时改善；生产改动只增加一个分支局部副本，没有扩大 API、并发或所有权复杂度。接受并披露大型通知与 ASCII 请求的小幅计时代价，不设统一最低加速比例，也不把这次结果描述为全面 CPU 加速。

## 验收与复现

在原 TestRpcFrameAllocation 中增加同构建的通知/有响应请求分配比较。新断言在旧生产代码失败（4 对4），新实现普通和 race/checkptr 构建通过。其余既有864组角色/header/长度场景、1728输出，以及预算、错误顺序、session隔离与清理用例继续保护原契约，没有复制另一套RPC夹具。见 checks.json。

本轮执行库全测（启用固定官方C/Lua）、race + checkptr=2、vet，及临时 modfile 下 GSR protocol/sproto、debugserver、benchmarks 三包，全部通过；Go/TS Plain/MAC 7/7、零跳过。本轮没有改 wire/Pack/body 算法，未重复其有限 fuzz；上一切片的 fuzz 证据仍是历史证据，不冒充本轮执行。未运行 GSR 全仓/race、跨平台或新 PB/TS 性能矩阵。

候选补丁与最终 Go 源码逐字节一致；四份归档、探针、夹具、生成物及工具指纹重建一致。所有原始块/分配窗口/GC、失败标记、命令及 source hashes 保留。

```sh
python3 -B docs/benchmarks/2026-10-17-gosproto-rpc-session/reproduce.py \
  --library /private/tmp/gsr-gosproto-fork --output tmp/rpc-session-new
python3 -B docs/benchmarks/2026-10-17-gosproto-rpc-session/analyze.py tmp/rpc-session-new
```

输出目录须不存在；--prepare-only 只重建四份归档而不计时。基线 dc4948d；candidate.patch 包含生产与测试，CHANGES仅记录发布事项。归档源码、构建产物与临时输出留在忽略目录。本轮没有推送、发布或升级正式依赖。
