# ADR 0007: CommandContext、Call 上限与超时停止

## Status

Accepted

## Context

GSR 已决定让本地 Send 与 Call 共用 Message，并由 Runtime 自动设置 Source。现有 Handler 草案却没有读取 Source 的入口；Call 虽要求有限等待，也没有定义调用方如何收紧 deadline；OnStart 的自身消息以及 Handler、OnStop 忽略取消后的状态同样需要确定语义。

Skynet 的 dispatch handler 直接接收 `session, source, ...`，`skynet.call` 以 session 关联响应并挂起当前 Lua coroutine，`skynet.ret` 与 `skynet.response` 由业务显式回复。普通 `skynet.call` 本身不强制超时，等待期间同一 Service 还能运行其它消息 coroutine。这些行为依赖 Skynet 的 Lua coroutine 调度、C session 表和 PTYPE response/error 协议。

GSR 使用每 Service 一个 runner，并已经选择 run-to-completion、Handler 返回值自动 Reply 和结构化错误，因此不能直接复制上述机制。Go 也不能安全终止一个不响应取消的 goroutine，超时只能结束等待和回收 Runtime 自有结构，不能伪装成业务代码已经停止。

## Decision

- Runtime 为每条 Message 创建 `CommandContext`。它组合当前 Service 的 Core 能力并提供只读 `Source() ServiceID`；普通 Send/Call 的 Source 由 Runtime 填入当前发起 Service，Timer Command 的 Source 固定为 `ServiceID(0)`，业务不能覆盖。
- 只有明确受信任的 Runtime/Adapter relay 可以转发已有 Runtime-owned Message；业务 API 不接收任意 source 参数，Call 的 PendingCall/Session 不通过 Redirect 转移。
- `CommandContext` 只在当前 Handler 返回前有效，不得保存或交给异步任务；它不提供 `Reply()`。
- Handler 继续通过 `(any, error)` 返回值完成一次自动 Reply。Send 的返回值只进入诊断，不创建 Reply。
- `ServiceContext.Call` 接收非 nil `context.Context`。Runtime Config 提供有限且大于零的 `MaxCallWait`，实际 deadline 取调用方 deadline 与 Runtime 上限中更早者。
- 调用方可以取消或缩短等待，不能请求超过 `MaxCallWait` 的等待，也不能创建无限 PendingCall。
- `Runtime.CreateService(ctx context.Context, service Service) (ServiceID, error)` 接收业务层构造的 Service 实例；`ctx` 原样传给 `OnStart(ctx, serviceContext)`，作为协作取消信号。Runtime 不提供 Bootstrap Service 或 Service 工厂。
- `OnStart` 向自身 Send 返回 `ErrServiceStarting`，向自身 Call 返回 `ErrCallCycle`。OnStart 可以登记 After，但 Timer 最早只能在 Service 进入 Running 后 admission；启动失败时取消。
- OnStart 失败只回滚当前 Service，Runtime 仍保持 Running，可以继续创建其它 Service。失败前分配的 ServiceID 不发布且永不复用；Runtime 至多调用一次 `OnStop` 清理部分初始化资源，OnStop 必须容忍未完整启动的实例。
- Go 无法强杀忽略取消的 OnStart；CreateService 必须等待它真实返回，只有成功返回后才发布 ServiceID。
- Stop 使用统一的有限 shutdown context 等待当前 Handler 和 `OnStop`。进入 Stopping 后立即拒绝新 admission、取消 Timer 和 outgoing PendingCall，并处理队列中未开始的 Send/Call。
- Handler 在 deadline 后仍未返回时，Stop 返回稳定超时错误并让旧 ServiceID 退出普通本地寻址；runner 任务继续被 Runtime 追踪到真实返回，不复用旧 ID，也不声称已经强杀业务代码。
- Runtime 不与超时 Handler 并发执行 `OnStop`。Handler 后续返回后，runner 至多调用一次 `OnStop`，其 shutdown context 已取消；如果 `OnStop` 仍不返回，继续作为残留任务被 Inspection 观测。
- 重复 Stop 复用同一个停止流程和已保存结果，不并发启动第二次 Handler 清理或 `OnStop`。
- Core admission 只复制 `Args` 外层切片。引用型参数的复制深度和传值语义由具体 typed client 保证，Core 不做通用递归深拷贝。

## Consequences

- Handler 能可靠判断当前本地消息来源，同时业务没有伪造 Source 或异步 Reply 的旁路。
- 所有 Call 都有确定上限，但业务可以使用 context 表达更短的请求 deadline 和 cancellation。
- API 比 Skynet 多一个显式 context，并取消手工 response；代价是更符合 Go 的取消、错误和静态接口习惯。
- 超时 Service 会从业务寻址中隔离，但进程仍可能保留不响应取消的 goroutine；Inspection、日志和泄漏门禁必须如实报告这一事实。
- 通用 Core 无法在类型层强制任意 `any` 的深层不可变性；typed client、测试和代码评审共同承担消息快照门禁。
