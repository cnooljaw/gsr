# GSR 客户端 Protocol

`protocol` 负责 Protobuf Envelope 的编解码、校验和强类型分发，不依赖 Gate，不拥有连接、Mailbox 或 goroutine。它只依赖 `ClientSender.Send([]byte) error`；业务 Agent 可以直接传入 `gate.ClientSession`，不增加适配层。

通用 `.proto`、生成的 Go 类型和目录生成工具分别位于 `proto/`、`gen/` 和 `cmd/`。根目录 Buf 配置同时生成通用类型和 Echo 示例类型。独立业务项目使用目录生成插件时，`module_path=<业务 Go module>` 控制输出相对路径，生成目录仍引用 GSR 的 `protocol/catalog`。

## 职责与代码入口

| 职责 | 代码入口 |
| --- | --- |
| Envelope 与通用消息定义 | [proto/gsr/common/v1/common.proto](proto/gsr/common/v1/common.proto) |
| 生成类型、生成消息目录 | `gen/` |
| Registry：协议目录与编解码校验 | [registry.go](registry.go) |
| Dispatcher：构造时显式绑定 Handler 与入站分发 | [dispatcher.go](dispatcher.go) |
| Context：request_id 关联、Response / Push | [context.go](context.go) |
| ClientSender：编码后字节发送窄接口 | [outbound.go](outbound.go) |
| 目录生成工具 | `cmd/protoc-gen-gsr-registry/` |

`ClientSession` 不属于此包：它由 Gate 提供，包含字节发送和连接关闭能力。Protocol 只使用其中的 Send，不读取 socket、不关闭连接，也不判断 MAC / seq。Agent 直接传入 ClientSession 即可，Go 接口由方法集合满足，不需要额外 wrapper。

## Agent 中的协议分发

两个 Dispatcher 处理不同的消息入口，不串联使用：

- `gsr.Dispatcher`：在 `Service.Handle` 内按 Core `CommandID` 分发本地或跨节点 Send/Call，调用强类型业务 Handler。
- `protocol.Dispatcher`：在 `HandleClientMessage` 内按 Envelope 的 `msg_id` 解码客户端 Protobuf 并调用强类型协议 Handler，封装客户端响应和推送。

两种入口都由同一个 Agent Mailbox 和 runner 串行执行，不因使用两个分发组件增加执行单元。Echo Agent 用具名 `gsr.Dispatcher` 处理 EchoService 发来的推送 Command；断线通知仍在显式 `Handle` 中处理。客户端 Echo 请求则经协议 Handler 调用 EchoService，两种编号空间不混用。

应用 Agent 用具名 `protocolDispatcher` 字段持有 `*protocol.Dispatcher[gsr.CommandContext]`，普通 Command 的组件命名为 `commandDispatcher`。共享 Registry 在应用组装时只传业务生成目录，构造器自动纳入框架通用目录：

```go
registry, err := protocol.NewRegistry(echopb.MessageCatalogGsrExampleEchoV1Echo)
```

调用方处理构造错误后再使用 Registry。通用 ErrorResponse 使用固定编号 9001，业务不得覆盖或重映射；重复传入通用目录与其它编号、类型冲突一样，在构造时拒绝，不提供兼容去重。

Agent 构造时将 Registry、自己的 ClientSession 和显式 Handler 表一次性交给 Dispatcher：

```go
agent.protocolDispatcher = protocol.MustDispatcher[gsr.CommandContext](
    registry,
    agent.client,
    map[uint32]any{
        uint32(echopb.MsgId_C2S_ECHO_REQUEST): agent.OnEcho,
        uint32(echopb.MsgId_C2S_ECHO_NOTIFY):  agent.OnEchoNotify,
    },
)
```

`.proto` 负责编号与 payload、方向、消息种类；表只负责编号到业务函数的接线。构造器复制表，校验编号已定义且允许客户端入站，并校验 `func(Scope, Context, *具体Protobuf消息) error` 的完整签名和对应 payload 类型。构造后只读，没有逐条 Register 或运行期修改。静态接线用 MustDispatcher，错误时 panic；需要返回构造错误时使用同样接收三参数的 NewDispatcher。协议可以先定义，表只绑定已实现的 Handler；未绑定消息保持既有错误行为。运行时仍使用 reflect.Call，不逐消息重新分析签名。

构造器内部保证解码与写回使用同一目录，业务不创建或拼接协议 Session。客户端 Handler 使用 `ctx.Response/Push`；普通 Command 等其它业务流程需要主动推送时，使用 `agent.protocolDispatcher.Push(msgID, message)`，仍在 Agent runner 内调用。Echo 的通知先 Send 到业务 Service，业务再 Send 回 Agent；推送不依赖原请求的 `protocol.Context`。

Gate 的两种模式在有限期限内等待自身发布或 Mailbox 恢复；仅重试尚未接纳的连接事件。Agent 构造、OnStart 失败以及已接纳 Call 的超时不会触发重复创建，失败只关闭该连接，原始原因留在 Gate 日志。

`HandleClientMessage` 只把当前消息的上下文和 Envelope 字节交给 Dispatcher，不重复解码：

```go
func (agent *echoAgent) HandleClientMessage(self gsr.CommandContext, frame []byte) error {
    return agent.protocolDispatcher.HandleFrame(self, frame)
}

func (agent *echoAgent) handlePushEcho(_ gsr.CommandContext, text string) error {
    // 普通 Command 入口没有当前客户端请求，用独立 Push 能力写回。
    return agent.protocolDispatcher.Push(
        uint32(echopb.MsgId_S2C_ECHO_PUSH),
        &echopb.EchoPush{Text: text},
    )
}
```

这里 `self` 是当前 Agent 的身份与 Service 能力；`ctx` 负责当前客户端 RequestID、Response 和 Push。不使用 self 的 Handler 可将该参数写为 `_`。Dispatcher 原样传递每次收到的 self，不保存上下文，也不建立额外 Mailbox、runner 或 goroutine；两种上下文都不得存入 Agent 字段或用于异步任务。Handler 表在构造时完成绑定，之后不从其它 goroutine 修改。

普通 Core Command 仍由 Agent 显式声明的 `Handle` 处理，不进入客户端协议 Dispatcher。完整代码及两条运行链路见 [Echo 示例](../examples/websocket-gate/README.md#agent-的两个消息入口)；[示例测试](../examples/websocket-gate/cmd/debugserver/agent_test.go) 通过真实 Gate 验证 Call 响应、业务拒绝和 Send 回 Agent 后的 Push。[应用接入测试](../gate_plain_test.go) 另有来源身份契约覆盖，不在示例中重复搭建该底层用例。

## 分发性能基线

在仓库根目录运行局部 benchmark，不启动 Gate 或外部依赖：

```sh
go test ./protocol -run '^$' -bench '^BenchmarkDispatcherHandleFrame$' -benchmem -benchtime=300ms
```

[benchmark](dispatcher_benchmark_test.go) 使用 `hello` 小消息和接口型 Scope；构造不计入分发耗时，写回只累计已编码帧字节，不保留队列或二次解码。2026-10-10 的 Apple M2 / darwin arm64 短时样本：

| 路径 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| NotifyDecode：Envelope 与 payload 解码、目录校验 | 239.3 | 160 | 4 |
| NotifyDispatch：解码、Context 创建和反射 Handler 分发 | 523.1 | 240 | 9 |
| RequestResponse：解码、反射 Handler 分发、响应编码与写回调用 | 787.3 | 424 | 13 |

解码基线不是语义完整的手写热点实现；与 NotifyDispatch 的差值包含 Context、查表和反射等成本，不能全部归因于 `reflect.Call`。这些数据不包含 Gate、MAC、Mailbox、socket I/O 或业务 Service Call，也不是整机容量验收。默认保留显式 Handler 表；只有真实业务 profile 确认分发成为热点后，再讨论局部优化，不据此增加新的注册框架或 Fast Handler API。
