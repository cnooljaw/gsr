# ADR 0009: 受控 Redirect 与 Source 来源语义

## Status

Accepted

## Context

Skynet 的 `skynet.redirect(dest, source, typename, ...)` 同时承担了几种不同职责，不能把它简单翻译成一个面向业务的 `Redirect` 方法：

- `service/gate.lua` 将尚未释放的 socket buffer 转交给 agent，重点是消息内存的所有权转移；它把 `c.client` 作为 source，若为零则由底层回退为当前 gate context。
- `service/multicastd.lua` 将同一发布消息扇出给多个订阅者，并保留发布者 source，同时依赖 C/Lua 层的引用计数和 buffer 生命周期。
- `service/cdummy.lua`、`service/cslave.lua` 把 Harbor 控制消息转发给 `harbor_service`，用 source 表达基础设施中的登记者或句柄。
- `examples/simplemonitor.lua` 把已退出服务的地址作为 source 通知观察者，此时 source 更接近事件主体而不是当前发送者。
- `service/clusterproxy.lua` 的普通请求/响应代理使用 `send`、`rawcall` 和 session bridge，而不是把 Call 直接交给 `redirect`。

这些行为依赖 Skynet 的 Lua coroutine、PTYPE、C 层 buffer ownership、session 表和手工 response。GSR 的 Core Message 使用 Go 的 `[]any` 快照和 Runtime-owned PendingCall，不能暴露可任意伪造 source 的入口，也不能把共享可变指针或 C 风格的零拷贝生命周期带入 Core。

## Decision

- 普通 Service `Send`/`Call` 由 Runtime 创建新 Message，并把 Source 写为当前发起 ServiceID；业务 API 不接收 source 参数。
- Timer 到期时由 Runtime 创建普通 Command，但 Source 固定为 `ServiceID(0)`。创建 Timer 的 Service 由 Timer owner 记录，用于取消权限和诊断，不伪装成消息来源。
- Core 内部对 Service、Runtime-owned 注入和 Timer 使用不同的显式投递入口；`nil` 不是 `Source == 0` 的业务语义或可复用哨兵。
- 宿主/受信任基础设施通过 `Runtime.Send` / `Runtime.Call` 注入本地消息，Source 固定为零。基础设施 Call 新建本地 Session，复用 Mailbox 与 PendingCall；不转移 ClusterSession，不绕过 Handler，最大等待受 MaxCallWait 限制。业务 Service 只获得 ServiceContext，继续使用自身绑定的 Send/Call。
- `Redirect`/`Relay` 不是首版普通业务 API。只有 Runtime 或明确受信任的 Adapter/Transport/Cluster 组件可以获得受控能力，转发 Runtime-owned Message 或 relay token；业务不能传入任意 ServiceID、覆盖 Source 或保存可变内部消息。
- 受控 relay 可以按固定策略保留同一 Runtime 内已有 Message 的原始 Source，以支持透明代理和基础设施转发；若需要表达当前 relay、事件主体、认证主体或远端原始来源，使用显式 typed 参数或 Cluster/Transport metadata，不复用 Source 伪装。
- 首版 Redirect 只覆盖单向消息/事件/控制面转发。普通请求/响应代理必须向下游发起新的 Call（Call-through），再沿代理自己的 PendingCall 返回结果；不转移原 PendingCall、Session、Reply 所有权，不承诺自动重试或 exactly-once。
- 多播/广播属于外层能力：为每个目标分别执行有界 admission，并独立处理失败，不把 fan-out 变成 Core 的无限转发。
- Transport 的零拷贝和 buffer ownership transfer 只能由 Transport/入站连接 owner 私有实现完成；Core Message 中的引用型 Args 仍必须满足消息独占快照契约。

## Consequences

- `CommandContext.Source()` 不是任意可填写的身份字段：普通消息表示当前本地发起者，Timer 消息可以是零值，受控 relay 才可能保留已有原始 Source。
- API 保持简单，业务不能通过 source 伪造权限或事件主体；需要授权、审计或业务主体时必须显式建模。
- Redirect 的能力边界与 Call 的 Reply completion 分离，避免在第一版引入 session 转移、重复完成和超时清理的复杂状态机。
- 如果未来出现真实的基础设施调用者，先增加针对该调用者的内部 capability 和测试，再决定是否把稳定名称加入公开 API；没有调用者时不预建通用 Redirect 框架。
