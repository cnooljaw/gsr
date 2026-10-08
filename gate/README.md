# GSR 客户端 Gate

`gate/prodgate` 和 `gate/debuggate` 是 Core 之外的 WebSocket 接入能力。两者复用 `gate.ClientSession` 与 `gate/protocol`，但分别保留生产握手与调试监听的显式配置；不按首帧猜测或自动降级。业务项目自行定义 `.proto`、Agent 和客户端鉴权。

底层组装时，业务 Agent 实现 `runtime.Service`，并按需实现 `runtime.ClientMessageHandler`。使用 `gsr.Run` 的应用入口时，业务 Agent 实现 `gsr.Service`，按需增加 `gsr.ClientMessageHandler`，由应用组装转换成底层接口。Gate 的网络读循环在校验帧后调用 `Runtime.DeliverClientMessage`，使客户端消息进入目标 Agent 的 Mailbox；Agent 使用 `protocol.Dispatcher` 解码并手工注册已实现的强类型 Handler，再通过 `ClientSession.Send` 写回。

底层 Gate 构造函数需要 `*runtime.Runtime`，因为网络 goroutine 不能保存或借用 Service 回调的 `ServiceContext`。应用可在 `gsr.Run` 的 MainService 中调用 `self.OpenDebugGate(ctx, gsr.DebugGateConfig{...})` 或 `self.OpenProdGate(ctx, gsr.ProdGateConfig{...})`：内部创建对应底层 Gate，`NewAgent` 返回应用层 Service，不把 Runtime 投递权限交给业务。`examples/websocket-gate` 仍保留显式组装底层 Runtime 的示例。

通用 `.proto`、生成工具和跨语言认证固定向量位于本目录；仓库根目录的 `buf.yaml`、`buf.gen.yaml` 同时生成框架通用类型和 `examples/websocket-gate` 的 Echo 示例类型。示例保留 Go Agent、启动入口和 TypeScript 客户端，使用这里的公共包。

独立业务项目可以使用 `gate/cmd/protoc-gen-gsr-registry` 生成自己的消息目录；`module_path=<业务 Go module>` 只控制输出到该 module 内的相对路径，生成的目录条目仍引用 GSR 的 `gate/protocol/catalog`。不传 `module_path` 时沿用 `.proto` 源路径输出。

提升为公共包不等于已经完成密码协议审计或生产容量验收。公网直连必须启用 WSS/TLS；由 Ingress 终止 TLS 时可以在受信任的 Pod 内网使用普通 WS。DebugGate 不提供握手、MAC 或 TLS，不得暴露到公网。
