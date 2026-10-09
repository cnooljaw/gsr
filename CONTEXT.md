# Context

## Glossary

- **协议 Handler 上下文**：`protocol.Dispatcher[gsr.CommandContext]` 将当前 `HandleClientMessage` 的 self 原样传到强类型 Handler；self 提供 Agent 的 Service 能力，独立的 `protocol.Context` 提供客户端请求关联和 Response/Push。两者仅在当前同步 Handler 内使用，不存入 Agent 字段或异步任务。协议包只透传类型参数，不依赖应用层 `gsr`。

客户端接入的通用 Gate 与协议分发已位于 `gate/` 公共包；`examples/websocket-gate` 只保留 Echo 业务协议、Agent 和联调入口。应用可在 `gsr.Run` 中通过 `self.OpenGate` 创建对应 Gate 和 `gsr.Service` Agent。Echo 示例已使用 `gsr.Run` 与 MainService，完成打开 Gate 后 MainService 自身退出，Gate 与连接 Agent 继续运行。

- **C/S Envelope**：Protobuf 应用消息，字段为 msg_id、request_id、payload，编号依次为 1–3；请求、响应、通知和推送使用独立的消息编号。通用协议文件定义 Envelope 和通用错误回复，业务协议文件定义各自的消息编号与 payload。每个非零 MsgId 在 `.proto` 中显式声明 payload 类型、方向和消息种类，并生成协议目录；业务 Agent 按实现进度手动注册强类型 Handler，不要求协议定义和业务 Handler 同步完成。Request 必须带非零 request_id 并得到一次响应，Notify 的 request_id 为零且不回复。Gate 不解析业务协议。首版不使用旧 12 字节头；业务拒绝使用业务 Response，无法取得正常结果时使用带原 request_id 的 ErrorResponse，业务错误码由业务协议定义。
- **C/S request_id**：Envelope 中的 uint32 客户端请求关联号，0 为通知/推送、非零为 Request，响应原样回传；单连接从 1 单调分配，不回绕、不复用，耗尽后重连。与 Core/Cluster Session 分离，不作为防重放或幂等凭据。见 [客户端接入草案](docs/RFC-WEBSOCKET-GATE.md)。
- **C/S seq**：独立于 request_id 的 uint64 上行业务包序号，在线上使用 8 字节大端序，单连接从 1 单调递增、不回绕；先对 seq 原始字节和 Envelope 原始字节验 MAC，认证成功后立即消费序号，通知也消耗序号。不得作为业务执行成功或幂等保证。
- **C/S 临时密钥**：每条生产 Gate 连接由双方分别生成 32 字节 X25519 私钥和公钥；私钥不发送，公钥使用固定 33 字节二进制握手帧交换，双方用 shared secret 和固定 HKDF-SHA256 参数派生上行 macKey。Plain 模式不执行此握手。
- **Agent Service（客户端接入）**：原则上一个客户端连接对应一个普通业务定义的 `AgentService`，不是 Cluster 的私有入站连接 owner。Runtime 内部消息类型区分客户端消息与普通 Core Command；应用 Agent 通过实现可选的 `gsr.ClientMessageHandler` 明确启用客户端处理能力，基础设施适配到 Core 的同名接口。Agent 使用 `.proto` 生成的协议目录按客户端 `msg_id` 解码，通过 `protocol.Register` 逐个绑定已实现的强类型 Handler；普通本地/Cluster Command 仍由显式 `Service.Handle` 按 `CommandID` 分发。协议映射可以先定义，Handler 随业务实现逐步注册，不需要占位 Handler。两类 handler 在同一个 Service、Mailbox 和串行 runner 中执行并共享连接私有状态。Core Command 的 Send/Call 仍是同一种消息类型，由 Session 是否为零区分；客户端 `request_id` 与 Core/Cluster Session 分离。`CommandContext.Source()` 表示 Service 来源，不用于推断 Gate/Cluster 或消息类别。Gate 持有 socket、握手、安全状态和关闭权；AgentService 持有客户端会话能力和 request_id 关联，并组合无独立 ServiceID/Mailbox 的 `protocol.Dispatcher` 及业务协议组件。应用 Gate 配置提供 `NewAgent func(gate.ClientSession) gsr.Service` 构造回调，底层配置使用 `runtime.Service`，不增加独立的 AgentFactory 或 Service 管理器；每条连接由 Gate 创建一个独立 Agent 实例。具体业务协议组件由业务项目实现，只负责 msg_id、Protobuf、请求校验、简单收发和业务 Service 路由；Agent 可以持有 playerID、登录状态、roomID、座位和临时流程等连接私有轻量状态，但不持有跨连接共享、可恢复或需持久化的权威房间/玩家/战斗状态。GSR 不内置大厅、游戏或其它业务协议；后者由独立业务 Service 所有，Agent 不接触 Gate/Runtime 内部细节，也不建立公开继承式 BaseAgent。
- **ClientSession（客户端接入）**：Gate 绑定单条连接并授予 AgentService 的受限写回能力，只提供 `Send(Envelope)` 和 `Close`；不暴露 `net.Conn`、Gate ServiceID 或连接表。Response、Push 以及 `request_id` 的合法性由 Agent 的协议分发器和业务协议负责，Gate 不解释这些语义。`Send` 只表示本地出站队列接纳，连接关闭或队列满时返回稳定错误，写循环是否送达由 Gate 管理。
- **Gate→Agent 内部投递（客户端接入）**：Gate 完成 WebSocket 安全校验后，使用 `Runtime.DeliverClientMessage(target, envelope)` 将完整 Protobuf Envelope 字节作为不透明消息送入 AgentService Mailbox，Source、Session 和 Core CommandID 均为零；Runtime 在接纳时复制字节快照。Runtime 内部消息类型明确区分客户端消息与普通 Core Command；目标 Service 必须实现可选的 `runtime.ClientMessageHandler`。未实现时立即返回稳定错误 `ErrClientMessageUnsupported`，不会入队；不能靠 Source 猜测，也不能把客户端 msg_id 当作 Core CommandID。该单向入口供 Gate 等受信任基础设施使用，不是业务 Service API；Agent 到业务 Service 仍使用普通 ServiceContext.Send/Call。
- **Gate 容量与关闭（客户端接入）**：Gate 对连接数、握手等待、单帧大小和每连接出站队列设置有限上限；初始握手等待默认 5 秒、完整 WebSocket 二进制帧默认 4 MiB，最大连接数必须由部署显式配置，出站队列按字节而不是消息条数限制。入站直接受 Agent Mailbox 容量约束，不增加无界中间队列。停止时先停止监听和新消息接纳，使 ClientSession 失效，再关闭连接、通知 Agent 一次并等待 Gate 自己的 I/O owner；不等待或强杀 Agent。
- **Gate 协议模式（客户端接入）**：统一 Gate 通过 `ProtocolMAC`（零值默认）或显式 `ProtocolPlain` 固定单个监听的协议。MAC 使用 X25519、seq 和 MAC；Plain 直接接收 Envelope，仅绑定回环/私有 IP。两者共享连接 owner、TCP 限额、队列、诊断和生命周期，不猜首帧或自动降级；TLS 独立配置，业务 token 鉴权属于 Agent。
- **C/S WebSocket 关闭码**：Gate 使用标准关闭码：1000 正常、1001 下线、1002 协议错误、1003 不支持数据、1008 安全策略拒绝、1009 超限、1011 Gate 内部故障；1006/1015 只作本地观察，不能主动发送。Gate 处理传输和安全失败，Agent 处理已认证 Envelope，业务 Service 处理业务拒绝；客户端不依赖关闭原因文本分支。
- **C/S Gate 错误日志**：Gate 使用共享 Logger 记录握手、帧、seq、MAC 和关闭错误，按单连接错误类别保留首条并汇总重复项；不记录私钥、shared secret、macKey、原始 payload 或未认证字段。Agent 和业务 Service 分别记录自己的路由及业务语义，不由 Gate 重复记录。

- **Service 绑定的 Cluster 入口**：应用层通过 `self.Cluster()` 取得；框架绑定当前 Service 的 Self ID，而非消息的 Source。多个入口共享节点唯一 Cluster，不各自拥有连接或关闭责任；业务不手工注入 Host/Cluster，不通过全局状态推断当前 Service。

> [ADR 0011](docs/adr/0011-service-first-application-api.md) 已落地应用入口：每节点一个 Runtime/Cluster，MainService 集中组装，CommandID 由目标解释。下文底层 ClusterClient 等类型属于基础设施，业务不手工构造。

- **应用运行入口（gsr.Run）**：Core 之外的统一启动函数，创建独立 Runtime、Cluster、Logger，运行普通 MainService，并负责初始化回滚和固定顺序关闭。内部资源 owner 不公开，不是全局单例或组件容器；app.Host 已移除。
- **HTTPIngress**：`gsr.Run` 给 HTTP Handler 的长期受信任本地入口，只提供按名称 Send/Call；每次查节点内名称表，通过 Runtime 基础设施入口投递，Source 固定为零。它不是可跨 goroutine 保存的 ServiceContext，不提供业务 Service 身份或 Cluster 调用能力。HTTP Listener 由 Run 持有，在 Runtime 停止前关闭。

- **坏帧诊断抑制**：单连接内按固定协议错误类别只保留首条明细，重复项不进入日志或 Diagnostics；Cluster.SuppressedDiagnosticCount 独立累计，连接结束用 DiagnosticBadFramesSuppressed 汇总数量。不是业务日志去重，不改变消息投递或错误结果。

- **统一日志服务**：logging.Logger 提供共享 FIFO 队列和单写入者；记录处采集时间、位置和内容快照，按成功入队序号输出。业务、Runtime、Cluster 注入同一 Logger，框架直接记录，不通过 Diagnostics 转发。应用最后关闭它；支持终端、借用 Writer 和 Directory 选择的滚动文件，内部文件由 Logger 管理。输出失败后拒绝接纳，Err() 与 Close 保留首次故障；SLS 外部采集待后续。

- **ServiceID**：`uint64` 形式的 Runtime 局部 Service 地址，只在当前 Runtime 生命周期内有效，不包含 Node、Epoch 或 Generation，也不是可持久化业务身份。
- **ServiceName**：由 Core 外层管理的稳定逻辑名称。应用层 `self.Register(name, id)` 只接受当前应用仍存活的目标 ServiceID；本地 `self.SendName/CallName` 与 Cluster 入站共用该表，每次解析成当前节点的 ServiceID。名称归目标 ServiceID，目标启动失败或停止时在业务 OnStop 前自动清理，之后可由新实例复用；运行中的名称不直接改绑。跨节点动态发现仍属外层能力。
- **NodeID**：Cluster 用于选择远端节点和连接的路由名称，不属于 Core ServiceID。仅出站调用不要求本地 NodeID 或监听；入站不持有来源 NodeID，Call 回复沿原连接返回。主动 Send/Call 与 open 接受新调用是独立能力，NodeID 不是认证凭据。
- **SessionID**：`uint64` 形式的 Core 本地请求关联号；零表示 Send、不需要 Reply，非零表示 Call、等待一次 Reply。它只负责关联，在当前 Runtime 内单调分配且不回绕复用。
- **TimerID**：`uint64` 形式的 Runtime 局部 Timer 标识，只在当前 Runtime 生命周期内有效；零值无效，已发放的 ID 不回绕复用。
- **CommandID**：`uint32` 形式的业务命令标识，与协议字段保持同宽以兼容现有命令值；由业务/协议包维护静态常量或生成代码。Runtime 不分配、不回收，也不从它推断 Service 能力。一个 CommandID 对应的 typed Handler 参数数量和类型固定。
- **Source**：由 Runtime 写入本地 Message 的来源字段。普通 Service Send/Call 使用当前发起 ServiceID；宿主/基础设施通过 Runtime.Send/Runtime.Call 注入的消息、Gate 投递的客户端消息使用 `ServiceID(0)`；Timer 触发的 Command 也使用零值，创建者由 Timer owner 表达。受控 Redirect 可以按固定 relay 策略保留已有 Message 的原始 Source，但业务不能传入或伪造任意值。跨节点 Call 的 Source 只保存在发起方本地 ClusterPendingCall 中，回复通过 ClusterSessionID 关联；Source 不进入 wire，目标 Handler 看到的是零值，不是远端 Service 身份。
- **Message**：Core 内存中的 Runtime-owned 本地消息对象。普通消息承载 Source、Target、Session、Command 和 Args；客户端消息由私有消息类型标记并承载不透明的 Protobuf Envelope 字节。Send/Call 共享普通消息路径，客户端消息不会伪装成 Core Command。
- **Dispatcher**：普通业务 Service 的默认命令分发方式，也是 `gsr` 应用层的可选能力。它在构造时复制并校验 `CommandID -> typed Handler` 表，运行时绑定参数并调用；它不是 Runtime Registry，也不接受运行期注册。只有经真实压测确认的个别热点 Command 才在 Service.Handle 中手写快速分支，其余命令继续交给 Dispatcher。
- **Command 参数类型边界**：本地调用保留 Go 类型，Dispatcher 不做隐式转换；默认 Cluster codec 解码为 int64、uint64、float64、[]any、map[string]any 等规范类型。跨节点 Handler 按规范类型声明输入与可编码输出，业务结构体和具名类型在边界显式转换；统一 Send/Call 模型不代表任意 Go 类型原样跨网传递。
- **协议 Dispatcher 构造**：`protocol.NewDispatcher[Scope](registry, client)` 内部绑定同一目录的解码和客户端写回，不公开独立协议 Session。请求内通过 `protocol.Context.Response/Push` 写回，普通 Command 等其它业务流程可通过 Dispatcher.Push 主动推送；两者不增加 Mailbox 或 runner。
- **ClientMessageHandler**：Service 可选实现的客户端消息处理能力。Core 的 `runtime.ClientMessageHandler` 使用 Core CommandContext；应用 Agent 实现同名的 `gsr.ClientMessageHandler`，使用应用 CommandContext。应用组装仅为实现该接口的 Service 使用客户端消息适配器，普通 Service 不因经过 `gsr.Run` 就获得此能力；未实现时 `Runtime.DeliverClientMessage` 在 Mailbox admission 前返回 `ErrClientMessageUnsupported`。
- **Message Ownership**：消息成功被 Runtime 接受后，Message 独占自己的 `Args` 外层切片。`any` 内部的 map、slice、pointer 等引用数据必须由 typed client 按具体消息类型复制成消息专用快照；发送方不得在 admission 后继续修改，也不得把 Service 拥有的可变状态引用作为参数发送。
- **CommandContext**：Runtime 为每次 Handler 调用创建的只读消息上下文，提供当前 Service 的 Core 能力以及由 Runtime 填入的 `Source()`；Source 可能为 `ServiceID(0)`（Timer 或宿主/基础设施注入）；业务不能覆盖 Source，也不能通过它手工或异步 Reply。
- **Runtime.Call**：宿主或受信任基础设施的本地 Call 入口，Source 固定为零；使用独立本地 Session、普通 Mailbox 和有限等待，不创建 goroutine。普通 Service 仍通过 ServiceContext.Call 发起调用。
- **Cluster.Open（基础设施）**：显式提供目标 Runtime 和 ListenConfig.Address，由 Cluster 管理监听及入站连接，不管理 Runtime 生命周期，不自动注册或发布地址；第一版每个 Cluster 最多一个监听，不支持多监听、监听热切换或运行中换绑 Runtime。不 Open 仍可主动 Send/Call 并沿原连接接收回复。
- **节点注册与发现**：Core 和 Cluster 之外的能力，由外层维护 NodeID 到可访问地址的映射；不等同于监听，也不替业务判断资源归属。ClusterConfig.Nodes 是当前静态路由来源，不是集群成员或存活表；动态发现的路由更新接线尚未实现。监听绑定地址不必等于外部发布地址，发布由应用负责。
- **入站连接并发**：连接组件管理一个读循环和数量有上限的 Call 等待任务；满载暂停读取，Send 顺序接纳，混合请求以本地 Mailbox 接纳顺序执行。连接关闭取消基础设施等待并等待任务退出；目标 Handler 不被强杀。
- **PendingCall**：Runtime 内部以非零 SessionID 索引的一次等待。Reply 通过 completion 路径完成它，不进入调用方 Mailbox 或业务 Handler。
- **ClusterPendingCall**：Cluster 内部以非零 ClusterSessionID 索引的一次远程等待，保存本地 Source、远端 Node、Target 和 Command；网络写入前绑定私有连接 owner，断线只清理该连接的等待，网络回复还须匹配此归属。尚未绑定的排队等待保留原超时；Source 和连接 owner 不进入 wire。
- **Run-to-completion**：一个 Handler（包括其中的 Call 等待）结束前，同一 Service 不执行下一条消息；等待期间其它 Service 仍可运行。
- **MaxCallWait**：Runtime 对一次本地 Call 施加的最大等待时间。调用方必须提供 `context.Context`，其 deadline 可以缩短实际等待，但不能延长或取消 Runtime 上限。
- **Service Runner**：由 Runtime 为每个 Running Service 管理的 goroutine，串行消费该 Service 的 Mailbox；其生命周期和 recovery 属于 Scheduler，不属于业务。
- **Service 组装**：宿主启动 Runtime 并显式创建普通 MainService；Service 内使用 ServiceContext.CreateService 创建其它实例，底层仍委托所属 Runtime。传实例，不引入工厂、Launcher、ServiceManager 或 Runtime 自动 Bootstrap。创建成功的 Service 归 Runtime 管理，不随创建者退出而停止。
- **MainService**：应用定义的普通入口 Service，负责业务组装和 Send/Call；名字不具有 Runtime 特权。退出不代表 Runtime 退出，宿主负责进程信号或完成通知及最终收尾。
- **Exit**：ServiceContext 的自身退出请求，幂等、不阻塞、不强杀当前 Go 执行。OnStart/Handler 返回后由 Runtime 收尾；当前 Handler 的 Call 先按 return 完成，其余排队请求停止执行，已有取消、超时、panic 和 Runtime Stop 语义不变。
- **Start Command**：可选的业务协议，不是所有 Service 必须实现的生命周期；与 Runtime.Start 和 Service.OnStart 区分。
- **OnStart 失败**：只回滚当前 Service，Runtime 保持 Running，可以继续创建其它 Service；失败实例不发布 ServiceID，且该 ID 永不复用。Runtime 至多调用一次 `OnStop` 为部分初始化资源清理。
- **ClusterEnvelope**：Cluster/Transport 使用的可编码网络载体，显式区分 request、response 和 error frame；不携带业务或 wire version。远端 relay 解码请求后创建新的本地 Message；本地投递不创建或编码 ClusterEnvelope。
- **FrameKind**：ClusterEnvelope 的显式方向标签，第一版只有 Request、Response 和 Error；不能只凭 Session 是否为零推断 frame 方向。
- **ServiceTarget**：ClusterEnvelope 的远端目标值；必须且只能填写当前远端 Runtime 的临时 ServiceID 或由外层解析的 ServiceName。它不把 NodeID、Epoch、Lease 或 Generation 编进 Core 地址。
- **ValueCodec**：Cluster 的默认值编码器，只接受 nil、bool、整数、有限浮点数、string、`[]byte`、list 和 string-key map；解码将整数规范化为 `int64`/`uint64`，浮点数规范化为 `float64`，并执行深度与 payload 上限。
- **PayloadCodec**：节点所有命令共享的参数与回复编解码器；默认 ValueCodec，不负责命令准入、路由或 Source。
- **ClusterConfig**：已实现的公开出站组装配置，包含静态目标地址表、统一 PayloadCodec（为空使用 ValueCodec）、有限 Call/Dial/Write 等待、每目标出站队列容量和帧上限；不包含本地 NodeID、Runtime 或监听能力。零值数值使用有限默认值，负数拒绝。
- **FrameCodec**：Cluster 的长度前缀 frame 编解码器；使用四字节大端 body length，`MaxFrame` 包含前缀，只处理一个完整 frame，不负责 stream I/O、版本协商或连接生命周期。
- **ClusterClient**：初始化时绑定一个 Source ServiceID、NodeSenderResolver、统一 PayloadCodec、ClusterCallRegistry 和有限 MaxCallWait 的 per-Service 窄 client；不能覆盖 Source，Send/Call 都通过同一个 Cluster 入口。基础设施由 NewCluster 构造实例，再由 Cluster.Client(source ServiceID) 提供绑定能力；普通业务使用 self.Cluster()，不手工组装；绑定入口属于受信任组装能力，不验证 Service 存活，不新增另一种业务 Call。公开出站组装已实现，Open 已实现。
- **ClusterCallRegistry**：一个 Cluster 级的 PendingCall owner。所有 source-bound ClusterClient 共享它分配 ClusterSessionID；`Complete(from, envelope)` 只完成未绑定连接的逻辑/内存链路响应，网络响应通过私有连接 owner 完成。关闭时统一结束未完成的 Call。
- **Redirect / Relay**：仅供 Runtime 或明确受信任的 Adapter/Transport 使用的消息转发能力。它转发 Runtime-owned Message 或 relay token，不接受业务任意指定的 Source；首版只考虑单向转发，不转移 Call 的 PendingCall/Session。
- **ClusterSessionID**：`uint64` 形式的 Cluster 请求关联号；零表示 Send，非零表示 Call。它由 Cluster 管理、在当前实例内单调分配且不回绕复用，不要求与 Core SessionID 相同。
- **ErrorClass**：Reply error 的顶层类别。第一版只有 Domain 与 Runtime；programmer error 在 Runtime 边界转成稳定 Runtime error，并把内部诊断留在本地。
- **ErrorCode**：跨本地与 Cluster 稳定传递的字符串错误代码，例如 `room.full` 或 `runtime.service_not_found`；程序逻辑不得依赖 Error Message。
- **StatusError**：包含 ErrorClass、ErrorCode 和安全 Message 的可判定 error；本地和远端 typed client 都通过 `errors.Is` 或 Class/Code 判断。
- **Revision**：领域状态版本，用于拒绝同一 Service 实例中的迟到异步结果；它不同于 ServiceID，也不属于 Core Service 身份。
- **业务语义边界**：Runtime/Cluster 只负责消息投递、路由和连接恢复，不判断业务资源的唯一所有权、一致性或过期。fencing、幂等、RequestID、补偿和业务过期规则由拥有该操作语义的业务或外层模块负责；ServiceID 不引入 Epoch、Lease 或 Generation。
- **Admission**：Mailbox 或出站队列对消息的本地接受动作。Send 返回 nil 只说明 admission 成功，不说明目标 Handler 已执行或业务操作已成功。
- **NodeSender**：Cluster 为一个远端 Node 唯一管理的 outbound 发送者。当前基础实现用固定容量队列和单一 worker 完成本地 admission 与顺序投递；队列满立即失败。admission 后的 Transport error/panic 和关闭丢弃进入有界、非阻塞的 `Failures()` 诊断 stream，慢消费者不会阻塞 sender，溢出由 `DroppedFailureCount()` 计数。内部单目标连接 owner 已实现按需建连与恢复：空闲时不拨号，下一请求至多触发一次有限 Dial，不自动重放结果不确定的请求；静态目标路由与公开出站组装已实现，动态路由及 Open 已实现。
- **DeliveryFailure**：NodeSender 在 Envelope 已 admission 后发布的一次基础设施失败，包含目标节点、路由元数据和底层 error，不携带业务 payload、不触发业务 callback，也不表示远端 Handler 的业务结果。
- **Diagnostic**：已实现的公开 Cluster 出站诊断事件，区分投递失败、连接异常和响应拒绝。所有组装内生产者共享一个 256 容量的非阻塞 Diagnostics 出口，满载丢弃并计数；不是广播、可靠审计或业务结果通道，正常调用失败仍遵循 Send/Call 契约。全部生产者退出后才关闭出口，日志消费者由宿主基础设施管理。
- **远程 Send 所有权**：A 节点只负责本地 admission 和自身的传输诊断，B 节点负责收到消息后的 Handler 处理与错误记录；Send 不把 B 的 Handler error 返回 A。需要远端结果时使用 Call。
- **Cluster 入站连接 owner**：私有 `inboundConnection` 持有单条 inbound socket 的读循环与有界 Call 任务，经 `localRequestAdapter` 解码并投递 `Runtime.Send/Call`，沿原连接写回结果；没有另一个公开 ConnectionAgent 或 Service。
- **InMemoryTransport**：Cluster 0 提供的同步、确定性进程内 Transport；它在接收边界 Clone Envelope，供测试和后续 sender/agent 使用，不代表生产 TCP、重连或远端执行语义。
- **Timer owner**：登记 Timer 的 Service/Runtime owner。它用于校验取消权限和表达 Timer 创建者；Timer 到期消息的 Source 不使用 owner 的 ServiceID，而固定为零值。
- **RuntimeState**：Runtime 的生命周期状态。公开顺序为 `Created → Starting → Running → Stopping → Stopped → Closed`；`Starting` 表示 Runtime-owned Core 组件正在启动，`Stopped` 表示停止流程完成，`Closed` 表示 Runtime-owned 资源已最终释放。
- **Lifecycle Coordinator**：Runtime 内部按固定 start/stop 计划编排 Core 组件的生命周期；它不拥有组件资源，也不是可无限注册的业务插件容器。
- **Quiesce**：关闭流程中停止新 admission、入站和重连的阶段，不等同于等待所有任务完成。已实现的 Cluster.Quiesce() 针对首轮出站组装拒绝新 Client/Send/Call，取消尚未绑定的 Call 并停止新拨号；已绑定 Call 保留原等待上限和回复机会。最终资源释放由 Cluster.Close(ctx) 完成，调用方停止等待不取消清理。内部 tcpListener 已支持独立停止入站接纳：关闭监听并阻止旧连接的新请求，保留已接纳 Call 的原连接回复机会，最终取消才断连；公开 Open 已确认复用 Cluster.Quiesce 统一停止入站与出站接纳并关闭监听，不主动关闭已建立连接、不等待任务退出，监听关闭错误由最终 Close 合并返回；接线已实现。
- **Stop / Close**：`Runtime.Stop(ctx)` 负责关闭 admission 并有序停止全部 Core 组件，`Runtime.Close()` 负责 Stop 完成后的最终资源释放；二者幂等，Stop 后不得再次 Start。`Close()` 使用 `ShutdownTimeout`，没有调用方 context。
- **RuntimeInspection**：`Runtime.Inspect()` 返回的只读值快照，包含 Runtime 状态、Service/ mailbox/runner 状态、PendingCall/Timer 数量和基础故障计数；它不暴露 Runtime-owned 内部对象。
- **Runtime Handler 诊断**：`runtime.Diagnostic` 是本地 Handler 错误或真实 panic 的值快照，包含消息关联字段、错误分类、最多 4 KiB 的 Message 和最多 16 KiB 的 panic Stack；不保留业务对象或原始 error 引用。`Runtime.Diagnostics()` 是容量 256 的非阻塞单消费流，满载丢弃由 `DroppedDiagnosticCount()` 计数，仅在 Runtime 进入 Closed 时关闭。它与 Cluster 诊断分属不同包，不共用事件框架；Runtime 直接写共享 Logger，诊断流仅供额外观察，不需要宿主转发为日志。
- **ServiceInspection**：RuntimeInspection 中一个 Service 的值快照，包含 ServiceID、生命周期状态、Mailbox 深度、runner 完成状态和 shutdown deadline 是否已超过。
- **ShutdownTimeout**：Runtime 关闭全部 Core 组件共享的内部时间预算。Go 无法强杀不响应取消的 Handler、OnStart 或 OnStop；超时后的残留任务和资源状态必须继续可观测。
