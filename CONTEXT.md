# Context

## Glossary

- **Service 绑定的 Cluster 入口（目标接口，待实现）**：应用层通过 `self.Cluster()` 取得；框架绑定当前 Service 的 Self ID，而非消息的 Source。多个入口共享节点唯一 Cluster，不各自拥有连接或关闭责任；业务不手工注入 Host/Cluster，不通过全局状态推断当前 Service。

> 迁移说明：[ADR 0011](docs/adr/0011-service-first-application-api.md) 已确认但未实现。目标是每节点一个 Runtime/Cluster，MainService 集中组装，业务不感知 Host；CommandID 由目标 Service 解释、跨 Service 可复用，不区分本地/远程。下文 Host、CommandCodecRegistry 与 ClusterClient 组装描述属于待替换的当前实现。

- **应用宿主（app.Host）**：Core 之外的具体组装 owner，统一创建 Runtime、Cluster、Logger、启动普通 MainService，并负责初始化回滚与固定顺序关闭。不是 ServiceManager、全局 Runtime 或通用组件容器；业务仍通过自身 ServiceContext 通信。

- **坏帧诊断抑制**：单连接内按固定协议错误类别只保留首条明细，重复项不进入日志或 Diagnostics；Cluster.SuppressedDiagnosticCount 独立累计，连接结束用 DiagnosticBadFramesSuppressed 汇总数量。不是业务日志去重，不改变消息投递或错误结果。

- **统一日志服务**：logging.Logger 提供共享 FIFO 队列和单写入者；记录处采集时间、位置和内容快照，按成功入队序号输出。业务、Runtime、Cluster 注入同一 Logger，框架直接记录，不通过 Diagnostics 转发。应用最后关闭它；支持终端、借用 Writer 和 Directory 选择的滚动文件，内部文件由 Logger 管理。输出失败后拒绝接纳，Err() 与 Close 保留首次故障；SLS 外部采集待后续。

- **ServiceID**：`uint64` 形式的 Runtime 局部 Service 地址，只在当前 Runtime 生命周期内有效，不包含 Node、Epoch 或 Generation，也不是可持久化业务身份。
- **ServiceName**：由 Core 外层 Registry/Discovery 管理的稳定逻辑名称，可解析为当前 ServiceID。
- **NodeID**：Cluster 用于选择远端节点和连接的路由名称，不属于 Core ServiceID。仅出站调用不要求本地 NodeID 或监听；入站不持有来源 NodeID，Call 回复沿原连接返回。主动 Send/Call 与 open 接受新调用是独立能力，NodeID 不是认证凭据。
- **SessionID**：`uint64` 形式的 Core 本地请求关联号；零表示 Send、不需要 Reply，非零表示 Call、等待一次 Reply。它只负责关联，在当前 Runtime 内单调分配且不回绕复用。
- **TimerID**：`uint64` 形式的 Runtime 局部 Timer 标识，只在当前 Runtime 生命周期内有效；零值无效，已发放的 ID 不回绕复用。
- **CommandID**：`uint32` 形式的业务命令标识，与协议字段保持同宽以兼容现有命令值；由业务/协议包维护静态常量或生成代码。Runtime 不分配、不回收，也不从它推断 Service 能力。一个 CommandID 对应的 typed Handler 参数数量和类型固定。
- **Source**：由 Runtime 写入本地 Message 的来源字段。普通 Service Send/Call 使用当前发起 ServiceID；宿主/基础设施通过 Runtime.Send/Runtime.Call 注入的消息使用 `ServiceID(0)`；Timer 触发的 Command 也使用零值，创建者由 Timer owner 表达。受控 Redirect 可以按固定 relay 策略保留已有 Message 的原始 Source，但业务不能传入或伪造任意值。跨节点原始来源由 ClusterSessionID 和传输上下文关联，不把节点身份编码进 ServiceID。
- **Message**：Core 内存中的本地消息对象，包含 Source、Target、Session、Command 和 Args；它不进行 wire 编码，Send 与 Call 共用该结构。
- **CommandDispatcher**：可选的每 Service 标准分发器。它在构造时复制并校验 `CommandID -> typed Handler` 表，运行时只负责固定参数的数量/类型绑定和调用；它不是 Runtime Registry，也不接受运行期注册。
- **CommandHandlers**：`CommandDispatcher` 的构造输入。命令表属于单个 Service，构造完成后不可变；非法 Handler 签名、参数不匹配和未知命令分别使用稳定的 Runtime Error 表达。
- **Message Ownership**：消息成功被 Runtime 接受后，Message 独占自己的 `Args` 外层切片。`any` 内部的 map、slice、pointer 等引用数据必须由 typed client 按具体消息类型复制成消息专用快照；发送方不得在 admission 后继续修改，也不得把 Service 拥有的可变状态引用作为参数发送。
- **CommandContext**：Runtime 为每次 Handler 调用创建的只读消息上下文，提供当前 Service 的 Core 能力以及由 Runtime 填入的 `Source()`；Source 可能为 `ServiceID(0)`（Timer 或宿主/基础设施注入）；业务不能覆盖 Source，也不能通过它手工或异步 Reply。
- **Runtime.Call**：宿主或受信任基础设施的本地 Call 入口，Source 固定为零；使用独立本地 Session、普通 Mailbox 和有限等待，不创建 goroutine。普通 Service 仍通过 ServiceContext.Call 发起调用。
- **Cluster.Open**：已确认的可选入站组装动作，已实现公开入口。显式提供目标 Runtime 和 ListenConfig.Address，由 Cluster 管理监听及入站连接，不管理 Runtime 生命周期，不自动注册或发布地址；第一版每个 Cluster 最多一个监听，不支持多监听、监听热切换或运行中换绑 Runtime。不 Open 仍可主动 Send/Call 并沿原连接接收回复。
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
- **ValueCodec**：Cluster 0 的默认值编码器，只接受 nil、bool、整数、有限浮点数、string、`[]byte`、list 和 string-key map；解码将整数规范化为 `int64`/`uint64`，浮点数规范化为 `float64`，并执行深度与 payload 上限。
- **CommandCodec**：一个 Command 的 payload 编解码能力，既可以使用默认 `ValueCodec`，也可以由业务提供 typed 实现；它只负责值与字节之间的转换，不负责路由、Service 能力或 Source。
- **CommandCodecRegistry**：启动期创建并冻结的 `CommandID -> CommandCodec` 表；构造后只查找，不允许运行期注册、覆盖或删除，重复 CommandID 是配置错误。
- **ClusterConfig**：已实现的公开出站组装配置，包含静态目标地址表、不可变 Command codec registry、有限 Call/Dial/Write 等待、每目标出站队列容量和帧上限；不包含本地 NodeID、Runtime 或监听能力。零值数值使用有限默认值，负数拒绝。
- **FrameCodec**：Cluster 0 的长度前缀 frame 编解码器；使用四字节大端 body length，`MaxFrame` 包含前缀，只处理一个完整 frame，不负责 stream I/O、版本协商或连接生命周期。
- **ClusterClient**：初始化时绑定一个 Source ServiceID、NodeSenderResolver、启动期 CommandCodecRegistry、ClusterCallRegistry 和有限 MaxCallWait 的 per-Service 窄 client；不能覆盖 Source，Send/Call 都通过同一个 Cluster 入口。已确认由 NewCluster 构造显式组装对象，再由 Cluster.Client(source ServiceID) 集中提供这些依赖以改善易用性；绑定入口属于受信任组装能力，不验证 Service 存活，不新增另一种业务 Call。公开出站组装已实现，Open 已实现。
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
- **ConnectionAgent**：Cluster 为一条 inbound socket 创建的接收适配器。当前只处理已解码的逻辑 Request：调用窄的本地 dispatch、为 Call 沿所属连接写回 Response/Error，并把 socket/frame 读写生命周期留给外层连接 owner。
- **InMemoryTransport**：Cluster 0 提供的同步、确定性进程内 Transport；它在接收边界 Clone Envelope，供测试和后续 sender/agent 使用，不代表生产 TCP、重连或远端执行语义。
- **Timer owner**：登记 Timer 的 Service/Runtime owner。它用于校验取消权限和表达 Timer 创建者；Timer 到期消息的 Source 不使用 owner 的 ServiceID，而固定为零值。
- **RuntimeState**：Runtime 的生命周期状态。公开顺序为 `Created → Starting → Running → Stopping → Stopped → Closed`；`Starting` 表示 Runtime-owned Core 组件正在启动，`Stopped` 表示停止流程完成，`Closed` 表示 Runtime-owned 资源已最终释放。
- **Lifecycle Coordinator**：Runtime 内部按固定 start/stop 计划编排 Core 组件的生命周期；它不拥有组件资源，也不是可无限注册的业务插件容器。
- **Quiesce**：关闭流程中停止新 admission、入站和重连的阶段，不等同于等待所有任务完成。已实现的 Cluster.Quiesce() 针对首轮出站组装拒绝新 Client/Send/Call，取消尚未绑定的 Call 并停止新拨号；已绑定 Call 保留原等待上限和回复机会。最终资源释放由 Cluster.Close(ctx) 完成，调用方停止等待不取消清理。内部 tcpListener 已支持独立停止入站接纳：关闭监听并阻止旧连接的新请求，保留已接纳 Call 的原连接回复机会，最终取消才断连；公开 Open 已确认复用 Cluster.Quiesce 统一停止入站与出站接纳并关闭监听，不主动关闭已建立连接、不等待任务退出，监听关闭错误由最终 Close 合并返回；接线已实现。
- **Stop / Close**：`Runtime.Stop(ctx)` 负责关闭 admission 并有序停止全部 Core 组件，`Runtime.Close()` 负责 Stop 完成后的最终资源释放；二者幂等，Stop 后不得再次 Start。`Close()` 使用 `ShutdownTimeout`，没有调用方 context。
- **RuntimeInspection**：`Runtime.Inspect()` 返回的只读值快照，包含 Runtime 状态、Service/ mailbox/runner 状态、PendingCall/Timer 数量和基础故障计数；它不暴露 Runtime-owned 内部对象。
- **Runtime Handler 诊断**：`runtime.Diagnostic` 是本地 Handler 错误或真实 panic 的值快照，包含消息关联字段、错误分类、最多 4 KiB 的 Message 和最多 16 KiB 的 panic Stack；不保留业务对象或原始 error 引用。`Runtime.Diagnostics()` 是容量 256 的非阻塞单消费流，满载丢弃由 `DroppedDiagnosticCount()` 计数，仅在 Runtime 进入 Closed 时关闭。它与 Cluster 诊断分属不同包，不共用事件框架；宿主负责日志输出与脱敏。
- **ServiceInspection**：RuntimeInspection 中一个 Service 的值快照，包含 ServiceID、生命周期状态、Mailbox 深度、runner 完成状态和 shutdown deadline 是否已超过。
- **ShutdownTimeout**：Runtime 关闭全部 Core 组件共享的内部时间预算。Go 无法强杀不响应取消的 Handler、OnStart 或 OnStop；超时后的残留任务和资源状态必须继续可观测。
