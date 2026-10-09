# GSR 客户端 Protocol

`protocol` 负责 Protobuf Envelope 的编解码、校验和强类型分发，不依赖 Gate，不拥有连接、Mailbox 或 goroutine。它只依赖 `ClientSender.Send([]byte) error`；业务 Agent 可以直接传入 `gate.ClientSession`，不增加适配层。

通用 `.proto`、生成的 Go 类型和目录生成工具分别位于 `proto/`、`gen/` 和 `cmd/`。根目录 Buf 配置同时生成通用类型和 Echo 示例类型。独立业务项目使用目录生成插件时，`module_path=<业务 Go module>` 控制输出相对路径，生成目录仍引用 GSR 的 `protocol/catalog`。

## 职责与代码入口

| 职责 | 代码入口 |
| --- | --- |
| Envelope 与通用消息定义 | [proto/gsr/common/v1/common.proto](proto/gsr/common/v1/common.proto) |
| 生成类型、生成消息目录 | `gen/` |
| Registry：协议目录与编解码校验 | [registry.go](registry.go) |
| Dispatcher：手动注册 Handler 与入站分发 | [dispatcher.go](dispatcher.go) |
| Context：request_id 关联、Response / Push | [context.go](context.go) |
| ClientSender：编码后字节发送窄接口 | [outbound.go](outbound.go) |
| 目录生成工具 | `cmd/protoc-gen-gsr-registry/` |

`ClientSession` 不属于此包：它由 Gate 提供，包含字节发送和连接关闭能力。Protocol 只使用其中的 Send，不读取 socket、不关闭连接，也不判断 MAC / seq。Agent 直接传入 ClientSession 即可，Go 接口由方法集合满足，不需要额外 wrapper。

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
