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
