# ADR 0005: Cluster wire、连接与重连

## Status

Accepted

## Context

GSR Cluster 需要保持 Skynet 风格的简洁调用面，同时明确 Go 的编码集合、注册时机、异常帧边界和连接 owner。若把每条连接都建模为同一 Node 的竞争全双工通道，就必须增加复杂的重复连接仲裁；Skynet 实际采用每个目标节点一个 sender、每条入站 socket 一个 agent 的方向性组织。

## Decision

- ClusterEnvelope 显式包含 request/response/error frame kind、ClusterSessionID、Target ServiceID/ServiceName、CommandID，以及 payload 或结构化 ErrorClass/ErrorCode/Message。它是逻辑消息值，不携带业务或 wire version；wire 直接编码 `StatusError` 的三个字段，不在 Core 另设重复的 ErrorStatus 类型。
- 默认动态值集合包含 nil、bool、整数、浮点数、string、`[]byte`、list 和 string-key map；领域 struct 通过启动阶段注册的 Command codec 转换。
- 不编码任意 pointer、func、chan、unsafe pointer、循环引用或进程内资源句柄。
- codec 与 Command 在启动阶段注册；重复注册是 programmer error，panic 并阻止启动。
- 未来 Transport/Codec framing 或连接握手负责 wire version；在完整且边界可信时，未知 framing version、未知 Command 和解码失败帧被丢弃，并记录限频诊断和指标。
- 非法长度、超过上限、无法恢复帧边界或认证失败时关闭连接。
- 每个远端 Node 只有一个 outbound NodeSender，并发建连请求合并；每条 inbound socket 使用独立 ConnectionAgent，响应沿原连接返回。
- 断线后 NodeSender 自动重连，但不自动重放已经失败或执行结果不确定的业务请求。
- NodeID/token 握手可配置；可信内网可以关闭，开启后使用 NodeID 加共享 token 的简单校验。
- 远端目标使用 NodeID 与 ServiceName/Discovery 或当前远端 Runtime 的临时 ServiceID 分层表达；不引入 Epoch、Lease 或 Generation。Runtime/Cluster 只负责路由、投递和连接恢复，不判断业务资源的唯一所有权、一致性或过期。

## Consequences

- 业务 API 保持 `Send(node, target, ...)` / `Call(node, target, ...)`，连接和 source 路由细节由绑定后的 ClusterClient 隐藏。
- 方向性连接避免额外的“双连接胜负”协议；同一节点可以各自拥有一条 outbound，并各自接受对方的 inbound。
- 丢弃完整但不支持的 Call 请求会使调用方等待超时，因此必须可观测；协议升级应优先在握手阶段阻止长期不兼容连接。
- 自动重连只恢复未来通信，不提供 exactly-once；RequestID、幂等、补偿、资源 fencing 和业务过期判断仍由拥有操作语义的业务/外层模块负责。
