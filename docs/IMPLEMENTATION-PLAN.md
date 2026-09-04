# GSR Core 与 Cluster 实施路线图

本文把已接受的 API 和 ADR 拆成可验证的纵向切片。每个切片只引入完成当前运行链所需的能力；Core 完成后再开始 Cluster。

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
- Runtime 不提供 Bootstrap Service；`Runtime.Start()` 只启动基础设施，应用层在 Running 后逐个创建 Service 实例。
- `CreateService` 固定为 `CreateService(ctx context.Context, service Service) (ServiceID, error)`；Service 由业务层构造，Runtime 接管其生命周期。
- `Service.OnStart` 固定接收 `context.Context` 和 `ServiceContext`；CreateService 的 context 是协作取消信号，并传给 OnStart。
- OnStart 失败只回滚当前 Service，Runtime 保持 Running，可以继续创建其它 Service；失败 ID 不发布且永不复用，并由 Runtime 至多调用一次 OnStop 做部分初始化清理。
- Runtime 生命周期固定为 `Created → Starting → Running → Stopping → Stopped → Closed`；`Stop(ctx)` 负责有序停止，`Close()` 负责最终资源释放，Stop 后不得再次 Start。
- Core admission 复制 `Args` 外层切片；引用型参数由 typed client 按消息类型构造快照。
- Handler 使用只读 `CommandContext.Source()` 获取 Runtime 填入的来源；普通 Send/Call 为当前本地 Service，Timer Command 为 `ServiceID(0)`。
- 业务 API 不接收任意 source 参数；Redirect/Relay 只作为明确受信任的 Runtime/Adapter 能力转发 Runtime-owned Message，首版不进入 `ServiceContext`，也不转移 Call 的 PendingCall/Session。
- Handler 通过返回值自动 Reply，不公开 `Reply()` 或异步 response API。
- `Service.Handle` 是 Runtime 唯一业务入口；Core 提供可选的每 Service 不可变 `CommandDispatcher`，不提供 Runtime 命令注册表或 per-Service 命令预检查。
- `CommandDispatcher` 构造时复制并校验 `CommandID -> typed Handler` 表，运行时固定检查参数数量/类型；未知命令仍进入 Handler，命令错误使用稳定 ErrorClass/ErrorCode。
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
- `Runtime.CreateService(ctx context.Context, service Service) (ServiceID, error)`；Service 由应用层传入实例，不提供工厂或 Bootstrap Service。
- Service registry 和不可复用 ServiceID。
- 有界 Mailbox 和非无限阻塞 admission。
- per-Service runner；Scheduler 负责 runner 创建、panic recovery、停止、等待和观测，不引入全局 ReadyQueue 或固定 worker pool。
- 将 Service 管理和 Scheduler 按 Core 组件生命周期计划接入 Runtime；不得在 `Close` 中追加匿名清理逻辑。
- 唯一的本地 Message 和 LocalDispatcher。
- 可选的 per-Service `CommandDispatcher`、typed Handler 参数绑定和构造期命令表校验；不实现全局 `RegisterCommand` 或 Runtime 命令白名单。
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

状态：Cluster 0 已完成，Cluster 1 正在推进。`cluster` package 已提供 `NodeID`、`ClusterSessionID`、`FrameKind`、`ServiceTarget`、`ClusterEnvelope` 的值类型与结构校验、受限的 `ValueCodec`、启动期不可变的 Command codec registry、长度前缀 `FrameCodec`，以及确定性的 `InMemoryTransport`。Cluster 1 已新增 `NodeSender` 的基础出站 admission；ConnectionAgent、TCP stream I/O、连接合并和自动重连仍按后续子切片推进。

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
- 已实现初始化绑定 Source ServiceID、NodeSenderResolver 和 CommandCodecRegistry 的 per-Service ClusterClient；当前只负责参数编码与远程 Send admission，不拥有 sender/连接。
- 普通远程 Send/Call 不暴露 source 参数；只有明确的基础设施 relay 才能转发 Runtime-owned Message，且不转移 Call session/PendingCall。
- admission 前的无本地路由/sender、编码失败、队列满和关闭错误由 `Send` 立即返回；admission 后的节点不可达、断线及网络/I/O 错误进入 NodeSender 诊断 stream。

### Cluster 2：远程 Call

- Cluster PendingCall 和 ClusterSessionID。
- Response/Error frame completion。
- 断线、timeout、未知、重复和迟到 response。
- Domain Error 经 Cluster 往返后保持 Class/Code 和 `errors.Is` 语义。

### Cluster 3：TCP 与连接 owner

- 长度前缀 framing。
- 每条 inbound socket 一个 ConnectionAgent，响应沿原连接返回。
- 可选 NodeID/shared-token 握手。
- NodeSender 自动重连，但不自动重放已经失败或结果不确定的请求。

### Cluster 4：故障与资源收尾

- FaultTransport 注入半写入、断线、重复响应、损坏帧和出站队列满。
- 边界可信的未知 framing version、未知 Command 和解码失败帧丢弃并限频诊断。
- 非法长度、越界、认证失败或无法恢复帧边界时关闭连接。
- 双节点集成、race、关闭和连接泄漏门禁。

## Core 之外的能力候选：反向代理与网关

状态：已记录，暂不进入 Core v0.1，也不提前实现通用网关框架。

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

有意吸收旧项目的 Command 设计：每个 Service 的不可变 typed `CommandDispatcher`、构造期签名校验，以及未知命令进入 Handler 的行为；但按新契约改为 Handler 返回值自动 Reply，不保留旧 Dispatcher 的 `CommandContext.Reply()` 调用。

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
