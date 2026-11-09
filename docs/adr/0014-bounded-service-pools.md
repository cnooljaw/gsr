# ADR 0014：有界 Service 池

## 状态

Accepted，2026-11-06。用户确认由框架提供通用有界并行能力：外部 Call 一个逻辑入口，内部多个独立 Service 并行；业务自行负责可并行性、事务、幂等与业务顺序。当前开发能力，尚未发布。

## 依据

只读核对 Skynet `lualib/skynet.lua` 的 `yield_call`、`raw_dispatch_message` 和 `service/clusterproxy.lua`：Lua Call 挂起当前 coroutine，服务可处理其它请求，代理用 rawcall 和手工回复桥接。GSR 的 Go Service 采用非重入 run-to-completion，不能通过单个入口 Service 同步 Call 下游实现并行，也不照搬 coroutine、手工回复或 session 转移。

## 裁决

- 增加 `Node.CreateServicePool(ctx, name, ServicePoolConfig{Size, MailboxCapacity, NewService}) error`。工厂返回非 nil 指针 Service，不得复用有状态实例；零大小无状态对象允许 Go 合并地址。每个成员具有真实独立 ServiceID、Mailbox 和 runner。共享的外部依赖由业务保证并发安全。
- Size 必须为正；每成员 MailboxCapacity 为正时覆盖 Runtime 默认，零值沿用默认，负值拒绝。并行 Handler 上限为 Size，等待消息最多 Size × MailboxCapacity；实际成员退出后上限相应降低。
- 池保留名称直到创建完成，不暴露半成品；创建者退出不停止池。名称与普通 Service 名称互斥，不修改 RegisterService 单名称/单实例语义。重复名称在调用工厂前拒绝。
- 创建阶段工厂 panic 转为安全 Runtime Error；无效工厂产物在启动成员前拒绝。工厂只组装对象，资源初始化放 OnStart；生命周期代码须响应 context，不能强杀不返回的钩子。成员启动失败时撤销名称，并先停止所有已成功创建成员，再在共享有限预算内等待回收；不关闭整个节点。
- 本地 SendName/CallName、HTTPIngress 和 Cluster 入站共用 Node 名称解析。轮询只选择一次具体 ID，随后使用现有 Send/Call 路径；选择时与调用前停止竞争可能返回原有 Service 停止错误，不重选、不转移 PendingCall、不重放。
- 选中的 Mailbox 满时返回 ErrMailboxFull；即使另一成员尚有空间也不扫描重试，不承诺全局工作守恒或最优负载均衡。池整体不保证到达/完成顺序；同一成员仍串行。
- Call 等待继续阻塞调用方 Service；超时只结束等待，不终止目标，不增加或替换执行者。池成员不得依赖同池嵌套同步 Call 来取得可用容量；具体 ID self Call 仍拒绝。
- 成员 Exit、panic 或节点停止沿现有生命周期收尾，并从池移除；不自动补员。剩余成员继续服务，最后一个退出后释放名称。正在执行但未返回的成员不被当作已回收，不进行强杀。
- Core 增加窄基础设施方法 `CreateServiceWithConfig(ctx, service, ServiceConfig{MailboxCapacity})` 与 `StopServices(ctx, ids...)`，分别建立实例队列和回滚生命周期；不增加池类型、名称、并发 Handler、额外队列或 Reply 入口。StopServices 先通知所有目标停止，再有限等待，调用方取消仅结束等待，runner 继续由 Runtime 跟踪。
- 业务接口仍只获得 ServiceContext，不能借上述基础设施入口伪造 Source 或绕过串行执行。SQL、结算、账户、分片规则不进入框架。

## 代价与验证

固定实例和轮询保持实现简单，代价是可能局部排队、成员失败后容量下降。后续只有真实负载证明需要时才讨论更复杂调度、重启或分片。

扩展既有 Runtime 队列/生命周期验证，新增池的本地并行、来源、满载、超时不重放、启动回滚和成员退出测试；通过真实 Cluster 入站验证名称解析及回复接线，不重复底层 codec/会话完整矩阵。切片收尾执行相关 race 与跨 package 验证。
