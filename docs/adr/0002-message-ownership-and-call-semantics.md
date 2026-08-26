# ADR 0002: 未编码消息所有权与 Call 的 run-to-completion 语义

## Status

Accepted

## Context

GSR 本地消息不做序列化，`Args []any` 因而可以直接携带 map、slice 和 pointer。若发送方在 admission 后继续修改这些引用，或直接传出 Service 状态的内部引用，接收方就能绕过 Mailbox 与 owner 共享可变状态，破坏 Service 单线程不变量。

另一个问题是 Service Handler 内调用 Call 时是否允许当前 Service 继续处理其它消息。允许重入可以提高吞吐，但会使一次 Handler 在 Call 前后的状态假设失效，也让同一 Service 的修改在逻辑上并发。

SessionID 还能关联请求和响应，但仅凭非零 Session 无法判断一帧是 request 还是 response。若把 Reply 再投递进调用方 Mailbox，又会与同步等待及非重入语义冲突。

## Decision

- Runtime admission 时浅拷贝最外层 Args 切片。
- 引用型参数必须是该 Message 独占的输入快照；admission 成功后发送方不得继续修改或复用它。
- 禁止发送 Service 拥有的可变状态或其内部引用。Handler 把 Args 视为只读，需要保存或修改时复制到自己的状态。
- typed client 负责构造快照、CommandID 映射以及参数和返回值校验。Core 不对任意 `any` 做递归反射复制。
- Service 内 Call 同步等待一次 Reply，并采用 run-to-completion：当前 Handler 结束前，同一 Service 不执行下一条 Message。
- 第一版每个 Running Service 使用一个 Runtime-owned runner goroutine；等待 Call 只阻塞当前 runner，使其它 Service 继续运行。
- Handler 通过 `(any, error)` 返回值完成调用结果；Runtime 自动完成 Reply，不向业务暴露手工或异步 Reply 入口。
- Reply 不作为业务 Command 进入调用方 Mailbox。Runtime 通过 SessionID 找到 PendingCall，并经内部 completion 路径恢复原 Handler。
- Reply、任一相关 Service 停止、Runtime 关闭或有限超时都会完成等待并恰好清理一次 PendingCall。
- 第一版拒绝 self Call 并返回 `ErrCallCycle`。跨 Service 环形 Call 由设计和测试避免；默认优先使用 Send + Result Command。
- ClusterEnvelope 显式携带 request、response 或 error frame kind；Session 仅用于关联。

## Consequences

- 本地路径避免序列化成本，同时消息边界仍保持唯一状态 owner。
- `[]any` 的契约不能完全依赖 Go 类型系统强制，typed client、代码评审和不变量测试是必要门禁。
- Call 前后的 Service 状态保持连续，不会被同一 Service 的其它 Handler 插入修改。
- 同步 Call 会带来等待链和环形依赖风险，因此必须有限超时并谨慎使用。
- Reply completion 成为 Runtime 内部能力，但不会形成第二套业务消息协议或对外暴露 Skynet PTYPE。
