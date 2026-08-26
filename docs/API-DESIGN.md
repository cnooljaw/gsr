# GSR API 设计规范

GSR 学习 Skynet 的核心设计方法：Service 使用 Runtime 内的局部数字地址，本地 Send 与 Call 共用消息管道，Cluster 在 Core 之外完成节点路由和网络编码。GSR 不复制 Lua 的动态类型，而是采用“Skynet 语义，Go 类型系统”。

## 1. 最小 Core

GSR Core 只建立以下模型：

```text
Runtime
Service
ServiceID
Command
Mailbox
Scheduler
Timer
```

其中 `Mailbox` 和 `Scheduler` 是 Core 内部一级概念，不是业务扩展接口。Cluster、Discovery、Snapshot、Diagnostic、AI Runner 等能力建立在 Core 之上，不得反向要求 Core 理解节点发现、序列化、恢复策略或业务领域。

公开 API 优先围绕以下稳定动词设计：

```text
Start
CreateService
Self
Send
Call
After
CancelTimer
Stop
Close
```

这些动词保留 Skynet API 的直接表达，但不复制其 Lua/C 执行机制：

| Skynet | GSR | 关键差异 |
| --- | --- | --- |
| `skynet.start()` | `Runtime.Start()` 与 `Service.OnStart()` | Runtime 启动基础设施，Service 启动失败显式返回 error。 |
| `skynet.self()` | `ServiceContext.Self()` | 返回 Runtime 局部 `ServiceID`。 |
| `skynet.send()` | `ServiceContext.Send()` | Source 由当前 Service 自动绑定；admission 有界。 |
| `skynet.call()` | `ServiceContext.Call(ctx, ...)` | 保持 run-to-completion，并强制受 Runtime 最大等待时间约束。 |
| `skynet.timeout()` | `ServiceContext.After()` | Timer 只投递 Command，不执行业务回调。 |
| `skynet.newservice()` | `Runtime.CreateService()` | 使用 Go Service 接口和显式启动错误。 |
| `skynet.dispatch()` | `Service.Handle()` | Source 通过只读 `CommandContext` 提供。 |
| `skynet.ret()/response()` | Handler 返回值 | GSR 不暴露手工或异步 Reply API。 |
| `skynet.exit()/kill()` | `Runtime.Stop()/Close()` | 使用幂等、可等待且有 deadline 的生命周期操作。 |

Skynet 的接口形状是 API 设计的重要参照，但 GSR 的具体契约由 Go 的类型系统、错误模型、消息所有权和生命周期约束决定。

- 不使用 `Actor`、`Spawn`、`Ask`、`Tell` 替代 GSR 术语。
- 不向业务 API 暴露 Skynet `PTYPE`、Mailbox、Session table、Scheduler queue 或 Transport frame。
- 不用一个不断增长的 `RuntimeContext` 承载 Cluster、Discovery、数据库、配置和其它外层能力。

## 2. Runtime ID

Core 与 Cluster 的运行期 ID 使用简单标量：

```go
type ServiceID        uint64
type SessionID        uint64
type TimerID          uint64
type ClusterSessionID uint64
```

`ServiceID` 的契约是：

- 只标识当前 Runtime 生命周期内的一个 Service。
- 不包含 `NodeID`、Generation、Epoch 或 Cluster 路由信息。
- 在同一个 Runtime 生命周期内已经发放的 ID 不复用；计数耗尽时拒绝创建，不回绕复用。
- Service 停止后，旧 ID 投递失败；重建 Service 获得新 ID。
- Runtime 关闭后，其中全部 ServiceID 失效。
- ServiceID 不持久化到数据库、Snapshot、长期配置或跨重启缓存。
- 稳定逻辑身份使用 `ServiceName` 或领域 ID，并由 Core 之外的 Registry/Discovery 解析。

不要再定义只包装 `ServiceID` 的 `ServiceRef`。除非未来出现当前模型无法表达的真实需求，否则不把全局网络身份提前加入 Core。

所有运行期 ID 统一遵守：

- 零值保留；`SessionID(0)` 和 `ClusterSessionID(0)` 专门表示 Send、不等待响应，其它 ID 的零值无效。
- 在各自 owner 生命周期内从 1 开始使用 uint64 单调分配，不随机、不扫描空洞、不复用。
- 分配器到达 `math.MaxUint64` 后永久返回 `ErrIDExhausted`，不得让计数器回绕到零或重新从 1 分配。
- ID 不跨 Runtime/Cluster 实例重启持久化；实例重启会使旧 Service、PendingCall、Timer 和连接全部失效，新实例可以重新从 1 开始。
- 测试通过注入接近上限的分配器状态验证耗尽行为，不实际循环分配海量 ID。

## 3. Service 单线程不变量

> 对同一个 Service，Runtime 在任意时刻最多执行一个 Mailbox Handler。Service 拥有的可变状态只能在 Handler 中修改。

异步流程固定为：

```text
Mailbox
  → Handler
  → 复制不可变输入
  → 外部异步任务
  → Result Command
  → Mailbox
  → 校验业务 Revision / OperationID
  → 修改状态
```

- goroutine 不得持有 `*Service` 并修改状态。
- goroutine 不得在 Handler 外使用保存的 `ServiceContext` 或 `CommandContext`。
- Service 停止后，发送给旧 ServiceID 的异步结果必须投递失败。
- 同一 Service 实例中的迟到结果由领域 `Revision`、`OperationID` 或 RequestID fencing；Core 不理解通用业务 Revision。

第一版采用 Runtime 管理的 per-Service runner goroutine：

- 每个 Running Service 有一个 runner，串行消费自己的 Mailbox。
- runner 的创建、取消、panic recovery 和等待全部由 Runtime/Scheduler 所有，业务不得接触。
- Handler 内 Call 只阻塞当前 Service runner，其它 Service runner 可以继续运行。
- 第一版不引入固定 worker pool、协程 continuation 或 Handler 重入；未来只有在规模基准证明 runner 成本不可接受时才重新裁决。

## 4. 本地消息管道

Core 内部只有一种未编码的本地消息模型：

```go
type Message struct {
	Source  ServiceID
	Target  ServiceID
	Session SessionID
	Command CommandID
	Args    []any
}
```

`Message` 是 Runtime 内存对象，不是网络包，不执行 protobuf/TCP 编码。Send 与 Call 不是两种 Message 或两个 Dispatcher：

```text
SessionID == 0  → Send，不需要 Reply
SessionID > 0   → Call，等待一次 Reply
```

公开 API 可以保留 `Send` 和 `Call` 两个符合调用意图的便利入口，但它们必须进入同一个本地 dispatch 实现：

- `Send` 使用 `SessionID(0)` 并在消息被投递层接受后返回。
- `Call` 分配非零 SessionID，通过同一管道投递并等待 Reply。
- Send/Call 请求共用 Message；Reply 不是业务 Command，也不进入调用方的 Mailbox 或 Handler。
- Runtime 以 SessionID 查询内部 `PendingCall`，通过独立的 completion 路径完成一次 Reply 或 Runtime Error。
- 不为本地 Send/Call 分别实现 mailbox、scheduler 或 local dispatcher。

### 4.1 Args 的所有权

未编码不代表可以共享可变状态。Runtime 接受消息时浅拷贝最外层 `Args` 切片；这只隔离参数列表本身，不递归复制 `any` 内部的数据。对每个参数遵守以下契约：

- 优先传值语义的不可变值；map、slice、pointer 等引用型参数必须是本次消息专用的输入快照。
- 消息成功被 Send/Call/After 接受后，引用型参数的所有权转移给 Message；发送方不得继续修改它。
- 禁止把 Service 拥有的可变状态或其内部引用直接放入 Args。
- Handler 将收到的参数视为只读输入；需要长期保存或修改时，先复制到自己拥有的状态。
- typed client 负责按具体消息类型决定复制深度、创建请求快照，并隐藏 `[]any` 的装配与类型检查。对使用者表现为传值语义，但该保证由 typed client 而不是通用 Core 反射复制实现。
- 可能走 Cluster 的 Command，其逻辑参数还必须能够被对应 Cluster codec 编码；Core 本地路径仍不执行该编码。

Runtime 不对任意 `any` 做递归反射复制。递归复制既不能可靠表达资源所有权，也会给本地热路径增加不可预测成本。

### 4.2 Call 的 run-to-completion 语义

Service 内 `Call` 采用同步等待语义，但不允许同一 Service 重入：

```text
Handler A
  → Call(B)
  → 保留 A 的 Service 执行权，挂起当前 Handler
  → B 和其它 Service 继续运行
  → Reply 通过 PendingCall 唤醒 A
  → A 的同一个 Handler 继续执行
  → Handler 返回后，A 才能处理下一条消息
```

- 等待 Reply 时当前 Service runner 保持阻塞且不处理下一条消息；其它 Service runner 不受影响。
- Reply、目标停止、调用方停止、Runtime 关闭或 Call 超时都会结束等待；pending 记录必须恰好清理一次。
- 第一版拒绝对自身 ServiceID 的 Call，并返回 `ErrCallCycle`。
- 跨 Service 的环形 Call 可能互相等待，属于必须通过设计和测试避免的程序错误；Call 应谨慎使用，默认优先 Send + Result Command。
- Runtime Config 必须提供有限且大于零的 `MaxCallWait`。每次 Call 必须接收非 nil `context.Context`，实际 deadline 取调用方 deadline 与 `now + MaxCallWait` 中更早者；调用方没有 deadline、deadline 更晚或使用 `context.Background()` 时，仍受 `MaxCallWait` 限制。

### 4.3 Handler 与 Reply

第一版使用 Go 返回值表达 Reply，不向业务暴露需要手工配对的 `Reply` 方法。Runtime 为每条消息创建只读的 `CommandContext`：

```go
type CommandContext interface {
	ServiceContext
	Source() ServiceID
}

type Handler interface {
	Handle(CommandContext, CommandID, ...any) (any, error)
}
```

- `Source()` 由 Runtime 从当前 Message 填入，业务只能读取，不能覆盖或伪造。
- `CommandContext` 只在当前 Handler 返回前有效，不得保存到 Service 字段或交给异步任务。
- SessionID 为零时，Runtime 不创建 Reply；Handler 返回的非 nil error 进入结构化诊断。
- SessionID 非零时，Runtime 把 Handler 的返回值或错误转换为一次 Reply completion。
- Handler 返回后不能异步 Reply；Runtime 负责防止重复 completion，并清理 PendingCall。
- typed client 负责把 `any` 结果转换为明确的领域结果类型。

Source、Call 上限和自动 Reply 的完整取舍见 [ADR 0007](adr/0007-command-context-call-and-timeout-stop.md)。

## 5. Source 语义

- Service 发起 Send 或 Call 时，`Message.Source` 必须是当前 Service 的 `ServiceID`。
- 业务调用者不能伪造 Source。
- 普通 Send/Call 必须从受 Runtime 管理的 Service 上下文发起。
- 外部连接、网关和 Cluster relay 要进入 Core 时，应先由对应的本地 Adapter Service 发起投递；`Source` 是该次本地投递的发起 ServiceID。
- 跨节点原始来源、连接、trace 和认证信息属于 ClusterEnvelope/Transport metadata，不得塞入 Core Message 或 `ServiceID`。

这里的 Source 表示“当前本地消息跳的发起者”，不是全局可寻址身份。Cluster 转发时，远端目标看到的是远端本地 relay/adapter 发起的本地投递；端到端请求由 Cluster session 和传输上下文关联。

## 6. Local 与 Cluster

统一的是编程模型，不是执行路径。

### 6.1 本地路径

```text
Service.Send(target ServiceID, command, args...)
  → construct local Message in memory
  → LocalDispatcher
  → target Mailbox
```

- 不经过 protobuf/TCP 序列化。
- 不依赖 Cluster、NodeID 或 Discovery。
- LocalDispatcher 只查询当前 Runtime 的 Service registry。

### 6.2 Cluster 路径

业务 API 采用 Skynet 风格，不显式传 Source：

```go
clusterClient.Send(node, serviceID, command, args...)
clusterClient.Call(node, serviceID, command, args...)
```

Go 没有可靠的“当前 Service”隐式上下文。Cluster 内部采用显式 Source 的实现入口，但只向业务暴露预先绑定当前 ServiceID 的窄 client：

```go
// Cluster 内部入口，不向业务包暴露。
sendFrom(source, node, target, command, args...)
callFrom(source, node, target, command, args...)

// Service 初始化时绑定一次 source，业务调用时无需重复传入。
clusterClient := cluster.Bind(serviceContext.Self(), dispatcher)
```

禁止使用包级全局变量、goroutine-local 或调用栈猜测来寻找“当前 ServiceID”。绑定后的 client 不允许业务覆盖 Source。

执行路径：

```text
origin Service
  → local ClusterSender/Transport
  → encode ClusterEnvelope
  → wire
  → remote ClusterAgent/Transport
  → decode ClusterEnvelope
  → construct local Message in memory
  → remote LocalDispatcher
  → remote ServiceID
```

- `NodeID` 由 Cluster 用来选择连接，不进入 Core ServiceID。
- 网络包中的目标 ServiceID 是远端 Runtime 的本地地址。
- ClusterEnvelope 是 Cluster/Transport 自己的编码结构，不复用 Core Message 作为 wire schema。
- ClusterEnvelope 必须显式携带 request、response 或 error 的 frame kind；Session 只用于关联，不能单独判断消息方向。
- 远端 relay 把 ClusterEnvelope 解码为新的本地 Message，再通过 LocalDispatcher 投递。
- ClusterSessionID 由 ClusterSender/ClusterAgent 管理，不要求与本地 Core SessionID 相同。
- 本地消息不创建 ClusterEnvelope，也不为了统一表象执行远程序列化。
- 数字形式的远端 ServiceID 只在当前远端 Runtime/连接生命周期内有效；断线重连后必须通过外层名称或 Discovery 重新解析。
- `clusterClient.Send` 返回 `error`。返回 nil 只表示请求已被本地 Cluster 边界接受或入队，不表示远端 Handler 已执行，更不表示持久化成功。
- 无路由、已知断连、编码失败、出站队列满或关闭必须返回 Runtime Error；需要确认业务结果时使用 Call，并为有歧义的网络失败设计 RequestID 与幂等。

### 6.3 Cluster wire 与连接模型

ClusterEnvelope 至少编码以下逻辑字段：

```text
ProtocolVersion
FrameKind: Request / Response / Error
ClusterSessionID
Target ServiceID
CommandID
Payload or ErrorClass/ErrorCode/Message
```

Error frame 实际携带 `ErrorClass + ErrorCode + Message`；Message 只用于安全展示或诊断，不能参与程序分支。

- 默认 codec 支持 nil、bool、有符号/无符号整数、浮点数、string、`[]byte`，以及由这些值递归组成的 list 和 string-key map。
- 领域 struct 通过启动阶段注册的 Command codec 转换；不直接编码任意 pointer、func、chan、unsafe pointer、循环引用或进程内资源句柄。
- codec 与 Command 在启动阶段注册；重复注册属于 programmer error，允许 panic 并阻止 Runtime/Cluster 启动。
- 能够确认完整帧边界的未知版本、未知 Command 或 payload 解码失败帧直接丢弃，并记录限频诊断与指标。
- 非法长度、超过最大帧、无法确认下一帧边界或认证失败不能安全丢弃，必须关闭该连接。

连接组织参考 Skynet：

- 每个远端 Node 只有一个 outbound NodeSender；并发建连请求合并到该 sender，不创建多个竞争 sender。
- 每条 inbound socket 由独立 ConnectionAgent 读取和响应，不要求把两个方向合并成一条全双工连接。
- outbound 通道断开后自动重连，但已经失败或结果不确定的业务请求不自动重放。
- NodeID 握手在可信内网可关闭；启用时使用 NodeID 加共享 token 的简单校验。认证配置属于 Cluster，不进入 Core。
- 对同一 Node 的“重复连接胜负”不单独建模：outbound 由唯一 NodeSender 保证唯一；inbound agent 按连接独立存在，响应关联保持在原连接内。

## 7. ServiceContext

`ServiceContext` 只能暴露当前 Service 使用 Core 所需的最小能力，例如：

```go
type ServiceContext interface {
	Self() ServiceID
	Send(ServiceID, CommandID, ...any) error
	Call(context.Context, ServiceID, CommandID, ...any) (any, error)
	After(time.Duration, CommandID, ...any) (TimerID, error)
	CancelTimer(TimerID) bool
}
```

- `Self()` 返回当前 ServiceID。
- `Send`、`Call` 和 `After` 由 Runtime 自动使用 `Self()` 作为 Source。
- `Call` 的 context 只能取消或收紧本次等待；实际等待永远不超过 Runtime `MaxCallWait`。
- Cluster、Discovery 和业务 Repository 通过单独的窄 client 注入，不加入 ServiceContext。
- Cluster client 在 Service 初始化阶段以 `Self()` 绑定 Source；其公开 Send/Call 不再要求业务重复传 source。
- `context.Context` 只负责 deadline、cancellation 和请求范围信息，不替代 ServiceContext。
- 第一版 Timer 只有一次性 `After`；不提供 ticker。Timer 使用单调时间计算期限，同一时刻到期不承诺彼此顺序，也不高于普通消息 admission。
- TimerID 只允许创建它的 Service 取消；取消已触发、已取消或未知 TimerID 返回 false。

## 8. Typed Client

Core 使用 CommandID 和参数管道；Tooling 和 Business 对外优先提供 typed client：

```go
type BattleClient struct {
	caller CommandCaller
	target ServiceID
}

func (client *BattleClient) StartBattle(
	request StartBattleRequest,
) (StartBattleResult, error) {
	// CommandID、参数和响应校验隐藏在 client 内部。
}
```

- typed client 不要求业务调用者手写 CommandID、`[]any` 校验和 Reply 包装。
- typed client 必须把输入转换为 Message 独占的不可变值或新快照；不能转发 Service 状态中的 map、slice 或 pointer。
- 本地 client 绑定 ServiceID。
- 领域 typed client 可以绑定 NodeID 与远端 ServiceID，并调用已经绑定 Source 的 per-Service ClusterClient。
- Core 的 `Send/Call/After` 采用一致的参数模型；不要同时维护 payload 与 args 两套语义。

## 9. Error、panic 与可观测性

错误分为三类，但继续使用标准 Go `error` 和稳定 error type/sentinel 表达：

- **Domain Error**：房间已满、状态不允许、余额不足等正常业务拒绝。
- **Runtime Error**：Service 不存在、Mailbox 满、Service 正在停止、节点不可达、Call 超时等运行失败。
- **Programmer Error**：重复注册、非法 Handler 签名或内部不变量破坏；可以 panic，并由 Runtime Handler 边界 recovery。

Call 的本地 completion 与 Cluster Error frame 使用同一套逻辑错误状态：

```go
type ErrorClass string

const (
	ErrorClassDomain  ErrorClass = "domain"
	ErrorClassRuntime ErrorClass = "runtime"
)

type ErrorCode string

type StatusError struct {
	Class   ErrorClass
	Code    ErrorCode
	Message string
}
```

- Domain code 使用稳定、可读的 `{domain}.{reason}`，例如 `room.full`；GSR 保留 `runtime.` 前缀。
- `StatusError` 同时承载本地 error 和可传输的稳定状态，不再额外包装一个重复的 `ErrorStatus`；它按 Class + Code 实现 `errors.Is`，本地和 Cluster 解码后的 typed client 使用同一种判断方式。
- Message 是经过审查的安全文本，只用于日志或展示，禁止比较字符串决定业务分支。
- Handler 返回未实现 GSR 状态契约的普通 error 时，Runtime 在本地记录原始 error，并向调用方返回 `runtime.handler_failed`。
- Handler panic 对调用方返回 `runtime.service_panicked`；panic value 和 stack 只进入本地诊断，不进入 Reply 或 wire。
- 第一版不支持任意 error details。调用方需要余额差额、当前人数等结构化信息时，优先设计为 typed result，而不是塞入 `map[string]any` error metadata。

Send 只能返回投递阶段的 Runtime Error，不能返回未来 Handler 产生的 Domain Error。需要业务结果时使用 Call。

Mailbox 必须有明确且可配置的有限容量，admission 不得无限阻塞或自动无限扩容：

- 本地 Send 在 Mailbox 满时返回 `ErrMailboxFull`。
- Call admission 失败时立即返回错误，并删除刚建立的 PendingCall。
- Timer 投递使用同一 admission 规则；失败时记录结构化 Runtime 诊断并结束本次触发，不静默无限重试。
- Cluster 出站队列同样有界；队列满时 Send/Call 返回明确错误。

## 10. 生命周期

Core 生命周期保持为：

```text
Created → Running → Stopping → Stopped
```

第一版 Service 接口固定为：

```go
type Service interface {
	OnStart(ServiceContext) error
	Handle(CommandContext, CommandID, ...any) (any, error)
	OnStop(context.Context) error
}
```

Service 不提供额外 `Close` 钩子；本地资源释放属于 `OnStop`。`Close` 是 Runtime 级操作，负责停止全部 Service 和 Runtime 自有并发资源。

启动期自身消息和无法强杀任务的语义见 [ADR 0007](adr/0007-command-context-call-and-timeout-stop.md)。

- Runtime `Start` 完成基础设施启动后进入 Running；`CreateService` 允许在 Running 阶段动态调用。
- `CreateService` 分配 ID 后运行 `OnStart`，只在启动成功后向调用方返回 ServiceID；启动失败的 ID 作废且不复用。
- `OnStart` 可以使用 ServiceContext 发起 Send、Call 和 After，但当前 Service 在 OnStart 成功前不接受普通业务消息，也不应发布到 Registry/Discovery。
- `OnStart` 向自身 Send 返回 `ErrServiceStarting`，向自身 Call 返回 `ErrCallCycle`。`After` 可以登记，但其 Command 最早只能在 Service 进入 Running 后参与普通 Mailbox admission；OnStart 失败时必须取消这些 Timer。
- 进入 `Stopping` 后立即拒绝新消息、取消 Timer，并结束该 Service 发起的 PendingCall。
- 当前 Handler 可以在 shutdown deadline 内结束；排队的 Send 被丢弃并计入诊断，排队的 Call 以 `ErrServiceStopping` 完成。
- `OnStop` 最多执行一次。Stop 和 Close 幂等并等待完成；重复 Start 返回稳定的生命周期错误。
- Stop 为当前 Handler 和 `OnStop` 建立统一的有限 shutdown deadline。Go 不能强杀忽略取消的 Handler 或 `OnStop`：deadline 到达后 Stop 返回稳定超时错误，Service 保持拒绝 admission 并从普通本地寻址中移除，但仍在运行的 runner 必须继续由 Runtime Inspection 追踪到真实返回。
- Runtime 不得在超时 Handler 尚未返回时并发调用 `OnStop`。Handler 后续返回后，runner 仍至多调用一次 `OnStop`，并传入已经取消的 shutdown context；若 `OnStop` 也不返回，任务继续保持可观测。
- 超时不表示业务 goroutine 已终止，也不得导致旧 ServiceID 复用。重复 Stop 返回或等待已保存的同一停止结果，不启动第二套停止流程。
- Handler panic 由 Runtime runner 边界 recovery，当前 Service 转入停止流程；相关 Call 以 `ErrServicePanicked` 完成，panic 与 stack 只进入本地诊断。
- `Failed`、`Quarantined`、`Degraded` 等作为停止原因或外层监督状态，不加入第一版 Core 状态机。
- Timer 在 Service 停止后不得继续投递。

## 11. Core 不变量测试

实现 Runtime 前先用测试锁死：

- 同一个 Service 的 Handler 永不并发，不同 Service 可以并发。
- 每个 Running Service 只有一个 Runtime-owned runner；Call 阻塞一个 Service 时不会阻塞其它 Service。
- Mailbox 按消息成功完成 admission 的顺序执行 FIFO；不把多个并发发送者的调用时刻定义成全局顺序。
- `Message.Source` 等于当前发起 Service 的 ServiceID，业务参数不能覆盖它。
- Send 产生零 SessionID，Call 产生非零 SessionID，两者进入同一个 Dispatcher 和 Mailbox。
- 各类 uint64 ID 到达上限后返回 ErrIDExhausted，不能回绕命中仍活跃或历史对象。
- Reply 不进入调用方 Mailbox；它通过 SessionID 恰好完成一个 PendingCall，重复或未知 Reply 被拒绝并记录诊断。
- Service 在 Call 等待期间不处理下一条消息，但其它 Service 仍可运行；self Call 被拒绝，超时和停止会清理 pending 状态。
- Call 的实际等待不超过 `MaxCallWait`；调用方 context 只能取消或选择更早 deadline。
- Core 测试必须证明 admission 会复制 `Args` 外层切片；每个携带 map、slice 或 pointer 的 typed client 必须单独证明其具体请求快照在 admission 后不受调用方修改影响。
- Mailbox 和 Cluster 出站队列容量有界；满载时快速返回稳定错误，不隐式扩容或无限等待。
- 进入 `Stopping` 后拒绝新消息；已经接受的消息遵守唯一、明确的停止策略。
- Service 进入 `Stopped` 后，Timer 不再向它投递 Command。
- After 只触发一次；CancelTimer、Stop 和触发完成之间的竞争最多产生一次成功 admission。
- CreateService 可以在 Running 阶段成功；OnStart 失败不会发布 ServiceID，且该 ID 永不复用。
- Stop 拒绝新 admission、处理当前 Handler、丢弃排队 Send、完成排队 Call，并保证 OnStop 最多执行一次。
- Handler 或 OnStop 超过 shutdown deadline 后，Service 退出普通寻址且残留 runner 保持可观测；Runtime 不宣称已强杀任务。
- Service 停止和重建后，旧 ServiceID 不能命中新 Service。
- 异步结果只能通过 Command 返回 Mailbox；领域测试负责证明旧 Revision 不会覆盖新状态。
- Local 与 Cluster Send/Call 的请求/响应语义一致；Core Message 和 ClusterEnvelope 是不同结构，本地路径不经过 Cluster 编码，ClusterEnvelope 以显式 frame kind 区分请求、响应和错误。
- Cluster 已知节点断连时必须返回 Runtime Error，不得静默接受；无法确定远端是否执行的写操作由 Call、RequestID 和幂等策略处理。
- Domain Error 经本地或 Cluster Call 后保持相同 Class/Code，typed client 的 `errors.Is` 判断不依赖 Message 文本。

## 12. API 评审清单

新增或修改公开 API 时检查：

1. 是否仍然只使用最小 Core 概念？
2. ServiceID 是否保持 Runtime 局部、临时且不可持久化？
3. Source 是否由当前发起 Service 自动设置？
4. 本地 Send 与 Call 请求是否只通过 SessionID 区分，并共用同一 Message/Dispatcher 路径；Reply 是否只完成 PendingCall 而不进入业务 Mailbox？
5. Cluster 的 NodeID 是否停留在 Cluster 层？
6. Core Message 与 ClusterEnvelope 是否保持分离，本地消息是否完全避免远程序列化？
7. 引用型 Args 是否为 Message 独占快照，发送方是否可能在 admission 后继续修改？
8. Call 等待时是否保持 Service 独占执行权，并具备超时、停止和 pending 清理路径？
9. Mailbox 与出站队列是否有界，Send 返回 nil 是否只代表本地接受？
10. Cluster Source 是否由 per-Service client 绑定，而不是全局隐式状态或业务参数？
11. 调用者是否被迫了解 Mailbox、Session、路由或清理顺序？
12. 谁拥有状态、goroutine、Timer 和关闭责任？
13. timeout、retry、幂等与补偿由谁决定？
14. 测试是否保护可观察契约，而不是绑定私有实现？
15. Handler 是否只通过返回值完成 Reply，是否存在漏回、重复回或异步回的旁路？
16. 生命周期和 Timer 是否具备 fake clock、阻塞 Handler 与停止竞态测试？
17. 所有 ID 分配器是否在上限永久失败而不是回绕，测试是否能直接注入临界状态？
18. Reply error 是否保留稳定 Class/Code，是否意外暴露原始 error、panic 或 stack？

如果调用者需要阅读 Runtime 内部实现才能正确使用 API，说明抽象边界仍未完成。
