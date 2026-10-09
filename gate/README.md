# GSR 客户端 Gate

`gate` 是 Core 之外的统一 WebSocket 接入能力。默认 `ProtocolMAC` 使用 X25519 握手、seq 和 MAC；显式 `ProtocolPlain` 直接传输 Envelope，仅绑定回环/私有 IP。每个监听固定一种模式，不猜首帧或降级。TLS 独立配置；业务项目自行定义协议、Agent 和 token 鉴权。

普通应用使用 `gsr.Run`：业务 Agent 实现 `gsr.Service`，按需增加 `gsr.ClientMessageHandler`，由应用组装转换成底层接口。只有基础设施集成才直接使用 `runtime.Service` 和 `runtime.ClientMessageHandler`。Gate 的网络读循环在校验帧后调用 `Runtime.DeliverClientMessage`，使客户端消息进入目标 Agent 的 Mailbox；Agent 使用 `protocol.Dispatcher` 解码并手工注册已实现的强类型 Handler，再通过 `ClientSession.Send` 写回。

底层 Gate 构造函数需要 `*runtime.Runtime`，因为网络 goroutine 不能保存或借用 Service 回调的 `ServiceContext`。应用可在 `gsr.Run` 的 MainService 中调用 `self.OpenGate(ctx, gsr.GateConfig{...})`：内部创建对应底层 Gate，`NewAgent` 返回应用层 Service，不把 Runtime 投递权限交给业务。`examples/websocket-gate` 已使用 `gsr.Run` 和 MainService，Echo Agent 实现应用层接口。

通用 `.proto`、生成工具和跨语言认证固定向量位于本目录；仓库根目录的 `buf.yaml`、`buf.gen.yaml` 同时生成框架通用类型和 `examples/websocket-gate` 的 Echo 示例类型。示例保留 Go Agent、启动入口和 TypeScript 客户端，使用这里的公共包。

独立业务项目可以使用 `gate/cmd/protoc-gen-gsr-registry` 生成自己的消息目录；`module_path=<业务 Go module>` 只控制输出到该 module 内的相对路径，生成的目录条目仍引用 GSR 的 `gate/protocol/catalog`。不传 `module_path` 时沿用 `.proto` 源路径输出。

提升为公共包不等于已经完成密码协议审计或生产容量验收。公网直连必须启用 WSS/TLS；由 Ingress 终止 TLS 时可以在受信任的 Pod 内网使用普通 WS。Plain 不提供握手或 MAC，即使配置 TLS 也仅用于可信内网；两种模式复用统一 TCP 限额、诊断和关闭。

## Agent 中的协议分发

两个 Dispatcher 处理不同的消息入口，不串联使用：

- `gsr.Dispatcher`：在 `Service.Handle` 内按 Core `CommandID` 分发本地或跨节点 Send/Call，调用强类型业务 Handler。
- `protocol.Dispatcher`：在 `HandleClientMessage` 内按 Envelope 的 `msg_id` 解码客户端 Protobuf 并调用强类型协议 Handler，封装客户端响应和推送。

两种入口都由同一个 Agent Mailbox 和 runner 串行执行，不因使用两个分发组件增加执行单元。Echo Agent 的普通 Command 只处理断线通知，因此直接在显式 `Handle` 中处理，不需要额外创建 `gsr.Dispatcher`；有多个普通业务命令时再按默认规范使用它。

应用 Agent 用具名字段持有 `*protocol.Dispatcher[gsr.CommandContext]`。共享 Registry 在应用组装时合并生成目录；Agent 在 `OnStart` 中将目录和自己的 ClientSession 交给 Dispatcher，并注册已实现的业务 Handler：

```go
dispatcher, err := protocol.NewDispatcher[gsr.CommandContext](registry, agent.client)
if err != nil {
    return err
}
if err := protocol.Register(dispatcher, agent.OnEcho); err != nil {
    return err
}
agent.dispatcher = dispatcher
```

构造器内部保证解码与写回使用同一目录，业务不创建或拼接协议 Session。客户端 Handler 使用 `ctx.Response/Push`；普通 Command 等其它业务流程需要主动推送时，使用 `agent.dispatcher.Push(msgID, message)`，仍在 Agent runner 内调用。

Gate 的两种模式在有限期限内等待自身发布或 Mailbox 恢复；仅重试尚未接纳的连接事件。Agent 构造、OnStart 失败以及已接纳 Call 的超时不会触发重复创建，失败只关闭该连接，原始原因留在 Gate 日志。

`HandleClientMessage` 只把当前消息的上下文和 Envelope 字节交给 Dispatcher，不重复解码：

```go
func (agent *echoAgent) HandleClientMessage(self gsr.CommandContext, frame []byte) error {
    return agent.dispatcher.HandleFrame(self, frame)
}

func (agent *echoAgent) OnEcho(
    self gsr.CommandContext,
    ctx protocol.Context,
    request *echopb.EchoRequest,
) error {
    // 需要访问其它 Service 时，在这里使用 self.CallName / self.Cluster().Call 等能力。
    return ctx.Response(
        uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
        &echopb.EchoResponse{Text: request.GetText()},
    )
}
```

这里 `self` 是当前 Agent 的身份与 Service 能力；`ctx` 负责当前客户端 RequestID、Response 和 Push。不使用 self 的 Handler 可将该参数写为 `_`。Dispatcher 原样传递每次收到的 self，不保存上下文，也不建立额外 Mailbox、runner 或 goroutine；两种上下文都不得存入 Agent 字段或用于异步任务。注册发生在初始化阶段，不从其它 goroutine 修改 Handler 表。

普通 Core Command 仍由 Agent 显式声明的 `Handle` 处理，不进入客户端协议 Dispatcher。完整代码见 [Echo Agent](../examples/websocket-gate/cmd/debugserver/agent.go)；现有 [应用接入测试](../gate_plain_test.go) 还验证了强类型协议 Handler 通过 `CallName` 调用业务 Service、业务收到 AgentID 来源，以及客户端 request_id 回传。
