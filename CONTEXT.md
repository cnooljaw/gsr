# Context

## Glossary

- **ServiceID**：`uint64` 形式的 Runtime 局部 Service 地址，只在当前 Runtime 生命周期内有效，不包含 Node、Epoch 或 Generation，也不是可持久化业务身份。
- **ServiceName**：由 Core 外层 Registry/Discovery 管理的稳定逻辑名称，可解析为当前 ServiceID。
- **NodeID**：Cluster 用于选择远端节点和连接的路由名称，不属于 Core ServiceID。
- **SessionID**：`uint64` 形式的 Core 本地请求关联号；零表示 Send、不需要 Reply，非零表示 Call、等待一次 Reply。它只负责关联，在当前 Runtime 内单调分配且不回绕复用。
- **TimerID**：`uint64` 形式的 Runtime 局部 Timer 标识，只在当前 Runtime 生命周期内有效；零值无效，已发放的 ID 不回绕复用。
- **Source**：当前本地 Message 的发起 ServiceID。跨节点原始来源由 ClusterSessionID 和传输上下文关联，不把节点身份编码进 ServiceID。
- **Message**：Core 内存中的本地消息对象，包含 Source、Target、Session、Command 和 Args；它不进行 wire 编码，Send 与 Call 共用该结构。
- **Message Ownership**：消息成功被 Runtime 接受后，Message 独占自己的 `Args` 外层切片。`any` 内部的 map、slice、pointer 等引用数据必须由 typed client 按具体消息类型复制成消息专用快照；发送方不得在 admission 后继续修改，也不得把 Service 拥有的可变状态引用作为参数发送。
- **CommandContext**：Runtime 为每次 Handler 调用创建的只读消息上下文，提供当前 Service 的 Core 能力以及由 Runtime 填入的 `Source()`；业务不能覆盖 Source，也不能通过它手工或异步 Reply。
- **PendingCall**：Runtime 内部以非零 SessionID 索引的一次等待。Reply 通过 completion 路径完成它，不进入调用方 Mailbox 或业务 Handler。
- **Run-to-completion**：一个 Handler（包括其中的 Call 等待）结束前，同一 Service 不执行下一条消息；等待期间其它 Service 仍可运行。
- **MaxCallWait**：Runtime 对一次本地 Call 施加的最大等待时间。调用方必须提供 `context.Context`，其 deadline 可以缩短实际等待，但不能延长或取消 Runtime 上限。
- **Service Runner**：由 Runtime 为每个 Running Service 管理的 goroutine，串行消费该 Service 的 Mailbox；其生命周期和 recovery 属于 Scheduler，不属于业务。
- **ClusterEnvelope**：Cluster/Transport 使用的可编码网络载体，显式区分 request、response 和 error frame。远端 relay 解码请求后创建新的本地 Message；本地投递不创建或编码 ClusterEnvelope。
- **ClusterClient**：初始化时绑定一个 Source ServiceID 的 per-Service 窄 client；业务使用 `Send(node, target, ...)` / `Call(node, target, ...)`，不能覆盖 Source。
- **ClusterSessionID**：`uint64` 形式的 Cluster 请求关联号；零表示 Send，非零表示 Call。它由 Cluster 管理、在当前实例内单调分配且不回绕复用，不要求与 Core SessionID 相同。
- **ErrorClass**：Reply error 的顶层类别。第一版只有 Domain 与 Runtime；programmer error 在 Runtime 边界转成稳定 Runtime error，并把内部诊断留在本地。
- **ErrorCode**：跨本地与 Cluster 稳定传递的字符串错误代码，例如 `room.full` 或 `runtime.service_not_found`；程序逻辑不得依赖 Error Message。
- **StatusError**：包含 ErrorClass、ErrorCode 和安全 Message 的可判定 error；本地和远端 typed client 都通过 `errors.Is` 或 Class/Code 判断。
- **Revision**：领域状态版本，用于拒绝同一 Service 实例中的迟到异步结果；它不同于 ServiceID，也不属于 Core Service 身份。
- **Admission**：Mailbox 或出站队列对消息的本地接受动作。Send 返回 nil 只说明 admission 成功，不说明目标 Handler 已执行或业务操作已成功。
- **NodeSender**：Cluster 为一个远端 Node 唯一管理的 outbound 发送者，合并并发建连并负责自动重连，但不自动重放结果不确定的业务请求。
- **ConnectionAgent**：Cluster 为一条 inbound socket 创建的接收适配器，负责解码远端帧、投递本地 Service 并沿原连接返回结果。
- **RuntimeState**：Runtime 的生命周期状态。公开顺序为 `Created → Starting → Running → Stopping → Stopped`；`Starting` 表示 Runtime-owned Core 组件正在启动。
- **Lifecycle Coordinator**：Runtime 内部按固定 start/stop 计划编排 Core 组件的生命周期；它不拥有组件资源，也不是可无限注册的业务插件容器。
- **Quiesce**：关闭流程中停止新 admission、入站和重连的阶段；它不等同于已经等待所有已接受任务完成。
- **ShutdownTimeout**：Runtime 关闭全部 Core 组件共享的内部时间预算；调用方传入的 `Close` context 只控制该调用者等待多久。
