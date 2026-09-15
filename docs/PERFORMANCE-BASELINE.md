# 本地规模基线：2026-09-15

测量代码版本：`78b58a0`。本轮只运行已有测试和 benchmark，没有修改调度、Timer 或消息实现。

## 环境与口径

- Go 1.23.3，darwin/arm64；Go benchmark 报告 CPU 为 Apple M2，运行后缀为 `-8`。
- 普通开发机的小样本测量，没有隔离其它进程负载，不使用 race 结果评价性能。
- 保留 Go 构建缓存。benchmark 命令实际执行，不将缓存测试输出视为本轮测量。
- 未采集 RSS、CPU 利用率、GC 暂停或 p95/p99，不给出生产容量承诺。

## 空闲规模测试

| 场景 | 创建/登记耗时 | 关闭耗时 | goroutine 增量 |
| --- | ---: | ---: | ---: |
| 1,000 个空闲 Service | 1.104 ms | 0.454 ms | 1,000 |
| 10,000 个空闲 Service | 15.360 ms | 8.524 ms | 10,000 |
| 10,000 个未触发 Timer | 9.259 ms | 13.171 ms | 10,000 |

Service 使用空 Handler、MailboxCapacity=1；创建耗时包含 Runtime 初始化及 Service 创建流程。Timer 使用 fake clock，测量登记和取消清理，不包含真实时钟到期精度、集中触发或 Handler 消费。相关测试同时检查关闭后的 Runtime 状态、Service/Timer/PendingCall 数量，不只打印耗时。

Service 测试打印的 HeapAlloc 分别为 1,427,376 和 11,646,200 字节。这是采样时整个测试进程的 Go 堆用量，包含测试夹具及前序活动影响，不是单个 Runtime 的独占内存，也不是总进程内存；不能直接除以 Service 数量得出每 Service 内存成本。

## 固定次数 benchmark

| 基准 | 迭代数 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| CreateServices1000 | 3 | 1,389,986 | 685,274 | 6,337 |
| CreateServices10000 | 3 | 16,573,778 | 6,508,901 | 65,589 |
| CreateTimers10000 | 3 | 24,192,264 | 6,517,856 | 96,796 |
| SendHotService | 10,000 | 151.7 | 193 | 2 |
| CallRoundTrip | 10,000 | 1,515 | 584 | 10 |

必须按各基准的真实计时范围理解：

- CreateServices 的一次 op 是完整创建并关闭一组 Service，包含 Runtime 生命周期，不是单个 Service。
- CreateTimers 的一次 op 包含 fake clock、Runtime、一个 Service、10,000 个 Timer 的创建及关闭；不是单次 After 延迟。
- SendHotService 只计时宿主 Runtime.Send 接纳，runner 同时消费，关闭不计时。MailboxCapacity=b.N+1，本次为 10,001；不代表默认队列容量、满载背压或端到端完成吞吐。
- CallRoundTrip 包含宿主 Send 触发 caller、Service 内本地 Call 往返，以及测试 channel 通知，不是纯 Call 内部成本，也不是跨节点 Call 延迟。
- B/op 是计时阶段内 Go 堆分配统计，不是常驻内存，也不完整体现 goroutine 栈和操作系统资源。
- 3 次迭代仅作量级记录；本轮没有统计显著性分析，不据此宣称性能提升或回退。

## 复现命令

```sh
go test ./runtime -run '^TestRuntimeScale(1000Services|10000Services|10000Timers)ReportsBaseline$' -bench '^BenchmarkRuntimeCreate(Services1000|Services10000|Timers10000)$' -benchtime=3x -benchmem -v -timeout=2m
go test ./runtime -run '^$' -bench '^BenchmarkRuntime(SendHotService|CallRoundTrip)$' -benchtime=10000x -benchmem -timeout=2m
```

固定消息次数避免现有 Send 基准的 MailboxCapacity 随自动校准次数增长到不必要的规模。这里不修改现有基准行为；未来若测真实背压，需另设固定容量的场景。

## 当前结论与下一项

在这台机器、这些空闲/空 Handler 场景下，10,000 规模的创建与关闭可以完成，未见初始化耗时上的明显阻塞；不能据此认定 10,000 个活跃业务 Service 的容量已经验收。

当前每个 Running Service 一个 runner、每个 Timer 一个等待 goroutine 的数量成本仍然存在。下一项有意义的测量应是固定 Mailbox 容量下的持续热点负载，分别观察接纳成功、队列满、实际处理数量和端到端延迟；再测 Timer 集中到期与日志开启后的影响。没有这些数据前，不据本轮小样本改 Scheduler 或引入集中 Timer loop，也不设未经业务确认的性能门槛。

## 固定容量热点突发：2026-09-15 补充

测量使用 `5e3aeb9` 的生产实现，加上本轮新增的 [热点测试](../runtime/hot_load_test.go)，未改变 Mailbox 或 Scheduler。环境与上文一致，未启用 race 的首轮输出如下：

| Mailbox 容量 | 发送尝试 | 接纳成功 | 满载拒绝 | Handler 处理 | 发送阶段耗时 | 到达延迟 p50 / p95 / p99 / 最大 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 64 | 100,000 | 16,532 | 83,468 | 16,532 | 24.743 ms | 48 / 314.458 / 652.209 / 987.167 μs |
| 1024 | 100,000 | 36,155 | 63,845 | 36,155 | 23.963 ms | 569.750 / 1,479.417 / 1,717.208 / 1,887.333 μs |

4 个宿主发送 goroutine 同时起跑，各固定尝试 25,000 次，满载不重试；全部投向一个 Service。只允许 Send 返回 nil 或 ErrMailboxFull，逐条校验接纳记录与 Handler 观察记录，拒绝重复或未接纳消息，等成功消息全部到达 Handler 后才关闭 Runtime。

测量口径：

- 延迟从每次调用 Send 前取时间戳到 Handler 采样，包含发送调用、锁竞争和队列等待；不是纯 Mailbox 等待，也不包含 Handler 返回后的 completion。
- 分位数只统计成功接纳消息，拒绝请求没有处理延迟样本；不能用它描述全部发送尝试的服务质量。
- Handler 只向固定上限 100,000 条的采样 channel 写结果，没有业务计算、数据库、日志或网络；采样本身有时间和内存开销。
- 测试记录全部样本的总观察耗时为 25.669 ms / 26.095 ms，但包含测试端遍历接纳结果与收集采样的开销，不能直接换算成框架的完成吞吐。
- 这是约 25ms 的短突发压力，不是长时间稳态负载。队列满载是既定非阻塞接纳策略的结果，不是已接纳消息丢失。
- 本次更大的队列接纳更多，同时延长到达尾延迟；该现象不作为跨机器或跨业务的普遍数值结论，不据此修改默认容量。

显式运行，默认测试跳过此测量：

```sh
GSR_LOAD_TEST=1 go test ./runtime -run '^TestRuntimeHotMailboxLoad$' -v -timeout=2m
GSR_LOAD_TEST=1 go test -race ./runtime -run '^TestRuntimeHotMailboxLoad$' -timeout=2m
```

race 只验证夹具和运行链并发安全，不能将其耗时与上表比较。重复观测时应记录运行条件和全部样本，不挑选“最好的一次”。下一步仍需按目标业务节奏确定持续输入速率、Handler 成本与时长，再测稳态容量和尾延迟；不把固定突发测试包装成生产验收。

## Timer 集中到期：2026-09-15 补充

测量使用 `180f697` 的生产实现，加上 [Timer 集中到期测试](../runtime/timer_burst_test.go)。在同一个 Service 的 OnStart 中登记 10,000 个 Timer，全部 Running/arm 后推进一次 fake clock，使它们共享的 deadline 到期。未启用 race 的首轮记录如下：

| Mailbox 容量 | 到期 | 接纳 | 满载拒绝 | Handler 处理 | 观察到全部投递完成 | 到达 p50 / p95 / p99 / 最大 |
| --- | ---: | ---: | ---: | ---: | ---: | --- |
| 64 | 10,000 | 5,738 | 4,262 | 5,738 | 11.454 ms | 3.105 / 9.828 / 10.199 / 10.242 ms |
| 1024 | 10,000 | 6,083 | 3,917 | 6,083 | 12.901 ms | 4.749 / 11.315 / 11.682 / 11.748 ms |

测试校验 Created=Fired=10,000、Cancelled=0、Admitted+Rejected=10,000；已接纳数量必须等于 Handler 观察数，每个消息下标不重复且 Source=0。等待这些消息到达后再关闭 Runtime，确认没有额外观察或残留 Timer。

口径与限制：

- 到达延迟以调用 fakeClock.Advance 前的真实时间为起点，到 Handler 采样为终点；包含 fake clock 遍历和唤醒、goroutine 调度、投递竞争与 Mailbox 排队。它不是操作系统 Timer 的到期误差。
- “观察到全部投递完成”包含 Inspect 及毫秒级轮询开销，不能用于宣称 Timer 调度精度。
- Handler 只写入固定容量采样 channel，没有业务负载或日志。分位数只包含成功接纳项；不能忽略被拒绝的 Timer。
- fake clock 使用 map 遍历 Timer，不承诺这一轮的唤醒顺序；不验证同 deadline 的 Timer 按创建顺序执行。
- 本次容量 1024 仍有 39.17% 满载拒绝，反映的是集中唤醒与目标消费的竞争，不表示 Timer 登记失败。After 成功不是最终投递成功保证；当前契约不为 Timer 提供优先级或自动重试。
- 该记录不足以决定更换 Timer 架构或增大默认 Mailbox。若业务要求关键定时动作不可丢失，需要先明确操作语义，再讨论外层补偿/调度策略，不能静默修改 Core 契约。

复现命令：

```sh
GSR_LOAD_TEST=1 go test ./runtime -run '^TestRuntimeTimerBurstLoad$' -v -timeout=2m
GSR_LOAD_TEST=1 go test -race ./runtime -run '^TestRuntimeTimerBurstLoad$' -timeout=2m
```

## 日志输出成本：2026-09-15 补充

生产代码版本 `4d1aec0`，新增 [日志批量测试及基准](../logging/load_test.go)。同一 Apple M2 / Go 1.23.3 环境，固定每批 10,000 次 Info，单发送者、容量 1024，记录固定消息和两个小字段；每批重新创建 Logger，并将关闭排空计入计时。禁用模式使用 nil Logger，不创建后台写入者。

| 模式 | 批次数 | 每批耗时 | 平均每次尝试 | 每批接纳 / 丢弃 / 禁用 | B/op（每批） | allocs/op（每批） |
| --- | ---: | ---: | ---: | --- | ---: | ---: |
| disabled | 3 | 0.184 ms | 18.43 ns | 0 / 0 / 10,000 | 78,224 | 9,748 |
| io.Discard | 3 | 18.596 ms | 1,860 ns | 10,000 / 0 / 0 | 12,035,389 | 139,789 |
| file | 3 | 21.928 ms | 2,193 ns | 10,000 / 0 / 0 | 12,037,693 | 139,804 |

基准分开累计禁用、接纳和满载丢弃；Close 错误、WriteFailureCount 非零或计数不一致直接使测量失败，不把输出故障算作正常高吞吐。普通 TestLoggingBatchAccounting 用小批次验证文件行数等于接纳数，禁用不算接纳。

解释边界：

- io.Discard 仍执行调用处时间/位置采集、字段快照、JSON 编码、排队和后台消费，仅省去实际 I/O；不是禁用日志。
- file 使用临时目录和正常文件写入，不 fsync，不能等同于可靠落盘延迟。每个 benchmark 子项独占目录，批次追加同一个文件；本次数据量不足以触发 100 MiB 滚动。
- ns/attempt 是整批生命周期总时间除以尝试次数，不是单次 Info 的延迟分位数。若未来出现丢弃，必须结合接纳数量理解，不可直接换算为日志写入吞吐。
- disabled 仍有调用侧字段构造及装箱成本；nil 入口不代表参数表达式不会执行，不据此新增 lazy callback 或另一套日志 API。
- B/op 和 allocs/op 是每 10,000 次尝试的批次统计，不是每条日志。当前 JSON 快照存在明确分配成本；真实业务场景需要先测对象大小和日志频率，再决定是否值得优化。
- 本次没有丢弃，只能说明此单发送者短批次没有观察到满载。没有测试多个 Service 同时记录、慢盘、日志滚动或 SLS 采集，更不能将两种输出的耗时差解释为纯磁盘成本。

复现：

```sh
go test ./logging -run '^TestLoggingBatchAccounting$' -bench '^BenchmarkLoggingBatch$' -benchtime=3x -benchmem -v -timeout=2m
```

普通全仓测试覆盖计数夹具；benchmark 仅显式运行。本轮不改变 Logger 实现、默认容量或输出契约。

## 暂定 10,000 条/秒输入：2026-09-15 补充

用户暂定单进程 10,000 条消息/秒作为后续测量目标。生产版本 `737cd6b`，新增 [定速测试](../runtime/paced_load_test.go)，不改 Runtime。此时还没有业务 Handler 计算成本、消息分布、日志频率或延迟/拒绝率验收线。

先采用单热点 Service 的最小场景：默认 Mailbox 容量 1024，单宿主发送者每 10ms 发送 100 条，共 1,000 批；Handler 只写采样结果、不记录日志。计数守恒和消息唯一性是测试断言，速率与延迟是观测结果，不预设未经裁决的性能门槛。

首轮非 race 结果：

| 指标 | 结果 |
| --- | ---: |
| 目标输入速率 | 10,000 条/秒 |
| 实际输入速率 | 9,860.92 条/秒 |
| 输入持续时间 | 10.141 秒 |
| 尝试 / 接纳 / Handler 处理 | 100,000 / 100,000 / 100,000 |
| 满载拒绝 | 0 |
| 到达 p50 / p95 / p99 | 16.875 / 49.625 / 189.083 μs |
| 到达最大值 | 8.790 ms |

生成器使用 Go ticker，可能受调度延迟和丢 tick 影响，不按累计欠额追赶补发；目标速率不能替代实际速率。本轮实际速率约为目标的 98.6%，只证明该实际输入下空 Handler 的运行链完成了全部消息，不能宣称严格达到 10,000 条/秒。

每批是 100 条短突发，不是完全均匀的 100μs 间隔。延迟从每条 Send 前取时间戳到 Handler 观察，包含接纳和排队，不包含真实业务处理、数据库或网络。10 秒不是长期稳定性测试；单 Service 的热点压力也不代表 10,000 个活跃 Service 的整体资源成本。最大延迟明显高于 p99，应保留该数据，不用平均值掩盖调度抖动。

```sh
GSR_LOAD_TEST=1 go test ./runtime -run '^TestRuntimePaced10000Load$' -v -timeout=1m
GSR_LOAD_TEST=1 go test -race ./runtime -run '^TestRuntimePaced10000Load$' -timeout=1m
```

默认跳过，显式负载开关启用；生成与等待处理共享 20 秒预算。成功接纳消息全部观察完才关闭 Runtime。race 用于检查同步，不用于容量比较。后续应增加业务成本和日志频率，并另行确定长时间及尾延迟验收线，不据本轮结果优化或更换 Scheduler。
