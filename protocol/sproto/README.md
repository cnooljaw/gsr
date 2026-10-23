# GSR Sproto 客户端分发

本包使用独立修复库 `github.com/cnooljaw/gosproto v1.0.1`，只处理原生 Sproto，不依赖 Protobuf、Gate、Core 或 GSR 应用上下文。业务 Agent 透传自己的 `gsr.CommandContext`，并借用连接的 `Send([]byte) error`。

```go
agent.protocolDispatcher = sproto.MustDispatcher[gsr.CommandContext](
    sproto.Config{C2S: c2s.Protocols, S2C: s2c.Protocols},
    client,
    map[string]any{"echo": agent.handleEcho},
)
```

Request Handler 的签名为 `func(gsr.CommandContext, sproto.Context, *c2s.EchoRequest) error`。无 Request 内容的协议省略最后参数。协议与 Handler 在构造时检查并复制；已定义但未实现的协议可以不绑定。C2S 与 S2C 分别限定方向，编号可以复用，原生 tag 零有效。不使用 MessageRoute、统一 Envelope 或通用 SchemaProvider。

- `Context.Session()` 为客户端关联号；本实现需要响应时使用正 int32，不回绕，Notify 不带 session。
- `Context.Response(*对应Response)` 自动关联协议和原 session，成功只表示字节接纳。一次 Handler 最多接纳一次响应，Context 离开 Handler 后失效。
- `Context.Push(name, request)` 或 `Dispatcher.Push(name, request)` 使用 S2C 定义且不登记响应会话，只在 Agent runner 内调用。
- 首版不提供服务端请求客户端并等待回复的机制；S2C 定义带 Response 时构造失败。
- 打包帧默认最多 1 MiB；编解码默认最多 8 MiB、深度 64、全结构字段与集合元素合计 100000，可通过 Config 显式设置有限预算。
- 业务失败由该协议自己的 Response 表达。坏包、未知协议、Handler error 或遗漏响应由 HandleFrame 返回错误；协议不持有 Close，Agent 负责关闭连接并记录诊断，使客户端结束等待。不编码 Protobuf ErrorResponse，不透传内部错误作为业务回复。

`Envelope`、Protobuf request_id 和 Core/Cluster Session 的语义保持独立。源码与 Go/TS 联调见 [WebSocket Echo](../../examples/websocket-gate/README.md)及 [接入计划](../../docs/SPROTO-INTEGRATION.md)。
