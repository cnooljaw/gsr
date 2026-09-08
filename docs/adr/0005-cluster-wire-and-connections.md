# ADR 0005: Cluster wire、连接与重连

## Status

Accepted

## Context

GSR Cluster 需要保持 Skynet 风格的简洁调用面，同时明确 Go 的编码集合、注册时机、异常帧边界和连接 owner。若把每条连接都建模为同一 Node 的竞争全双工通道，就必须增加复杂的重复连接仲裁；Skynet 实际采用每个目标节点一个 sender、每条入站 socket 一个 agent 的方向性组织。

## Decision

- ClusterEnvelope 显式包含 request/response/error frame kind、ClusterSessionID、Target ServiceID/ServiceName、CommandID，以及 payload 或结构化 ErrorClass/ErrorCode/Message。它是逻辑消息值，不携带业务或 wire version；wire 直接编码 `StatusError` 的三个字段，不在 Core 另设重复的 ErrorStatus 类型。
- 默认 `ValueCodec` 只接受 nil、bool、整数、有限浮点数、string、`[]byte`、list 和 string-key map；解码使用 `int64`、`uint64`、`float64`、`[]any` 和 `map[string]any` 等规范 Go 类型，并限制嵌套深度与 payload 大小。领域 struct 通过启动阶段注册的 Command codec 转换。
- 不编码任意 pointer、func、chan、unsafe pointer、循环引用或进程内资源句柄。
- Command codec 在启动阶段通过不可变 registry 注册；重复 CommandID、零 CommandID 或 nil codec 是 programmer error，构造失败（静态配置可使用 Must 形式 panic），运行期不允许覆盖。
- Cluster 0 的 FrameCodec 使用四字节大端 body length，完整 frame 的 `MaxFrame` 包含长度前缀；它只编解码一个完整 frame，不负责 stream I/O、连接生命周期或版本协商。
- 未来 Transport/Codec framing 或连接握手负责 wire version；在完整且边界可信时，未知 framing version、未知 Command 和解码失败帧被丢弃，并记录限频诊断和指标。
- 非法长度、超过上限、无法恢复帧边界或认证失败时关闭连接。
- 每个远端 Node 只有一个 outbound NodeSender，并发建连请求合并；每条 inbound socket 使用独立 ConnectionAgent，响应沿原连接返回。
- 入站连接组件拥有单读循环和有上限的 Call 等待任务；Send 在读循环中顺序接纳，Call 名额满载时暂停后续读取，不建立无界等待队列。混合 Send/Call 只保证本地 Mailbox 接纳顺序；连接关闭取消基础设施等待并等待任务退出，不强杀 Handler。节点级连接数上限由外层 TCP Transport 负责。
- 入站诊断使用 listener 持有的单一有界非阻塞出口，所有连接直接上报并共享丢弃计数；不运行外部 callback，不建立每连接诊断缓冲或转发任务。只有组装 owner 在全部生产者退出后关闭出口，单连接不关闭它。共享容量不提供逐连接诊断公平性。
- 出站 PendingCall 先登记以覆盖排队时间，写入前绑定具体连接的内部 owner；绑定、取消、回复和断线清理在同一同步边界竞争。断线只结束该连接的等待，不波及同节点未发送或新连接的等待；网络回复必须匹配连接归属。绑定前已取消的 Call 不发包，已尝试写入的请求不自动重放。连接归属仅为进程内对象引用，不是公开 ID、Epoch 或 wire 字段。
- 断线后采用按需重连：下一条尚未发送的请求至多触发一次有限 Dial，空闲时不启动后台拨号循环。Dial 失败只结束当前请求，后续请求可重新尝试；不自动重放已经失败或执行结果不确定的请求，也不重置 Call 总 deadline。连接 owner 负责取消、关闭和等待读循环退出，NodeSender 保留唯一有界出站队列。见 [按需建连方案](../RFC-CLUSTER-ON-DEMAND-CONNECT.md)。
- NodeSender 的基础出站实现使用固定容量队列和单一 owner worker；`Send` 先 Clone Envelope 并执行非阻塞 admission，队列满返回稳定错误，nil 只表示本地 admission 成功。admission 后的 Transport error/panic 和关闭丢弃进入有界、非阻塞的失败 stream；NodeSender 不在唯一 worker 中运行外部 callback，stream 溢出通过计数暴露，当前 Transport 调用不被强杀。
- 远程 Send 不返回远端 Handler 结果：A 节点负责自己的 admission 和传输诊断，B 节点负责自己的 Handler 执行与错误记录。需要远端返回值或处理错误时使用 Call。
- 失败诊断保留 Transport 的稳定 error identity：当前内存 Transport 的 `ErrNodeNotFound` 表示目标 receiver 不可用，`ErrTransportClosed` 表示本地 Transport 已关闭；未来网络 Transport 不通过字符串猜测原因，也不把一般连接错误断言为“远端进程未启动”。
- Cluster Call 的 PendingCall 只在发起节点保存 Source、远端 Node、Target 和 Command；wire 不携带 Core Source。Response/Error 必须匹配登记的 Session、RemoteNode、Target 和 Command，未知、迟到、重复或错路由 frame 不得完成其它等待。
- 同一 Cluster 使用一个共享 `ClusterCallRegistry` 管理全部 source-bound `ClusterClient` 的 Session 分配和 completion；每个 client 只绑定自己的 Source。`Call` 的等待必须受正数 `MaxCallWait` 约束，调用方 context 只能取消或缩短等待。
- 每条入站连接的 `ConnectionAgent` 只接收已解码的 Request，调用注入的本地 dispatch 并通过该连接的 writer 返回 Call Response/Error；Session 为零的 Send 不回包。Agent 不拥有 socket、FrameCodec、goroutine 或重连状态，Handler 的 panic/未分类 error 转换为安全 StatusError，writer 故障保留为基础设施错误。
- NodeSender 只编排注入的 Transport 调用；内部 outboundPeer 已实现单目标按需建连、复用和恢复，生产 TCP Transport 与目标路由组装仍待后续切片。
- 出站调用与监听身份分离：仅出站进程不要求本地 NodeID 或 open；目标 NodeID 用于地址解析，open 用于接受新入站连接。不强制交换 NodeID，Call 回复沿原连接返回。删除出站 LocalNode 与入站来源 NodeID，入站按各自连接 owner 管理；目标及响应匹配节点仍必须非空。A 主动调用 B 与 B 主动调用 A 使用各自的出站连接，各自的回复沿原连接返回，不进行双向连接仲裁。
- 发送器仅依赖 `Transport.Send(to, envelope)`；注册监听能力与主动发送分离。内存往返替身通过绑定的 writer 完成响应，不通过反向目标路由模拟回复。
- 认证与可寻址 NodeID 分开，可信内网可关闭认证；具体认证协议另行裁决，不要求调用方先注册节点或监听。此裁决取代原先 NodeID 加共享 token 捆绑的握手描述，见 [连接身份方案](../RFC-CLUSTER-CONNECTION-IDENTITY.md)。
- 远端目标使用 NodeID 与 ServiceName/Discovery 或当前远端 Runtime 的临时 ServiceID 分层表达；不引入 Epoch、Lease 或 Generation。Runtime/Cluster 只负责路由、投递和连接恢复，不判断业务资源的唯一所有权、一致性或过期。

## Consequences

- 业务 API 保持 `Send(node, target, ...)` / `Call(node, target, ...)`，连接和 source 路由细节由绑定后的 ClusterClient 隐藏。
- 方向性连接避免额外的“双连接胜负”协议；同一节点可以各自拥有一条 outbound，并各自接受对方的 inbound。
- 丢弃完整但不支持的 Call 请求会使调用方等待超时，因此必须可观测；协议升级应优先在握手阶段阻止长期不兼容连接。
- 自动重连只恢复未来通信，不提供 exactly-once；RequestID、幂等、补偿、资源 fencing 和业务过期判断仍由拥有操作语义的业务/外层模块负责。
