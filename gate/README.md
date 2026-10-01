# GSR 客户端 Gate

`gate/prodgate` 和 `gate/debuggate` 是 Core 之外的 WebSocket 接入能力。两者复用 `gate.ClientSession` 与 `gate/protocol`，但分别保留生产握手与调试监听的显式配置；不按首帧猜测或自动降级。业务项目自行定义 `.proto`、Agent 和客户端鉴权。

业务 Agent 实现普通 `runtime.Service`，并按需实现 `runtime.ClientMessageHandler`。Gate 的网络读循环在校验帧后调用 `Runtime.DeliverClientMessage`，使客户端消息进入目标 Agent 的 Mailbox；Agent 使用 `protocol.Dispatcher` 解码并手工注册已实现的强类型 Handler，再通过 `ClientSession.Send` 写回。

当前 Gate 构造函数需要 `*runtime.Runtime`，因为网络 goroutine 不能保存或借用 Service 回调的 `ServiceContext`。`gsr.Run` 在内部持有 Runtime，尚未提供 Gate 所需的窄投递接线。`examples/websocket-gate` 因此继续显式组装底层 Runtime；不要为了省去该组装步骤向普通业务 Service 暴露 Runtime。后续接入方式需单独裁决。

通用 `.proto`、生成工具和跨语言认证固定向量位于本目录；仓库根目录的 `buf.yaml`、`buf.gen.yaml` 同时生成框架通用类型和 `examples/websocket-gate` 的 Echo 示例类型。示例保留 Go Agent、启动入口和 TypeScript 客户端，使用这里的公共包。

独立业务项目可以使用 `gate/cmd/protoc-gen-gsr-registry` 生成自己的消息目录；`module_path=<业务 Go module>` 只控制输出到该 module 内的相对路径，生成的目录条目仍引用 GSR 的 `gate/protocol/catalog`。不传 `module_path` 时沿用 `.proto` 源路径输出。

提升为公共包不等于已经完成密码协议审计或生产容量验收。公网直连必须启用 WSS/TLS；由 Ingress 终止 TLS 时可以在受信任的 Pod 内网使用普通 WS。DebugGate 不提供握手、MAC 或 TLS，不得暴露到公网。
