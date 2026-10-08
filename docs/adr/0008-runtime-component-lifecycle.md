# ADR 0008: Runtime 组件生命周期与关闭顺序

## Status

Accepted

后续说明：本文的gsr.Run 内部运行环境 表示 Core 之外的生命周期协调职责，不要求公开 Host 对象；该职责现由 gsr.Run 内部承担，见 [ADR 0011](0011-service-first-application-api.md)。Core 固定组件计划与本裁决保持不变。

## Context

Core 0 为让 `Runtime.Close` 可测试，暂时使用了 `closeFn func() error` 占位。随着 Service、Scheduler、Timer 和 Cluster 进入实现，如果继续把清理逻辑追加到一个函数中，组件所有权、依赖顺序、超时、错误聚合和启动失败回滚都会被隐藏在 Runtime 的单一过程里。

Skynet 的退出流程也依赖明确的组件顺序：服务句柄全部回收后，Timer 线程退出 socket 事件源并唤醒 worker，主线程等待线程结束，最后才释放 Harbor 和 socket 资源。这个顺序依赖 Lua/C 的具体实现，不能原样搬到 Go，但“组件拥有资源、Runtime 编排顺序”的原则适用于 GSR。

## Decision

- `closeFn` 只属于 Core 0 的临时实现，不作为后续扩展点；Runtime 使用内部 Lifecycle Coordinator 编排组件。
- 生命周期计划是 Runtime 内部固定的显式 `startOrder` 和 `stopOrder`，不按注册顺序、字母顺序或隐式依赖自动推导。添加一个 Core 组件必须同时补齐启动、关闭顺序和行为测试。
- 每个组件拥有自己的资源、goroutine、取消和等待逻辑；协调器只调用组件的窄生命周期方法、按顺序继续后续组件并收集错误。组件不直接关闭其它组件，也不反向调用 Runtime 的关闭流程。
- Runtime 生命周期为 `Created → Starting → Running → Stopping → Stopped → Closed`。`Start` 在所有 Core 组件成功启动后才打开 admission；Runtime 不提供 Bootstrap Service，应用层随后通过 `CreateService(ctx context.Context, service Service) (ServiceID, error)` 逐个组装 Service。
- `Runtime.Stop(ctx)` 的线性化点是进入 `Stopping` 并关闭全局 admission。Core 停止顺序固定为：停止未来 Timer 投递、结束 PendingCall、停止 Service、等待 Scheduler/runner 真实返回；只有安全完成这些步骤后才释放本地 Registry 和其它 Core 资源。
- `Runtime.Close()` 是无调用方 context 的幂等最终释放操作。如果 Runtime 尚未停止，Close 使用 `ShutdownTimeout` 启动或等待 Stop；Stop 完成后才释放 Runtime-owned 资源。Close 不允许 Runtime 重新 Start。
- 如果 `Stop` 或 `Close` 在 `Starting` 期间到达，Runtime 记录待停止意图；调用者 context 只影响该调用者是否继续等待，不会取消启动完成后的停止流程。
- `Stop` 可从 `Created`、`Starting` 或 `Running` 发起；`Starting` 只记录停止意图，基础设施启动返回后立即进入 `Stopping`。`Stopped` 表示停止流程完成，`Closed` 表示最终资源释放完成；两者都不可重新 `Start`。
- Service 的 `OnStop`、Timer 的取消、PendingCall 的完成和 Scheduler 的 runner 等待由各自 owner 实现；它们不被压缩为一个通用 `Close` 语义。
- Cluster、Transport、NodeSender 和入站连接 owner 不进入 Core Runtime 的生命周期计划。gsr.Run 内部运行环境 先停止入站和新的出站 admission，再关闭 Core；Core 完成后，外层关闭连接和 I/O goroutine。
- Runtime 使用 `ShutdownTimeout` 作为所有 Core 组件共享的内部关闭预算。组件收到取消后仍必须保持真实完成状态可观测，Go 不能强杀不响应取消的 OnStart、Handler 或 OnStop；未完成任务不能被伪报为已停止。
- 同一 Runtime 的并发或重复 `Start`/`Stop`/`Close` 只执行一套生命周期流程。Stop 的调用方 context 只控制当前等待；Close 的错误继续执行后续安全清理并通过稳定错误链返回。

## Consequences

- 关闭依赖在一个可审计的计划中，新增组件不会悄悄改变已有组件的清理顺序。
- 组件可以独立测试自己的 admission、取消、等待和残留任务，不需要通过 Runtime 的匿名函数间接测试。
- Core 与 Cluster 的依赖方向保持单向；跨层关闭顺序由外层宿主明确表达。
- 固定计划需要在新增组件时修改协调器和测试，这是有意保留的架构审查点，而不是运行期动态插件能力。
- `ShutdownTimeout` 不能终止任意 Go 业务函数；超时后的 Service 隔离、任务追踪以及 Runtime 进入 `Closed` 前的安全资源释放必须在 Service/Scheduler 切片中继续细化。
