# ADR 0004: Service runner、生命周期与 Timer

## Status

Accepted

## Context

Service 内同步 Call 必须等待返回，同时保持同一 Service 不重入。固定 worker pool 在多个 Handler 同时 Call 时可能耗尽；实现 continuation 又会显著增加第一版复杂度。Service 的动态创建、初始化失败、停止队列和 Timer 竞争也必须有唯一策略，避免代码阶段反复改变生命周期形状。

## Decision

- 每个 Running Service 由 Runtime 管理一个 runner goroutine，串行消费 Mailbox；业务不能管理或访问 runner。
- Call 只阻塞当前 Service runner，不影响其它 Service。
- Handler 通过 `(any, error)` 返回结果，由 Runtime 根据 SessionID 自动完成 Reply。
- Runtime Running 后允许动态 CreateService。ID 在 OnStart 前分配，只有 OnStart 成功后才返回和发布；失败 ID 作废且不复用。
- OnStart 可以使用 ServiceContext，但当前 Service 在成功前不接受普通业务消息。
- Stop 立即拒绝新 admission、取消 Timer 和 outgoing PendingCall；当前 Handler 在 shutdown deadline 内结束，排队 Send 丢弃并诊断，排队 Call 返回 `ErrServiceStopping`。
- Stop/Close 幂等且等待完成，OnStop 最多执行一次。Handler panic 停止当前 Service，并使相关 Call 返回 `ErrServicePanicked`。
- 第一版只提供一次性 After 和 CancelTimer；Timer 使用普通 Mailbox admission，不保证同一时刻到期顺序。

## Consequences

- 单线程语义可以直接映射到 Go 执行模型，第一版不需要自制协程调度器。
- 大量空闲 Service 会消耗 goroutine stack 和调度元数据，必须用规模基准验证；若成本不可接受，再以新的 ADR 替换实现策略。
- Stop 不排空全部 Send，关闭时间可预测，但未处理的 fire-and-forget 工作只通过诊断可见。
- OnStart 中的 Call 可能形成启动依赖环，服务装配必须保持单向依赖并设置超时。
