# 向业务交付：错误响应后关闭连接

自 GSR v1.0.4 提供，v1.0.3 及更早标签不包含此能力。业务项目应升级到 v1.0.4 或后续正式版本；不要修改 module cache 或相邻框架源码绕过版本。

## GSR 提供什么

- `gate.ClientSession.SendAndClose(frame)`：复制并接纳唯一末帧，封闭新发送和新的入站接纳，按此前 FIFO 写出后关闭。返回 nil 仅表示本地接纳，不保证客户端收到或处理。
- `gate.Config` / `gsr.GateConfig.SendAndCloseTimeout`：零使用一秒总预算，负数拒绝；从接纳起覆盖正在写的旧帧、队列、末帧及关闭，不逐帧续期。显式 Close、对端断开、写失败、超时或停机可提前终止。
- 最后一帧仍受原帧长/队列字节预算限制，满载或超大帧返回 ErrClientQueueFull / ErrFrameTooLarge，同时封闭并异步关闭；不抢占旧队列发送、不重试、不为末帧扩容。
- Closing 期间 Send/SendAndClose 返回 ErrClientSessionClosed；重复请求不追加帧或延长期限。Close 仍是可立即中止的幂等关闭。
- 断开继续沿原 ClientDisconnectedCommand 通知 Agent，没有第二套事件或业务回调。入站先接纳的消息可能已在 Agent Mailbox，Gate 无法撤回。

## Protobuf 接线

在 Agent 的构造/OnStart 组装 Dispatcher，策略只在当前失败消息的同步处理期间运行。示意片段中的 `agent`、`registry`、`handlers` 和 `shouldClose` 属于业务：

```go
dispatcher, err := protocol.NewDispatcher[gsr.CommandContext](
    registry, client, handlers,
    protocol.WithErrorPolicy(func(self gsr.CommandContext, msgID uint32, cause error) protocol.ErrorAction {
        if shouldClose(msgID, cause) {
            agent.closing = true
            return protocol.ReplyErrorAndClose
        }
        return protocol.ReplyError
    }),
)
```

无策略时保持既有自动安全回错和 Send-only 接线；显式配置时 sender 必须同时实现 SendAndClose/Close，否则构造返回 ErrCloseUnsupported，Must 构造按约定 panic。策略不覆盖安全错误码/文案，不把 SQL、Token、内部错误文本发给客户端。无效 ErrorAction 会关闭连接并保留策略错误供诊断。

策略只处理尚未成功响应的失败，包括 Envelope 无法解码（msgID 为零）、未知消息、解码/方向/编号错误、Handler error 和遗漏响应。可关联请求得到唯一安全 ErrorResponse，保留原 RequestID；未知编号但有非零 RequestID 可以关联。无法解码 Envelope、RequestID 为零或已知 Notify 时只 Close，不信任部分解码出的编号。编码失败也只 Close。终止发送失败直接返回聚合错误，不再回第二份错误。

已经成功响应的请求不调用策略、不追加错误；普通 Response 接纳失败仍沿既有自动错误路径处理。策略不是“所有错误的连接拦截器”。Handler panic 继续归 Runtime recovery，不保证最后响应。

## 业务必须做什么

1. 决定哪些错误需要关闭。认证失败、进入结果未知、非法后端响应可以按业务规则关闭；正常繁忙、目标失效或其它可恢复拒绝可保留连接。GSR 不识别这些领域条件。
2. 选择关闭时，在 Agent owner 中设置 closing。HandleClientMessage 开头检查该状态，丢弃已入邮箱的后续客户端业务。不要把它用来丢弃 ClientDisconnectedCommand 或其它必要清理 Command。
3. 不要在策略或 SendAndClose 接纳后立即 Exit，也不要马上 Close。若 OnStop 会 Close，提前 Exit 会中止末帧。保持 Agent 存活，等现有断线通知后清理再退出；节点停机可以提前中止。
4. Leave、条件 Unbind、旧登录不影响新登录、未知 Join 的恢复与幂等继续由业务定义。超时不代表业务操作已回滚，连接关闭不替代事务或结果恢复。
5. OnStop 的 Close 保持幂等，能处理未完整启动、对端先断开和节点停机；不在 GSR 回调外保存 self/context，不自行启动 goroutine 或延迟关闭 Timer。

如果业务已经发送自己的强类型拒绝 Response，这次请求不会再触发自动错误策略。不要先调用 Response/Send，再返回错误期待补一个终止错误；需要强类型终止响应/踢线 Push 的协议快捷能力另行设计，本次只提供字节 SendAndClose。

## 升级与验收

ClientSession 新增方法，业务自行实现的会话/测试替身需补实现；不得简单使用 `Send; Close` 冒充顺序关闭。NewDispatcher/MustDispatcher 新增 variadic options，普通调用保持不变；持有其精确函数类型的代码需调整。

业务升级后删除 failureCloser、固定 200ms Timer 及辅助 Service 创建失败分支。验证客户端实际收到唯一安全错误并断线，而非只断言 SendAndClose 返回 nil；同时验证普通拒绝仍可继续请求、closing 拦截排队消息、断线清理幂等及旧登录不影响新登录。

GSR 验证传输顺序、快照、容量、总预算、并发接纳、关闭与协议适配；gsrdemo/业务项目验证领域规则、真实节点与客户端流程。Sproto 可复用字节能力，但不生成 Protobuf ErrorResponse，本次不增加它的错误策略 API。
