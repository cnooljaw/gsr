# GSR Core 与 Cluster 实施路线图

本文把已接受的 API 和 ADR 拆成可验证的纵向切片。每个切片只引入完成当前运行链所需的能力；Core 完成后再开始 Cluster。

当前应用入口及编码契约以 [ADR 0011](adr/0011-service-first-application-api.md) 为准：gsr.Run 统一管理运行环境，业务使用 self.Cluster()/self.Logger()；已移除 CommandCodecRegistry，合法未知命令由目标 Service 判断。下方历史切片中的 registry、未知 Command 坏帧及 Node/Host 组装描述不作为当前实施要求。

本文保留历次切片记录，部分“本轮未实现”描述仅代表当时范围。当前能力与证据汇总见 [当前阶段验证清单](VALIDATION-STATUS.md)，不要将历史切片的非目标误读为当前缺失。

当前空闲规模及消息基准的实测记录见 [2026-09-15 本地规模基线](PERFORMANCE-BASELINE.md)。该记录不作为生产容量门槛，持续负载、真实背压、集中 Timer 到期和日志开销仍需分别测量。

用户暂定单进程消息输入目标为 10,000 条/秒，尚未确定业务 Handler 成本、消息分布、日志频率、持续时长和延迟/拒绝率验收线。TestRuntimePaced10000Load 先采用单热点 Service、默认容量 1024、无业务计算和日志的 10 秒定速场景，通过 GSR_LOAD_TEST=1 显式启用；必须报告实际输入速率，不能将生成器未达到目标的样本宣布为目标容量已验收。

该测试已扩展关闭日志/每消息文件日志对照，核对业务接纳、日志接纳与文件行数，分别观察 Handler 到达及日志调用后延迟。首轮对照实际输入均低于目标，文件模式出现 Mailbox 与日志满载拒绝；行为断言通过不等于容量达标，结果保留在性能基线，尚未据此修改实现或容量配置。

后续仅补生成器 tick 唤醒、估算跳过间隔、发送批次耗时和输入阶段 GC/分配观测；新样本接近目标且无丢弃，生产实现未变，不宣称性能优化或已确定旧样本根因。保留两轮数据，异常再现时需要同轮 trace/profile 证据。

已补固定容量热点突发测量：GSR_LOAD_TEST=1 启用 TestRuntimeHotMailboxLoad，分别记录成功接纳、满载拒绝、实际处理数量及发送到 Handler 的延迟分位数；只有成功消息全部被观察后才关闭，不把关闭丢弃算作正常处理。持续输入速率与业务 Handler 成本仍需单独确定。

同一开关已加入 TestRuntimeTimerBurstLoad：10,000 个 Timer 同轮到期、容量 64/1024，核对到期=接纳+拒绝、Handler 次数、零 Source 和关闭后清理，记录从 fake clock 推进到 Handler 的延迟；不当成真实时钟精度，也不改变满载不重试的契约。

日志基准 BenchmarkLoggingBatch 已补禁用、io.Discard、文件三种模式，每批 10,000 次调用并包含关闭排空，报告接纳/丢弃/禁用及分配成本；TestLoggingBatchAccounting 验证计数与文件行数。单发送者文件缓存写入结果不代表慢盘、多 Service 并发或可靠落盘性能。

Core v0.1 是第一条公开兼容基线。此前各切片只在能力真正实现时把对应方法加入代码接口：Core 1 提供 Self/Send，Core 2 加入 After/CancelTimer，Core 3 加入 Call；不得为了让接口提前看似完整而加入固定返回“未实现”的占位方法。每次接口增长仍须同步文档和测试。

## 总体顺序

```text
Phase 0  契约闭合
  ↓
Core 0   基础类型与 Runtime 骨架
  ↓
Core 0.5 Runtime 组件生命周期骨架
  ↓
Core 1   本地 Send 与 Service 生命周期
  ↓
Core 2   Timer
  ↓
Core 3   Call、PendingCall 与自动 Reply
  ↓
Core 4   故障、观测、并发与规模收尾
  ↓
Core v0.1
  ↓
Cluster 0–4
```

## Phase 0：契约闭合

状态：已完成当前实现所需裁决，后续发现公开契约缺口时先更新文档和 ADR。

已经冻结：

- Core 和外层地址都只使用 Runtime 局部 `ServiceID`；不定义额外的地址包装类型。稳定逻辑身份由 Core 外层的 Registry/Discovery 管理。
- Runtime 不自动创建 Bootstrap Service；`Runtime.Start()` 只启动基础设施，宿主随后创建普通 MainService，由它通过 ServiceContext.CreateService 组装业务。
- `CreateService` 固定为 `CreateService(ctx context.Context, service Service) (ServiceID, error)`；Service 由业务层构造，Runtime 接管其生命周期。
- `Service.OnStart` 固定接收 `context.Context` 和 `ServiceContext`；CreateService 的 context 是协作取消信号，并传给 OnStart。
- OnStart 失败只回滚当前 Service，Runtime 保持 Running，可以继续创建其它 Service；失败 ID 不发布且永不复用，并由 Runtime 至多调用一次 OnStop 做部分初始化清理。
- Runtime 生命周期固定为 `Created → Starting → Running → Stopping → Stopped → Closed`；`Stop(ctx)` 负责有序停止，`Close()` 负责最终资源释放，Stop 后不得再次 Start。
- Core admission 复制 `Args` 外层切片；引用型参数由 typed client 按消息类型构造快照。
- Handler 使用只读 `CommandContext.Source()` 获取 Runtime 填入的来源；普通 Send/Call 为当前本地 Service，Timer Command 为 `ServiceID(0)`。
- 业务 API 不接收任意 source 参数；Redirect/Relay 只作为明确受信任的 Runtime/Adapter 能力转发 Runtime-owned Message，首版不进入 `ServiceContext`，也不转移 Call 的 PendingCall/Session。
- Handler 通过返回值自动 Reply，不公开 `Reply()` 或异步 response API。
- `Service.Handle` 是 Runtime 唯一业务入口；`gsr` 应用层提供可选的每 Service 不可变 `Dispatcher`，Core 不提供第二套分发器、命令注册表或 per-Service 命令预检查。
- `Dispatcher` 构造时复制并校验 `CommandID -> typed Handler` 表，运行时检查参数数量/类型并通过反射调用；未知命令仍进入 Handler，命令错误使用稳定 ErrorClass/ErrorCode。
- Call 使用 run-to-completion，并受 Runtime `MaxCallWait` 和调用方 context 中更早的 deadline 限制。
- 第一版每个 Running Service 使用一个 Runtime-owned runner goroutine。
- OnStart 的 self Send 返回 `ErrServiceStarting`，self Call 返回 `ErrCallCycle`；启动期 Timer 最早在 Running 后 admission。
- Go 不能强杀 Handler 或 OnStop；超时任务从业务寻址中隔离，但必须追踪到真实返回。
- Cluster、NodeID、ClusterEnvelope、连接和编码不进入 Core；不引入 Epoch/Lease/Generation，Runtime/Cluster 不判断业务资源的所有权、一致性或过期。

## Core 0：基础类型与 Runtime 骨架

状态：基础切片已完成。`runtime` package 提供类型、ID 分配、结构化错误、配置校验和 Runtime 生命周期；Service、Mailbox、Timer、Cluster 分别在后续 Core/Cluster 切片实现。本轮公开生命周期已收口为 `Stop(ctx)` + `Close()`。

实现：

- `ServiceID`、`SessionID`、`TimerID`、`CommandID`。
- `CommandID` 固定为 `uint32`，与协议命令字段同宽；CommandID 由业务/协议包静态维护，Runtime 不分配。
- 可注入临界状态的 uint64 单调 ID 分配器。
- `ErrIDExhausted`。
- `ErrorClass`、`ErrorCode`、`StatusError`。
- Runtime Config 默认值与完整校验，包括有限且大于零的 `MaxCallWait`、Mailbox 容量和 shutdown deadline。
- Runtime 的 Created、Starting、Running、Stopping、Stopped、Closed 状态骨架。
- 空 Runtime 的 `Start`、幂等 `Stop(ctx)`、`Close()` 和并发等待结果。

验收：

- 所有 ID 从 1 开始，零值保留。
- 从 `math.MaxUint64-1` 直接验证最后一个 ID 和永久耗尽，不执行长循环。
- `StatusError` 按 Class 和 Code 支持 `errors.Is`，Message 不参与判断。
- 重复 Start 返回稳定生命周期错误。
- 并发 Stop/Close 只执行一套生命周期流程；等待者可以因自己的 context 提前返回，但不取消已经开始的停止流程。

非目标：Service、goroutine、Mailbox、Timer 和 Cluster。

## Core 0.5：Runtime 组件生命周期骨架

状态：基础协调器已完成，Core 1 已将 Service admission 和 Scheduler 以固定组件接入；Timer 与 PendingCall 已按计划作为独立组件加入，不得恢复单一 `closeFn`。

实现：

- Runtime 的 `Created → Starting → Running → Stopping → Stopped → Closed` 状态。
- 内部 Lifecycle Coordinator 和显式 `startOrder`/`stopOrder`。
- 组件启动失败时逆序回滚已成功启动的组件；部分启动失败后 Runtime 不允许重试。
- `Stop(ctx)` 进入 `Stopping` 并关闭全局 admission；`Close()` 使用 Runtime `ShutdownTimeout` 启动或等待 Stop，再释放已安全可释放的 Core 资源。
- `Stop`/`Close` 在 `Starting` 期间记录停止意图；调用方提前取消等待不会取消启动完成后的停止流程。
- `Stopped` 表示停止流程完成，`Closed` 表示 Runtime-owned 资源最终释放；两者都拒绝新的 admission，且不得再次 Start。
- 生命周期组件 panic 转换为 Runtime 生命周期错误，后续组件继续执行并聚合关闭错误。

关闭计划（组件进入实现后逐项填入）：

```text
Runtime admission
  → Timer 投递
  → PendingCall completion
  → Service admission / OnStop
  → Scheduler / runner wait
  → Core Registry 和资源释放
```

Cluster、Transport、NodeSender 和 ConnectionAgent 不加入 Core 计划。外层 Node/Host 先停止入站和新的出站 admission，再关闭 Core；Core 完成后再释放网络连接和 I/O goroutine。

验收：

- 启动和关闭严格按计划顺序执行，而不是隐式反向或注册顺序。
- 启动失败只回滚已经成功启动的组件，失败组件的局部清理由其 owner 负责。
- 同一关闭计划只执行一次；并发等待者可以提前结束等待，但不取消关闭流程。
- 关闭预算到期时返回稳定可判定错误，组件状态和残留任务仍可观测。

## Core 1：本地 Send 与 Service 生命周期

状态：已完成当前切片。本轮已落地 Service、Mailbox、LocalDispatcher、per-Service runner、Scheduler、Source 绑定和生命周期接入；Call 和完整观测按后续切片推进。

实现第一条完整运行链：

```text
Start
  → CreateService
  → Allocate ServiceID
  → OnStart
  → Publish Running Service
  → Start runner
  → Send admission
  → Mailbox
  → Handler
  → Stop
  → OnStop
```

实现：

- `Service`、`ServiceContext`、`CommandContext`。
- `Runtime.CreateService(ctx context.Context, service Service) (ServiceID, error)`；Service 由应用层传入实例，不提供工厂或内置 Bootstrap Service。Service 内通过 ServiceContext.CreateService 复用这条创建路径。
- Service registry 和不可复用 ServiceID。
- 有界 Mailbox 和非无限阻塞 admission。
- per-Service runner；Scheduler 负责 runner 创建、panic recovery、停止、等待和观测，不引入全局 ReadyQueue 或固定 worker pool。
- 将 Service 管理和 Scheduler 按 Core 组件生命周期计划接入 Runtime；不得在 `Close` 中追加匿名清理逻辑。
- 唯一的本地 Message 和 LocalDispatcher。
- 可选的 per-Service `gsr.Dispatcher`、typed Handler 参数绑定和构造期命令表校验；不实现全局 `RegisterCommand` 或 Runtime 命令白名单。
- `Self`、`Send`、`CreateService`、Runtime `Stop`、Runtime `Close`。
- Runtime-owned adapter 可使用 `Runtime.Send` 注入 `Source == ServiceID(0)` 的本地消息；普通 Service 使用绑定自身 ID 的 `ServiceContext.Send`。
- `OnStart(context.Context, ServiceContext)` 的协作取消和 Service 组装边界。
- Starting、Running、Stopping、Stopped、Closed 的接受边界。
- 排队 Send 丢弃与结构化诊断。
- Handler 和 OnStop 超时后的残留 runner 追踪。
- Dispatcher 对非法 Handler 签名、参数数量/类型错误和未知命令的稳定错误行为。
- Dispatcher 构造输入复制、直接实现 `Service.Handle` 的 escape hatch，以及 Send/Call 由 Runtime 统一完成 Reply 的边界。

验收：

- Service A 普通 Send 到 B 时，B 的 `CommandContext.Source()` 等于 A；Timer Command 的 Source 为零值；业务不能覆盖 Source，受控 Redirect 只能按固定 relay 策略转发已有消息。
- Core 内部的 Service、Runtime-owned 和 Timer 投递走不同的显式入口，不以 `nil` 约定 `Source == 0`；后续 Redirect 只能转发 Runtime-owned Message。
- 同一 Service Handler 永不并发；不同 Service 可以并行。
- Mailbox 按成功 admission 顺序 FIFO；多个并发发送者不承诺调用时刻的全局顺序。
- Mailbox 满时快速返回 `ErrMailboxFull`，不扩容、不无限等待。
- Runtime 只复制 `Args` 外层切片；typed client 的引用型 request 快照在各 client 自己的测试中验证。
- OnStart 失败只回滚当前 Service，Runtime 仍保持 Running；ID 不发布、不复用，并由 Runtime 至多调用一次 OnStop 清理部分初始化资源和启动期 Timer。
- CreateService 的 ctx 取消不会强杀 OnStart；Runtime 等待 OnStart 真实返回，只有成功返回才发布 ServiceID。
- OnStart 的 self Send、self Call 和 After 遵守 ADR 0007。
- Stop 与 Send 竞争只有一个 admission 结果：进入 Stopping 前成功入队，或进入 Stopping 后稳定拒绝。
- Stop/Close 幂等，OnStop 最多执行一次；超时 Handler 与 OnStop 不并发；Stop 后不得再次 Start，Close 最终进入 Closed。
- Handler panic 只停止当前 Service，panic 和 stack 只进入本地诊断。
- Service 重建获得新 ID，旧 ID 永远不能命中新 Service。

非目标：Call、PendingCall、Timer 触发、Cluster、Supervisor 和自动恢复。

## Core 2：Timer

状态：已完成当前切片。已落地一次性 After/CancelTimer、FakeClock seam、owner 清理、普通 Mailbox admission 和基础 Timer 诊断计数；Ticker、周期任务和完整 Inspection 仍不在本切片。

实现：

- Runtime 内部 Clock seam 和 FakeClock。
- 一次性 `After` 与 `CancelTimer`。
- TimerID owner 校验和单调耗尽。
- Timer 到期后构造普通 Message，使用与 Send 相同的 Mailbox admission。
- Service Stop、OnStart 失败和 Runtime Close 的 Timer 清理。
- Timer admission 失败的结构化诊断和基础指标。
- 当前实现为每个已 arm Timer 管理一个 Runtime-owned 等待 goroutine；Core4 基准不足时再替换为集中 Timer loop/heap，保持公开语义不变。

验收：

- After 最多触发一次，只通过 Command 影响 Service 状态；到期 Command 的 Source 为 `ServiceID(0)`，创建者由 Timer owner 表达。
- `After` 从调用时开始计时；OnStart 期间到期的 Timer 在 Service 进入 Running 后立即按普通 Mailbox admission 尝试一次。
- 只有创建 Timer 的 Service 可以取消；未知、已取消或已触发 Timer 返回 false。
- CancelTimer、Stop 和触发竞争最多产生一次成功 admission。
- 同时到期 Timer 不承诺彼此顺序，也没有高于普通 Message 的 admission 优先级。
- Mailbox 满、目标 Stopping 和 Runtime Closing 时不重试、不静默丢失诊断。
- 测试使用 FakeClock，不依赖真实 sleep。

非目标：Ticker、周期任务、时间轮和业务 Timeline。

## Core 3：Call、PendingCall 与自动 Reply

状态：已完成当前切片。已落地本地 `Call`、SessionID/PendingCall completion、Runtime 最大等待、调用方 cancellation、停止清理和安全错误转换；Redirect 的 Call session 转移与 Cluster Call 仍不在本切片。

实现：

- SessionID 分配和 PendingCall registry。
- `ServiceContext.Call(context.Context, ...)`。
- Send 和 Call 共用 Message、Dispatcher、Mailbox 和 Handler 路径，只由 SessionID 是否为零区分。
- Handler 返回值或 error 的一次 completion。
- 调用方 context 与 Runtime `MaxCallWait` 的 deadline 合并。
- self Call 拒绝。
- caller Stop、target Stop、Runtime Close、timeout 和 admission 失败的 PendingCall 清理。
- Domain/Runtime StatusError；未知 Handler error 和 panic 的安全转换。

当前实现把 `PendingCall` 作为 Runtime-owned 生命周期组件，关闭顺序位于 Timer 之后、Service admission 之前；`Call` 使用同一 Runtime clock 观察 `MaxCallWait`，以便 FakeClock 在测试中确定性推进超时。

验收：

- Reply 不进入调用方 Mailbox，只通过 PendingCall 恢复原 Handler。
- Call 等待期间当前 Service 不处理下一条 Message，其它 Service 继续运行。
- 调用方没有 deadline 或给出更晚 deadline 时仍受 `MaxCallWait` 限制；更早 deadline 和 cancellation 生效。
- self Call 返回 `ErrCallCycle`；跨 Service 环形 Call 依靠有限 timeout 退出。
- admission 失败、timeout、双方 Stop 和 Runtime Close 都恰好清理一次 PendingCall。
- 未知、重复和迟到 completion 不会完成其它 Session，并产生受控诊断。
- Send 的 Handler error 只进入诊断；Call 的 Handler error 返回稳定 StatusError。
- 未分类 error 转为 `runtime.handler_failed`，panic 转为 `runtime.service_panicked`；原始文本和 stack 不进入 Reply。

非目标：手工 Reply、异步 Reply、无限 Call、全局调用环检测、自动 retry，以及 Redirect 的 Call session/PendingCall 转移。

## Core 4：故障、观测与规模收尾

状态：只读 Inspection 子切片和 1K/10K 空闲 Service 基线已完成。`Runtime.Inspect()` 已提供确定排序的 Service 快照、Mailbox/runner/deadline 状态、PendingCall/Timer 数量和基础故障计数；完整故障分类和 runner/Timer 引擎收尾仍待后续切片。

第一轮 1,000 Service 基线（2026-09-03，Darwin arm64 / Apple M2，`go test ./runtime -run '^$' -bench '^BenchmarkRuntimeCreateServices1000$' -benchtime=1x -benchmem`）：创建与关闭约 1.25 ms，约 1.20 MB、8,093 allocs/op；基线测试观测到 runner goroutine 增量约 1,000、创建后 HeapAlloc 约 1.40 MB。该数据只用于当前机器上的趋势比较，不是跨平台 SLA；是否替换 per-Service runner 仍以目标环境的规模数据和压力模型为准。

第二轮 10,000 Service 基线（同一环境，`go test ./runtime -run '^$' -bench '^BenchmarkRuntimeCreateServices10000$' -benchtime=1x -benchmem`）：创建与关闭约 23 ms，约 11.28 MB、79,016 allocs/op；基线测试观测到 runner goroutine 增量约 10,000、创建后 HeapAlloc 约 11.66 MB，创建约 15.2 ms、关闭约 7.7 ms。该数据只用于当前机器上的趋势比较，不是跨平台 SLA；目前没有数据证明需要替换 per-Service runner。

第一轮 10,000 Timer 基线（同一环境，`go test ./runtime -run '^$' -bench '^BenchmarkRuntimeCreateTimers10000$' -benchtime=1x -benchmem`）：创建与关闭约 22.1 ms，约 12.57 MB、120,073 allocs/op；基线测试观测到等待 goroutine 增量约 10,000，登记约 11.9 ms、关闭约 10.9 ms。此次切片把运行中 `After` 的 arm 从扫描 owner 全集改为直接 arm，避免 O(n²) 登记；当前仍保留一 Timer 一等待 goroutine，集中 Timer loop 需更大目标规模数据再裁决。

第一轮 Hot Service Send 基线（同一环境，`go test ./runtime -run '^$' -bench '^BenchmarkRuntimeSendHotService$' -benchtime=100ms -benchmem`）：约 166 ns/op、211 B/op、3 allocs/op；Mailbox 容量按 benchmark 请求数设置，Handler 使用空实现，计时只覆盖本地 Send admission，不代表业务处理吞吐。

第一轮本地 Call round-trip 基线（同一环境，`go test ./runtime -run '^$' -bench '^BenchmarkRuntimeCallRoundTrip$' -benchtime=100ms -benchmem`）：约 1.53 μs/op、584 B/op、10 allocs/op；计时覆盖 caller Mailbox admission、Call Pending、target Handler 和 completion，不包含 Cluster 编解码或网络延迟。另有 100 个并发 PendingCall 的 Stop 清理门禁测试。

Timer 压力门禁已覆盖 10,000 个 Timer 的半数取消、半数到期和普通 Mailbox admission；测试等待 `Fired` 与 `Admitted` 同时收敛，明确接受 Inspection 的点时快照可能短暂看到中间状态。

关闭收敛门禁已覆盖同一 Runtime 同时存在 PendingCall、Timer 和运行中 Handler 的场景：Stop 先清理等待状态，再等待 Handler 真实返回；Close 完成后 Inspection 必须只报告 `Closed` 和零 Core 残留资源。

实现唯一只读 Inspection，至少包含：

- Runtime 状态。
- Service 状态和 Mailbox 深度。
- runner 是否运行、是否超过 shutdown deadline、真实完成状态。
- PendingCall 和 Timer 数量。
- admission 拒绝、Handler error、panic、Call timeout、Timer 投递失败和 Stop timeout。

建立基准：

- 1、1K、10K 和目标规模空闲 Service 的内存、goroutine 与创建/停止耗时。
- Send 吞吐、Mailbox 等待和热点 Service。
- Call 延迟、timeout 和 PendingCall 压力。
- Timer 创建、取消、触发和清理。
- 大规模 Runtime Close 收敛。

Core v0.1 门禁：

- Core 公开 API、CONTEXT 和 ADR 一致。
- `go test ./...`、`go vet ./...` 和 `go test -race ./...` 通过。
- Stop 后不存在未登记 runner、Timer 或 PendingCall；故意阻塞的业务任务必须作为已知残留被 Inspection 准确报告。
- per-Service runner 的 1K/10K 规模数据已记录。只有数据证明成本不可接受时，才新增 ADR 讨论固定 worker pool 或 continuation。

## Cluster 后续路线

Cluster 是 Core 的调用者和适配层，不进入 Core v0.1。

### Cluster 0：协议与内存传输

状态：Cluster 0 已完成，Cluster 1 正在推进。`cluster` package 已提供 `NodeID`、`ClusterSessionID`、`FrameKind`、`ServiceTarget`、`ClusterEnvelope` 的值类型与结构校验、受限的 `ValueCodec`、启动期不可变的 Command codec registry、长度前缀 `FrameCodec`，以及确定性的 `InMemoryTransport`。Cluster 1 已新增 `NodeSender` 的基础出站 admission；逻辑 `ConnectionAgent` 已具备，TCP stream I/O、连接合并和自动重连仍按后续子切片推进。

- `NodeID`、`ClusterSessionID`、`ClusterEnvelope`。
- 显式 Request、Response、Error frame。
- `StatusError` 的 Class/Code/Message wire 表达。
- 远端地址只使用 NodeID 与 ServiceName/Discovery 或当前远端临时 ServiceID；Runtime 只负责路由、投递和连接恢复，不负责业务所有权、一致性或过期判断。
- `ClusterEnvelope.Validate` 在进入任意 Transport 前检查 frame kind、Session、目标和结构化错误；`Clone` 在 admission 边界隔离 payload 与错误值。
- 默认 `ValueCodec` 只支持 nil、bool、规范化整数/浮点、string、`[]byte`、list 和 string-key map；解码分别得到 `int64`、`uint64`、`float64`，并限制嵌套深度与 payload 大小。
- Command codec registry 在启动期一次性校验、复制并冻结 `CommandID -> CommandCodec` 映射；重复注册、零 CommandID 或 nil codec 直接失败，运行期不提供注册和覆盖。
- 帧长、容器深度和 payload 上限。
- `InMemoryTransport` 以同步回调连接两个节点，作为测试和后续 sender/agent 的可替换边界；本地路径不执行 Cluster 编码。
- `ClusterEnvelope` 不携带业务或 wire version；未来版本兼容由 Transport/Codec 的 framing 或连接握手单独负责。
- `FrameCodec` 使用四字节大端 body length，`MaxFrame` 包含长度前缀；只解析一个完整 frame，不负责 stream I/O 或版本协商。

### Cluster 1：远程 Send

- 状态：进行中（基础子切片已完成）。
- 每个远端 Node 唯一 outbound `NodeSender`，使用固定容量队列和单一 owner worker。
- `Send` 在本地 Clone 后执行非阻塞 admission；队列满返回 `ErrOutboundQueueFull`，nil 只代表本地 Cluster admission 成功。
- admission 后的 Transport error/panic 和关闭丢弃进入有界、非阻塞的 `Failures()` stream；慢消费者不阻塞 sender，溢出由 `DroppedFailureCount()` 计数，事件不携带业务 payload。
- Send 不返回远端 Handler 结果；A 记录自己的 admission/传输诊断，B 记录自己的 Handler error。需要远端处理结果时使用 Call。
- 当前 NodeSender 直接调用注入的 Transport；并发建连合并、TCP stream 和自动重连留在后续 Connection/Transport 子切片。
- 已实现初始化绑定 Source ServiceID、NodeSenderResolver、CommandCodecRegistry、共享 ClusterCallRegistry 和有限 MaxCallWait 的 per-Service ClusterClient；client 不拥有 sender/连接。
- 普通远程 Send/Call 不暴露 source 参数；只有明确的基础设施 relay 才能转发 Runtime-owned Message，且不转移 Call session/PendingCall。
- admission 前的无本地路由/sender、编码失败、队列满和关闭错误由 `Send` 立即返回；admission 后的节点不可达、断线及网络/I/O 错误进入 NodeSender 诊断 stream。

### Cluster 2：远程 Call

- 状态：基础远程 Call、本地等待和内部 TCP response 路由已完成并通过真实回环测试；公开静态出站组装已完成，公开入站最小组装已完成。
- Cluster-owned PendingCall 表保存 `Session → Source、RemoteNode、Target、Command`，Source 只在发起节点本地使用。
- ClusterSessionID 在当前 Cluster 实例内单调分配，耗尽返回 `runtime.ErrIDExhausted`，不回绕或复用。
- Response/Error 只完成匹配的 RemoteNode/Target/Command；未知、迟到、重复或错路由 frame 被拒绝并进入诊断。
- 取消、timeout、所属连接断开或 Cluster 关闭都恰好清理一次 PendingCall。
- 已实现：共享 `ClusterCallRegistry` 的 Session 分配与 `Complete` 入口，以及 `Client.Call` 的请求编码、有限等待、caller cancellation、Response/Error 解码。
- 已实现内部 `connectionCalls`：写入前在共享 registry 同步边界内绑定等待；断线只清理当前连接已绑定 Call，保留底层原因，不影响同节点未绑定或其它连接等待。移除原节点范围清理入口。
- 接线约束：出站写入必须经 `outboundConnection` 绑定，响应读循环使用同一连接 owner；绑定是发送尝试的线性化点，绑定前已取消则不发包，绑定后不承诺撤销或重放。
- 已接通内部响应读循环：通过 streamConnection 读取 Response/Error，并校验连接归属后完成 PendingCall；流终止时清理该连接等待。已用 net.Pipe 验证成功/业务错误回复、远端断线、主动关闭和非法帧长的收尾。
- 异常响应与完整坏帧通过调用方持有的非阻塞诊断通道报告；无消费者或通道满载时丢弃，循环返回丢弃计数。此通道只含节点、Session 和错误，不携带业务载荷。
- 已实现公开静态出站组装：管理目标地址、唯一 sender、响应读循环、按需连接替换和统一诊断。真实 TCP 门禁不要求 NodeID 握手；日志消费由宿主基础设施负责。
- 已实现断线与被拒绝 response 的结构化诊断；正常 Call timeout/cancellation 由调用方处理，迟到 response 可进入诊断。限频与可靠日志不由当前共享队列承诺。
- 已验证：`cluster/roundtrip_test.go` 通过公开 API 组装双节点内存链路，连接 ClusterClient、NodeSender、ConnectionAgent 和 ClusterCallRegistry；请求与回复均经过完整 FrameCodec 编解码。覆盖成功返回、Domain Error 的 Class/Code、安全 Message 和 `errors.Is`，以及未知错误/panic 的安全转换。
- 已验证：Send 的 B 侧 Handler error 由接收适配器本地记录、不产生回复；随后同一链路可继续 Call。目标节点未注册时，Send 入队成功后的失败在 A 侧保留节点、目标、命令和 `ErrNodeNotFound` 原因。
- 门禁范围：InMemoryTransport 同步调用接收器，测试使用逻辑 Handler，没有组装两个 Core Runtime 或真实 socket；不能据此证明慢 Handler 不阻塞同节点后续投递。TCP 调度、连接关闭和 Service 并发隔离仍需在真实 owner 接入时验证。

### Cluster 3：TCP 与连接 owner

- 状态：逻辑 ConnectionAgent、内部单连接 I/O 封装、出站响应读循环、有界并发入站请求循环、单 listener 入站连接 owner 和单目标按需出站连接 owner 已完成；公开静态出站组装已完成，Open 已实现，认证与动态路由仍待后续切片。
- 已裁决并实现基础设施 Runtime.Call：Source 固定为零，经普通 Mailbox 执行，复用有限等待与停止收尾；ServiceContext.Call 仍绑定真实 Service。取舍见 [Cluster 入站 Call 方案](RFC-CLUSTER-INBOUND-CALL.md)。
- 已用 ConnectionAgent 测试适配器接通真实 Runtime，覆盖成功/领域错误回复、零 Source，以及本地 Session 与 ClusterSession 分离；网络入站并发管理和正式适配器组装仍待接入。
- 已实现内部 localRequestAdapter 并接入 inboundConnection：查找 Command codec、解码参数列表、按 ID 或外层名称解析目标，再直接经 Runtime.Send/Call 执行；未知 Command/坏载荷仅返回接收侧诊断错误，不产生响应。Send 接纳错误归接收侧，Call 通过与逻辑 ConnectionAgent 共用的响应函数回包，写回故障保留 I/O 错误链。已删除临时 ConnectionAgent 和闭包错误回传；保留解析后取消检查、编码与写入双重失败原因。适配器不管理并发或连接生命周期，公开 ConnectionAgent 保持不变。
- 入站并发方案已获确认并实现为内部 inboundConnection：单读循环、显式 Call 名额上限、满载暂停读取、顺序 Send、有界非阻塞诊断；详见 [入站并发方案](RFC-CLUSTER-INBOUND-CONCURRENCY.md)。关闭取消基础设施等待并等待连接任务退出，不强杀目标 Handler。
- 已用 net.Pipe 验证慢 Call 隔离、满载背压、连续 Send 顺序、满载时取消、诊断溢出、codec/名称解析 panic 和重复启动拒绝。响应编码失败也返回接收侧诊断；这些测试不等于真实 TCP 或节点级资源上限验证。
- 真实 TCP 回环集成门禁 `tcp_integration_test.go` 已改用内部 `outboundPeer` 与 `tcpListener`，删除预建 socket 和测试专用 Transport，连接 ClusterClient、NodeSender、帧编解码、入站连接、目标 Runtime 和响应读循环。覆盖 Send/Call、领域错误、慢 Call 隔离、断线完成等待和下一 Call 按需恢复；通过 Handler 执行计数验证旧请求不重放。A 不配置 NodeID 或监听，B 接受匿名入站并沿原连接回复。夹具仍负责静态目标与生命周期组装，不代表公开 TCP API、生产认证或路由管理已完成。
- 已实现内部 stream framing 函数：按长度前缀读取单帧，分配前检查上限；覆盖逐字节拆包、连续帧、截断和完整坏帧后的边界保持。写入失败或短写直接返回，不重放。
- stream 解码错误带有内部“完整帧已消费”标记，并保留原错误链，供连接层识别边界；非法长度、截断和 I/O 错误不携带该标记。标记不替代认证、越界等必须关闭连接的检查。
- 已通过 `net.Pipe` 验证 ConnectionAgent 原连接请求/回复、关闭解除读写等待，以及连接 deadline 的超时错误传播；尚未覆盖真实 TCP、连接重建或双 Runtime 调度。
- 内部 `streamConnection` 接管一个已建立的 `net.Conn`：串行写帧、设置有限单次写 deadline、幂等关闭。I/O 写失败和边界不完整的读取失败关闭连接；编码失败不关闭，完整坏帧交给上层执行丢弃/诊断或更严格的安全检查。
- `streamConnection` 不启动 goroutine、不建连或重连；写 timeout 不包含等待串行锁的时间。后续 Transport 必须限制并发写调用数量、拥有唯一读循环并负责关闭后等待退出，尚未接入生产连接。
- 长度前缀 framing。
- 每条 inbound socket 一个 ConnectionAgent，响应沿原连接返回。
- 已确认并清理出站调用与监听身份：删除 LocalNode、入站来源 NodeID；主动 Send/Call 只需目标，open 只决定是否接受新调用。Transport 仅要求 Send(to, envelope)，内存往返测试通过绑定 writer 回复，不注册调用方或反向 Send。见 [连接身份方案](RFC-CLUSTER-CONNECTION-IDENTITY.md)。认证协议另行裁决，不与调用方的可寻址身份绑定。
- 已用真实 TCP 测试夹具验证两个方向分别建立独立连接，关闭 A→B 后 B→A 仍能主动 Call；此门禁验证连接组织，不代表生产节点管理器或公开 open API 已实现。
- 已实现内部 tcpListener：接管一个 TCP listener，限制已接纳连接总数；满额时关闭新 socket，不启动连接任务、不建立用户态等待队列，诊断有界非阻塞。每条已接纳连接使用独立 inboundConnection，来源诊断只附连接远端地址与 Session，不添加 NodeID。
- tcpListener 的取消或异常 Accept 失败会关闭监听和全部已接纳连接、取消基础设施 Call 并等待全部连接处理任务退出；不会强杀目标 Handler。已验证满额隔离、坏帧隔离、慢 Call 期间关闭、空闲 Accept 取消、诊断满载计数与非法配置/重复启动。
- 已确认并实现内部入站 quiesce：关闭监听，所有连接共享请求门禁；完整帧在交给处理任务前决定 admission，停止后暂停读取，已接纳 Call 仍可沿原连接回复。最终取消解除读取、满载名额和停止等待。已补充真实 TCP 的新连接拒绝、新 Send/Call 不执行、旧 Call 回复、并发重复停止、空闲与满载取消测试。公开 Open 和统一入站诊断接线已实现。
- 已确认公开 Open 的资源归属及签名：`Open(target *runtime.Runtime, config ListenConfig) (string, error)` 返回实际监听地址；现有 Cluster 拥有可选单监听及入站连接，但不管理目标 Runtime 生命周期。不支持多监听与热切换，不影响仅出站调用。ListenConfig 的 Address、MaxConnections、MaxConcurrentCalls 和 ResolveService 四个字段已确认。默认连接上限 128、每连接 Call 上限 32 及零值/负数规则已确认。重复调用规则已确认：成功后拒绝重复打开、失败回滚后可重试、并发仅一个打开流程、停止后禁止 Open。停止接线已确认复用 Quiesce 关闭监听和入站门禁、保留连接回复，关闭错误由 Close 合并返回。入站诊断已确认新增 InboundFailure 和 RemoteAddress，共用现有有界出口。已实现公开入站最小切片，见 [可选入站组装](RFC-CLUSTER-OPEN.md)。
- 已确认注册与发现属于外部能力：保留显式地址的 Open，不增加按节点名打开的公共 API；静态表查找和发布地址由应用组装。当前静态出站路由不等于已实现动态发现或地址更新，相关接线留待后续需求。
- 公开入站首步已实现 ListenConfig 类型及内部默认值/校验，覆盖显式配置保留、无解析副作用、非法输入和监听/出站端口零的区别。已接入 Open，覆盖重复打开、失败重试、关闭竞争及入站生命周期/诊断接线。
- 已统一入站诊断出口：listener 与全部连接共享一个有界队列和丢弃计数，删除每连接的 256 容量缓冲与转发 goroutine。单连接退出不关闭出口，listener 在全部生产者退出后关闭；保留解除 Accept/Read 阻塞所需的取消监视机制。
- 当前连接上限属于单 listener owner。公开 open API、多个 listener 共享的节点总配额及生产诊断消费尚未组装，不能据此宣称完整 TCP Transport 已完成。
- 出站连接等待归属已确认并实现内部绑定、响应校验和连接范围清理，真实 TCP 夹具已改用此路径。见 [出站连接与 Call 等待归属方案](RFC-CLUSTER-OUTBOUND-CALL-OWNERSHIP.md)。单目标 Dial 与连接替换由 outboundPeer 接管，静态目标路由由公开 Cluster 组装，动态路由仍待后续切片。
- 按需建连已确认并实现内部 `outboundPeer`：单目标有限 Dial、连接复用、下一请求触发恢复、不重放；关闭先取消 Dial/关闭 socket，再等待发送与唯一响应读循环退出。见 [按需建连方案](RFC-CLUSTER-ON-DEMAND-CONNECT.md)。已覆盖 TCP 回环恢复、失败隔离、取消跳过、半写不重放、Send 诊断及关闭；公开 TCP Transport 与目标路由组装仍待后续切片。
- 已确认易用性方向、`NewCluster(config)`、`Client(source ServiceID)` 两个签名及首轮 ClusterConfig 字段与默认值：显式 Cluster 对象统一出站组装，业务继续使用 source-bound ClusterClient，不手工接线内部组件。见 [公开组装入口方案](RFC-CLUSTER-ASSEMBLY.md)。两阶段关闭已确认，统一诊断入口也已确认，公开出站组装已实现。
- 生命周期已确认，见 [两阶段关闭方案](RFC-CLUSTER-SHUTDOWN.md)：Quiesce 停止接纳，Close(ctx) 最终释放与等待；已绑定 Call 保留回复机会，未发送请求按停止策略收尾。统一诊断入口也已确认，公开出站组装代码与对应生命周期测试已实现。
- 统一诊断出口已确认，见 [诊断方案](RFC-CLUSTER-DIAGNOSTICS.md)：每 Cluster 单一 256 容量的非阻塞 Diagnostics 与丢弃计数，内部直接上报，不执行外部 callback 或创建每目标转发任务。创建/绑定、配置、关闭与诊断契约均已收口，已实现最小公开出站组装及其测试；Open、认证和动态路由不纳入本轮。
- NodeSender 自动重连，但不自动重放已经失败或结果不确定的请求。

### Cluster 4：故障与资源收尾

- 已补公开 Open 的真实 TCP 残帧回归：对端在长度前缀或正文未发送完整时半关闭写端，服务端以 UnexpectedEOF 诊断并关闭连接；残帧不进入 Handler，不提取未经验证的 Session/Target/Command，不纳入可继续读取的坏帧去重；新连接上的合法 Call 正常完成。

- 已补充公开 Open 的真实 TCP 协议故障回归：完整坏载荷丢弃后同连接继续处理合法 Call；超限长度只发送四字节前缀即断连，不等待正文、不填充不可信请求关联字段，且监听继续服务其它连接。新增双连接日志回归：每连接同类坏帧只保留首条，合法 Call 继续完成，Cluster.Close 后各连接输出一条抑制数量汇总；Diagnostics 和 JSON 日志同时验证，关闭后累计计数仍可读取。这些测试不代表认证或全部故障注入门禁已经完成。

- FaultTransport 注入半写入、断线、重复响应、损坏帧和出站队列满。
- 边界可信的完整帧格式错误、未知 Command、载荷解码失败和错误入站请求类型丢弃；同连接同类只记录首条，重复计数并在连接结束汇总。当前 wire 不包含 Version 字段。
- 非法长度、越界、认证失败或无法恢复帧边界时关闭连接。
- 双节点集成、race、关闭和连接泄漏门禁。

## Core 之外：日志输出

- 已补 Runtime 日志故障隔离门禁：日志阻塞满载和已失败时，业务成功/领域错误保持原结果，独立诊断可用；Runtime 关闭不等待或关闭借用 Logger。此测试验证基础设施隔离，不承诺业务主动等待或返回日志错误后仍不受影响。

- 已提供 [最小双进程示例](../examples/twonode/README.md)：标准库 JSON 配置、独立日志目录、Service 绑定 ClusterClient、Timer 驱动新操作、名称解析和有序退出；不引入通用配置框架，不新增 Core/Cluster API。显式开启 GSR_PROCESS_TEST=1 可运行跨进程正常停机及 SIGKILL 强杀后的重启恢复门禁，验证调用端继续运行、后续新请求恢复、Send 到达、演练中无重复投递及正常退出记录；强杀进程不要求收尾日志。当前强杀发生在首轮调用完成后，不代表在途执行、持久性、网络分区或跨机器故障均已覆盖。

- 已补输出器生命周期门禁：短写终止、首次错误保留、失败后跳过 Flush、Flush error/panic、重复关闭幂等与借用 Writer 不被关闭。此测试不代表 fsync 或 SLS 可靠送达。

统一 FIFO 日志入口、单写入者及 Runtime/Cluster 直接接线已实现；不再通过 Diagnostics 转发主日志。已覆盖并发顺序、记录处时间位置、字段快照、满载、关闭和输出失败。默认终端，可借用 Writer；Config.Directory 已支持文件输出，与 Writer 互斥。固定 100 MiB、10 个历史文件，由现有写入者滚动且不重置 seq，不拆记录、不压缩；不同启动配置显式指定独占相对目录，在 New 时解析并固定。首次输出失败后停止写入和接纳、残留记录计数，Err() 和 Close 返回首次错误，内部文件由 Logger 最终释放。已补文件滚动、历史保留、跨文件序号及失败状态测试。Cluster 同连接同类坏帧只记录首条、重复抑制计数与关闭汇总已实现，不增加定时任务，不抑制业务日志、投递失败或断线诊断。SLS 由外部采集器接入，见 [日志方案](RFC-LOGGING.md)。

## Core 之外：WebSocket Gate 联调示例

状态：DebugGate 的 Go/TypeScript Echo 往返、跨语言认证固定向量、生产 WebSocket Gate（支持 Ingress 终止 TLS 与可选直连 TLS）首个纵向切片和 Runtime 共享 Logger 接线已完成；单/多慢客户端、混合连接和部分失败阶段已有可重复验收，目标部署容量与完整失败矩阵仍待验证，不提前加入 Core 或通用测试框架。

- 在 `examples/websocket-gate/` 提供一个 GSR Go 服务端和一个命令行 TypeScript 客户端，用于验证真实 WebSocket 接入、Envelope 编解码、request_id 关联、业务 Response、业务拒绝、ErrorResponse、服务端 Push 和连接关闭。
- 示例以 `proto/` 下的通用协议和业务协议 `.proto` 作为协议源，使用 Buf CLI 的单个 `buf generate` 配置调用本地 `protoc-gen-go`、`protoc-gen-es` 和 `protoc-gen-gsr-registry`，分别生成 Go/TypeScript 消息及 Go 协议目录。每个 MsgId 的 payload、方向和 Request/Notify/Response/Push 种类都显式定义；业务 Handler 按已实现消息逐步手动注册。客户端使用 `@bufbuild/protobuf` 的类型化消息和二进制 codec，不手写 wire 编码；示例锁定 generator/runtime 版本并跟踪生成源码。
- `examples/websocket-gate` 已覆盖 DebugGate 的内网请求/响应闭环、request_id 回传、连接数量限制、断开通知和 Runtime 关闭回收；Go 服务端与 TypeScript CLI 共用通用协议和 Echo 业务 `.proto`，由 Buf 生成两侧类型及 Go 协议目录，Agent 手动注册 Echo 强类型 Handler，并完成实际 WebSocket 往返。协议目录还提供通用错误回复、Request 必须回复与 Notify 不回复的约束。它只在示例层显式注入底层 Runtime，不改 `gsr.Run` 或业务 `ServiceContext`；跨语言认证固定向量、首帧握手解析、服务端临时密钥派生和生产 WebSocket Listener 接线现已完成，覆盖可选直连 TLS 与 Ingress 终止 TLS 的普通 HTTP WebSocket。
- 固定向量位于 `examples/websocket-gate/testdata/client-auth-vector.json`，Go 与 TypeScript 测试直接读取同一夹具，覆盖双向 X25519 结果、HKDF 中间值、含高位的 seq 大端编码、原始 Envelope 字节 MAC，以及修改 seq/Envelope/MAC、错误 info、公钥顺序与错误截断长度。
- `examples/websocket-gate/prodgate/` 已实现并测试逐连接握手、HKDF 密钥派生、常量时间 MAC 校验、严格递增 seq、坏 MAC 不推进序号和帧长度上限；生产 `GateService` 已接入可选直连 WSS/TLS 或 Ingress 后 HTTP WebSocket Listener、5 秒默认首帧握手期限、TCP accept 边界连接上限、有界出站队列、唯一 WebSocket writer，以及认证后直接投递 Agent 的路径。直连 TLS、Ingress 终止 TLS、握手与业务帧往返、错误 MAC、握手超时、握手前连接容量和关闭回收已有本机集成测试。
- 示例 Gate 已使用 `NewAgent func(ClientSession) runtime.Service` 回调；新连接和断开事件由 GateService runner 管理，创建返回的 AgentID 绑定到该客户端连接。读循环对每帧直接调用 `Runtime.DeliverClientMessage`，不经 Gate Mailbox/Core Command 转发；不新增 AgentFactory、Service 管理器或通用依赖注入层。
- DebugGate 与生产 Gate 均限制最大连接数、单帧大小和每连接出站队列；生产 Gate 的连接限制在 TCP accept 边界生效，覆盖尚未完成 TLS/HTTP/WebSocket 握手的连接。出站按序列化帧字节和每项 64 字节队列开销计费，入站复用 Agent Mailbox，不建立无界中间队列。生产 Gate 六类容量计数和 Runtime 共享 Logger 接线已实现；同一连接同一错误类别只记录首条，关闭时补充抑制汇总。连接关闭、正常断开、Agent Mailbox 满载后的有限重试、单/多慢客户端背压及健康连接隔离、短时多连接并发、带“发送数等于处理数”断言的显式 `GSR_GATE_STRESS=1` 混合连接压力、一次 2 分钟多连接样本、可调时长的慢客户端压力以及坏 TLS/应用握手/MAC 后成功恢复连接的失败阶段样本已有集成覆盖；更长部署时长、客户端业务超时与其它失败阶段的组合压力仍待验证。不等待或强杀 Agent。
- 业务拒绝由业务 Response 表达，只有没有正常业务结果时才发送 ErrorResponse；客户端只按稳定错误码分支，不依赖 message 文本。
- 示例只负责可重复联调；固定向量和显式 Gate 压力开关是独立测试夹具，不替代后续的慢客户端背压、断线恢复和长期容量测试。

## Core 之外的能力候选：反向代理与网关

状态：已记录，暂不进入 Core v0.1，也不提前实现通用网关框架。

`examples/http-outbound` 与 `examples/http-inbound` 分别验证 Service 内有界 HTTP 出站调用、HTTP 基础设施经有限 `Runtime.Call` 查询本地 Service；后者只是无认证的只读联调示例，不是通用反向代理或新的客户端协议。

- L4/TCP 网关属于 Transport/ConnectionAgent 层的字节转发能力，不进入 Core Message，也不使用业务 Redirect。
- L7/HTTP 反向代理属于 Core 之外的 Adapter/业务能力：解析并限制请求后，以 typed request 调用本地或远端 Service，使用新的 Call-through 获取 typed response，再由连接 owner 写回客户端。
- 普通 HTTP 请求/响应不使用 Redirect 转移 Call 的 Session 或 PendingCall；Redirect 只保留给受信任的单向基础设施 relay。
- 外部客户端身份、连接信息、RequestID 和 TraceID 使用显式请求字段或 Transport metadata，不伪装成 Core Source；后端看到的普通 Source 是 Gateway ServiceID。
- 路由解析、HTTP 状态映射、timeout、retry、幂等和客户端断开后的迟到结果处理由网关拥有；Runtime 只负责消息投递和连接恢复。
- 第一版网关若需要并发，优先使用多个 Gateway shard；高并发的 Send + Result Command 方案另行评估，不为此改变 Core 的 run-to-completion 语义。

## 旧参考项目的使用方式

`../GameServiceRuntime` 与 `../skynet` 始终只读。

从旧 GameServiceRuntime 吸收问题和黑盒测试场景：

- Send/Stop admission 竞争。
- 并发 Stop/Close 和 OnStart/Runtime Close 竞争。
- PendingCall 的 admission 失败、timeout、重复、迟到和停止清理。
- Timer 与目标停止竞争。
- panic、内部 error 脱敏和任务真实返回。
- Registry、Mailbox、Timer、PendingCall、指标标签和 goroutine 泄漏。

不带入旧实现形状：

- 额外地址包装类型、Node 嵌入 Core 地址和兼容别名。
- 固定 worker、ReadyQueue 和 permit 调度结构。
- 手工 `Reply()`、异步 Reply 和旧 Service 四钩子接口。
- `Payload` 兼容字段、可组合 Cluster codec 和公开远端解析。
- Cluster、Discovery、Snapshot、Supervisor、Runner 或业务类型进入 Core。

有意吸收旧项目的 Command 设计：每个应用 Service 的不可变 typed `gsr.Dispatcher`、构造期签名校验，以及未知命令进入 Handler 的行为；但按新契约改为 Handler 返回值自动 Reply，不保留旧 Dispatcher 的 `CommandContext.Reply()` 调用。

从 Skynet 优先参考其实际 API 和运行模型：

- `self/send/call/timeout/newservice/dispatch` 的简单动词和数字地址。
- source、session、每 Service mailbox、消息调度、Timer 投递消息和 Cluster sender/agent 方向性组织。
- 对依赖 Lua coroutine、PTYPE、C 引用计数、模块加载和历史 Harbor 的机制，只提炼目的，不逐字移植。

## 测试夹具

- FakeClock：确定性推进 Timer 和 Call timeout，不使用真实 sleep。
- ProbeService：记录 Source、执行顺序和同时运行的 Handler 数。
- BlockingService：在 OnStart、Handler、Call 和 OnStop 阶段受测试控制地阻塞和恢复。
- InMemoryTransport：单进程连接两个 Runtime，不依赖 TCP。
- FaultTransport：注入网络边界故障。
- Leak/Race harness：核对 runner、Timer、PendingCall、连接和 goroutine owner。

## 第一轮任务

1. 初始化 Go module 和最小目录结构。
2. 先为 ID 分配器、StatusError、Config 和 Runtime 空生命周期写失败测试。
3. 完成 Core 0 的最小实现。
4. 运行受影响 package 的格式、编译、测试和 vet。
5. Core 0 收尾后运行全仓 test/vet；其代码不包含并发 owner 时不机械运行 race。
6. 以中文提交一份可验证切片，例如“实现 Core 基础类型与 Runtime 生命周期骨架”。
7. Core 1 完成后进入 Timer，再进入 Call；每个切片只保留已经有测试和调用者的能力，不预建 Cluster 抽象。

## 通用阶段门禁

- 新行为先写可失败测试，再完成最小实现。
- 开发循环运行相关测试；package 收尾运行 package 全测和 vet。
- 功能切片收尾运行全仓 test/vet；涉及 goroutine、生命周期、锁、channel、Timer、Scheduler、Transport 或共享状态时运行 race。
- 测试默认使用 fake、probe 和受控阻塞点，不用反复 sleep 掩盖竞态。
- 测试保护公开行为、owner 和失败语义，不依赖无必要的私有字段或 goroutine 数量细节。

## 暂不进入第一版

Discovery、服务迁移、自动业务 retry、Snapshot、监督树、动态负载均衡、多 codec 协商和 durable delivery。
