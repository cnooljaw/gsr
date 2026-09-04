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
- 断线后 NodeSender 自动重连，但不自动重放已经失败或执行结果不确定的业务请求。
- NodeSender 的基础出站实现使用固定容量队列和单一 owner worker；`Send` 先 Clone Envelope 并执行非阻塞 admission，队列满返回稳定错误，nil 只表示本地 admission 成功。admission 后的 Transport error/panic 和关闭丢弃进入有界、非阻塞的失败 stream；NodeSender 不在唯一 worker 中运行外部 callback，stream 溢出通过计数暴露，当前 Transport 调用不被强杀。
- 远程 Send 不返回远端 Handler 结果：A 节点负责自己的 admission 和传输诊断，B 节点负责自己的 Handler 执行与错误记录。需要远端返回值或处理错误时使用 Call。
- 失败诊断保留 Transport 的稳定 error identity：当前内存 Transport 的 `ErrNodeNotFound` 表示目标 receiver 不可用，`ErrTransportClosed` 表示本地 Transport 已关闭；未来网络 Transport 不通过字符串猜测原因，也不把一般连接错误断言为“远端进程未启动”。
- Cluster Call 的 PendingCall 只在发起节点保存 Source、远端 Node、Target 和 Command；wire 不携带 Core Source。Response/Error 必须匹配登记的 Session、RemoteNode、Target 和 Command，未知、迟到、重复或错路由 frame 不得完成其它等待。
- 当前 NodeSender 只编排注入的 Transport 调用；TCP stream、并发建连合并和自动重连属于后续 Connection/Transport 子切片，在此基础行为上扩展。
- NodeID/token 握手可配置；可信内网可以关闭，开启后使用 NodeID 加共享 token 的简单校验。
- 远端目标使用 NodeID 与 ServiceName/Discovery 或当前远端 Runtime 的临时 ServiceID 分层表达；不引入 Epoch、Lease 或 Generation。Runtime/Cluster 只负责路由、投递和连接恢复，不判断业务资源的唯一所有权、一致性或过期。

## Consequences

- 业务 API 保持 `Send(node, target, ...)` / `Call(node, target, ...)`，连接和 source 路由细节由绑定后的 ClusterClient 隐藏。
- 方向性连接避免额外的“双连接胜负”协议；同一节点可以各自拥有一条 outbound，并各自接受对方的 inbound。
- 丢弃完整但不支持的 Call 请求会使调用方等待超时，因此必须可观测；协议升级应优先在握手阶段阻止长期不兼容连接。
- 自动重连只恢复未来通信，不提供 exactly-once；RequestID、幂等、补偿、资源 fencing 和业务过期判断仍由拥有操作语义的业务/外层模块负责。
