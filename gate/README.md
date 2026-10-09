# GSR 客户端 Gate

`gate` 是 Core 之外的统一 WebSocket 接入能力，只收发不透明字节，不依赖 Protocol 或 Protobuf。默认 `ProtocolMAC` 使用固定 33 字节 X25519 握手、seq 和 MAC；显式 `ProtocolPlain` 直接传输字节消息，仅绑定回环/私有 IP。监听模式固定，不猜首帧或降级。

应用在 `gsr.Run` 的 MainService 中通过 `self.OpenGate` 启动 Gate。Gate 为每条连接创建业务 Agent；读循环剥离安全封装后，通过 `Runtime.DeliverClientMessage` 将字节快照直接投递到 Agent Mailbox。Agent 持有 `ClientSession`，通过 `Send([]byte)` 写回或 `Close()` 关闭连接，不接触 socket 或连接表。

Send 接纳时复制字节快照，调用方之后可以复用缓冲区。队列有界且顺序发送；成功只表示本地接纳，不保证送达、业务执行或执行一次。nil 和空切片均表示零字节消息，仍收取队列条目开销。

TLS 独立配置：公网直连必须启用 WSS/TLS；Ingress 已终止 TLS 时可在可信内网使用 WS。Plain 不提供握手或 MAC，即使配置 TLS 也仅用于可信内网。两种模式共用 TCP 限额、诊断和关闭流程；公共包不等于已经完成密码协议审计或生产容量验收。

业务 Agent 是 Gate 与 [Protocol](../protocol/README.md) 的组合点。Protocol 理解 Envelope、编解码和分发；Gate 只负责连接与字节传输。认证固定向量留在 Gate；通用 `.proto`、生成类型和生成工具位于同级 `protocol/`。完整接线见 [WebSocket 示例](../examples/websocket-gate/README.md)。
