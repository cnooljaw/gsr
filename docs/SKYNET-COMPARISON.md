# GSR 与 Skynet 概念对照表

本文是 GSR 开发时的“术语翻译表”。它回答的是“GSR 的一个概念在 Skynet 中大致对应什么”，不是兼容性承诺。GSR 当前契约仍以 [`CONTEXT.md`](../CONTEXT.md)、[`API-DESIGN.md`](API-DESIGN.md) 和已接受的 ADR 为准。

对照依据：

- 本项目设计文档和实现路线图。
- 本地只读参考目录 `../skynet`，当前基线为 commit `2251550`。
- 重点源码：`skynet-src/skynet_start.c`、`skynet_server.c`、`skynet_mq.c`、`skynet_timer.c`、`lualib/skynet.lua`、`lualib-src/lua-skynet.c`、`lualib/skynet/cluster.lua`、`service/gate.lua`、`service/multicastd.lua`、`service/cdummy.lua`、`service/cslave.lua`、`examples/simplemonitor.lua`、`service/clusterd.lua`、`service/clustersender.lua`、`service/clusteragent.lua`、`service/clusterproxy.lua`。

关系栏使用以下含义：

- **直接对应**：解决的问题相同，可以作为概念翻译，但不表示 API 或二进制兼容。
- **借鉴后重构**：保留设计意图，执行机制按 Go 和 GSR 边界重新组织。
- **有意不照搬**：Skynet 有对应能力，但 GSR 不提供同形 API。
- **GSR 新增约束**：为了类型安全、可测试性或明确所有权而增加的契约。

## 一、Runtime、Service 与生命周期

| GSR 概念 | GSR 阶段 | Skynet 对应概念 | 关系 | 关键区别 |
| --- | --- | --- | --- | --- |
| `Runtime`、`Start`、`Stop`、`Close` | Core 0.5 | 一个 Skynet node：`skynet_start()`、全局 `G_NODE`、worker/timer/socket/monitor 线程 | 借鉴后重构 | Skynet 主要是进程级全局运行时；GSR 是可创建、可测试、可明确关闭的 Go 对象。`Stop(ctx)` 有序停止，`Close()` 最终释放且不允许重启。 |
| `RuntimeState`：`Created → Starting → Running → Stopping → Stopped → Closed` | Core 0.5 | 没有同等的公开状态机；启动和退出分散在 `skynet_start.c`、`skynet.start()`、`skynet.exit()` | GSR 新增约束 | GSR 用状态决定 admission 边界，并把“停止完成”和“资源最终释放”分开；Skynet 更依赖命令、句柄和协程状态。 |
| Lifecycle Coordinator、固定 `startOrder`/`stopOrder` | Core 0.5 | `skynet_start.c` 中的固定初始化、`thread_timer` 退出 socket/唤醒 worker、主线程 join、最后 `harbor_exit`/`socket_free` | 借鉴后重构 | GSR 把顺序和回滚集中成可测试的内部计划；组件自己拥有资源，Cluster 不进入 Core 计划。 |
| `ShutdownTimeout` | Core 0.5 | Skynet 主退出路径主要 join 线程，没有 GSR 同等的统一 Runtime deadline | GSR 新增约束 | GSR 的内部关闭预算用于 `Close()` 触发的停止；`Stop(ctx)` 的调用方 context 只控制当前等待。Go 不能强杀不响应取消的 Handler/`OnStop`。 |
| `Service`、`CreateService(ctx, service)` | Core 1 计划 | `skynet_context`、Lua service、`skynet.newservice()`、`.launcher` | 借鉴后重构 | GSR 由应用层传入 Go Service 实例，不提供 Bootstrap Service 或工厂；创建成功要经过 `OnStart(ctx, serviceContext)`，失败只回滚当前 Service，ID 作废且不复用。 |
| `ServiceID uint64` | Core 0 已定 | `skynet_context` 的 32 位 handle；远端 handle 还可能带 harbor 位 | 借鉴后重构 | GSR ServiceID 只在当前 Runtime 有效，不编码 Node、Epoch、Generation，也不作为远端地址。 |
| `ServiceName`、外层 Registry/Discovery | Core 1/Cluster 计划 | `skynet.register()`、`skynet.localname()`、`.service`、`SERVICE`、`service_mgr.lua` | 直接对应 | GSR 稳定逻辑名称不属于 Core ServiceID；解析名称与持有运行期 ID 分层。 |
| `Service.OnStart` / `OnStop` | Core 1 计划 | `skynet.start(func)`、`skynet.init_service()`、`skynet.exit()` | 借鉴后重构 | Skynet 的 `start` 是单个 Lua service 的初始化入口；GSR 把 Runtime 基础设施启动和 Service 生命周期分开，OnStart 接收协作 context 并显式返回 error。 |
| `Stop(ctx)`、Runtime `Close()` | Core 1/0.5 | `skynet.exit()`、`skynet.kill()`、`skynet_handle_retireall()` | 直接对应但语义更严格 | GSR 将有序停止与最终资源释放分成两个幂等阶段；有 deadline 但不能强杀，旧 ServiceID 隔离，Stop 后不允许重启。 |
| Service 状态唯一 owner | Core 1 计划 | `skynet_context`、handle 表和 Lua service 状态 | GSR 新增约束 | GSR 业务状态只能由所属 Service Handler 修改，Service 之间不能持有彼此实现指针。 |

Skynet 的关闭顺序可概括为：timer 线程停止时间/socket 事件并唤醒 worker，主线程等待 worker 结束，再释放 Harbor 和 socket。GSR 沿用“先 quiesce、再等待、最后释放”的思想，但由 Core 与外层 Node/Host 分别负责。

## 二、消息、Handler 与本地执行

| GSR 概念/API | GSR 阶段 | Skynet 对应概念 | 关系 | 关键区别 |
| --- | --- | --- | --- | --- |
| `Message`：`Source`、`Target`、`Session`、`Command`、`Args` | Core 1/3 计划 | `skynet_message`：`source`、`session`、`data`、`sz` | 直接对应 | GSR 本地 Message 不做 wire 编码；Skynet 常规路径把 Lua 参数 pack 成 buffer，并把类型编码进 `sz` 高位。 |
| `Command` / `CommandID uint32` | Core 1 计划 | `PTYPE_*`、`skynet.register_protocol()`、protocol id 和 Lua command string | 借鉴后重构 | GSR Core 不暴露 `PTYPE`；CommandID 使用 32 位静态命令字段，与协议保持同宽，Command codec 在边界注册。 |
| `LocalDispatcher` | Core 1 计划 | `skynet_context_message_dispatch()`、`skynet.dispatch()` | 直接对应 | GSR 本地 dispatcher 只处理本地 Message；Skynet callback 还要处理 protocol type、解包和 session。 |
| `Handler.Handle(CommandContext, CommandID, ...any)` | Core 1 计划 | `skynet.dispatch(typename, func)`，回调参数通常为 `session, source, ...` | 借鉴后重构 | GSR 用 Go Handler 和只读 `CommandContext.Source()`；Source 由 Runtime 写入，Timer 可为零值，业务不能伪造。普通 Service 可选择嵌入不可变 `CommandDispatcher`。 |
| `CommandDispatcher`（每 Service、不可变、可选） | Core 1 计划 | Service 内 `CMD[cmd]` 表配合 `skynet.dispatch()` | 借鉴后重构 | GSR 在构造期校验并复制 `CommandID -> typed Handler` 表，运行期不注册、不预检查；Dispatcher 返回结果/error，不调用手工 Reply。直接实现 `Handle` 仍是扩展口。 |
| `CommandContext.Source()` | Core 1 计划 | dispatch callback 的 `source` 参数、`skynet.context()` | 借鉴后重构 | 普通 Send/Call 的 Source 由 Runtime 写入当前 Service；Timer Command 使用 `ServiceID(0)`；受控 Redirect 才能按固定策略保留已有 Source，业务 API 不允许覆盖或传入任意值。 |
| 受控 `Redirect` / `Relay` | Core/Adapter 后续 | `skynet.redirect(dest, source, typename, ...)` | 借鉴后重构 | Skynet 的 redirect 同时承担 buffer ownership、fan-out、Harbor 控制 relay 和事件 provenance。GSR 不提供任意 source 的业务 API；首版只考虑受信任 Runtime/Adapter 的单向 Runtime-owned Message 转发，不转移 Call session/PendingCall。 |
| `Args []any` 的消息快照 | Core 1 计划 | `skynet.pack()`、C 层 `_filter_args()`；`PTYPE_TAG_DONTCOPY` 是不复制逃生口 | 借鉴后重构 | GSR 只隔离 `Args` 外层；map/slice/pointer 的复制深度由 typed client 负责，Core 不做通用深拷贝。 |
| `Mailbox` | Core 1 计划 | 每个 context 的 `message_queue`，以及全局 MQ | 直接对应但策略不同 | Skynet 队列满时动态扩容；GSR Mailbox 有界，admission 失败返回稳定 Runtime Error，不无限阻塞或扩容。 |
| `Scheduler` | Core 1 计划 | worker threads、global MQ、`skynet_context_message_dispatch()` 的 weight 和 monitor | 借鉴后重构 | GSR 第一版每个 Running Service 一个 Runtime-owned runner，Scheduler 负责创建、停止、等待和 recovery，不暴露全局队列。 |
| per-Service runner | Core 1 计划 | Skynet worker 从全局队列取 context，再创建/恢复 Lua coroutine | 有意不照搬 | GSR 用一个 runner 串行消费一个 Service 的 Mailbox；业务不得接触 runner 或自行启动 goroutine。 |
| 同一 Service 的 run-to-completion | Core 1/3 计划 | 一个 context 的 message queue 通常由一个 worker dispatch；但 `skynet.call()` 会 yield，服务仍可运行其它 coroutine/message | 有意不照搬 | GSR 当前 Handler 内 Call 等待时保留该 Service 执行权，不处理下一条消息；其它 Service 仍可运行。 |
| `Self()` | Core 1 计划 | `skynet.self()` / `c.addresscommand("REG")` | 直接对应 | GSR 返回当前 Runtime 的 ServiceID；不返回可编码的全局或远端地址。 |
| `Send(target, command, args...)` | Core 1 计划 | `skynet.send()`、`skynet.rawsend()` | 直接对应但边界更窄 | GSR 使用 `SessionID == 0`，返回值只代表本地 admission；Core Send 不直接理解 Node、Transport 或 `PTYPE`。 |
| `Call(ctx, target, command, args...)` | Core 3 已完成 | `skynet.call()`、`skynet.rawcall()` | 直接对应但语义更严格 | GSR 必须受 `MaxCallWait` 和调用方 context 的更早 deadline 约束，拒绝 self Call；Skynet 普通 `call` 本身没有统一最大等待时间。 |
| `PendingCall` | Core 3 已完成 | `session_id_coroutine`、`watching_session`、等待中的 Lua coroutine | 直接对应 | GSR 用独立 PendingCall completion 路径恢复原 Handler；Reply 不作为业务 Command 投递回 Mailbox。 |
| `SessionID uint64` | Core 0/3 计划 | context 内 session、`c.genid()`、`session_id_coroutine` 表 | 借鉴后重构 | GSR 零值表示 Send，非零表示 Call，并在 owner 生命周期内单调递增、耗尽即失败；Skynet 的 session 需要处理较小整数空间的回绕冲突。 |
| Handler 返回 `(any, error)` 自动完成 Reply | Core 3 已完成 | `skynet.ret()`、`skynet.retpack()`、`skynet.response()`、`PTYPE_RESPONSE` | 有意不照搬 | GSR 不提供手工或异步 Reply API；Runtime 根据 SessionID 恰好完成一次成功结果或结构化错误。 |
| `ErrorClass`、`ErrorCode`、`StatusError` | Core 0/3 计划 | `PTYPE_ERROR`，通常是空 payload 或 Lua error 文本 | GSR 新增约束 | GSR 程序分支只看 Class/Code，Message 只用于安全诊断；未知 Handler error/panic 不透传内部 stack。 |

### Skynet `redirect` 的实际用途

本地 Skynet 示例显示，`redirect` 不是单一的“把业务请求换个目标”API，而是底层消息转发/所有权工具：

| 本地示例 | 实际行为 | 对 GSR 的启示 |
| --- | --- | --- |
| `service/gate.lua` | 把仍由 gate 持有的 socket buffer 直接交给 agent；source 取 `c.client`，为零时由底层回退为 gate context，重点是 buffer ownership 而不是伪造来源。 | 零拷贝和缓冲区转移留在 Transport/ConnectionAgent 私有层，不能把共享可变指针带入 Core。 |
| `service/multicastd.lua` | 将同一发布消息扇出到订阅者，并保留发布者 source；依赖 C/Lua 引用计数和消息生命周期。 | 多播属于外层功能；GSR 对每个目标分别做有界 admission，受信任 relay 才能保留已有 Source。 |
| `service/cdummy.lua`、`service/cslave.lua` | 将 Harbor 注册/查询控制消息送到 `harbor_service`，source 表示相关句柄或登记者。 | 基础设施 relay 可有独立 source 策略，但不向业务开放任意 source 参数。 |
| `examples/simplemonitor.lua` | 将已退出服务的地址作为事件通知中的 source，source 更像事件主体而非即时发送者。 | 生命周期事件使用显式 typed 参数或诊断 metadata，不把事件主体伪装成业务 Source。 |
| `service/clusterproxy.lua` | 普通请求/响应使用 `send`、`rawcall` 和 session bridge，没有用 `redirect` 转移 Call。 | GSR 的 Call 代理发起新的下游 Call；首版 Redirect 不转移 PendingCall、Session 或 Reply 所有权。 |

Skynet 的这些行为还依赖 `skynet_message.source/session`、PTYPE、Lua coroutine、C 层 buffer ownership 和 session 表。GSR 只提炼“受控转发”和“消息所有权”的目的，不照搬一个可任意填写 source 的公共方法。

两条本地路径的核心关系是：

```text
GSR:    Send/Call → 同一个 Message → LocalDispatcher → Mailbox → Service runner → Handler
Skynet: send/call → c.send → skynet_message → message_queue/global MQ → worker → Lua coroutine
```

GSR 只用 `SessionID == 0` 或 `SessionID > 0` 区分 Send/Call；Skynet 还把 protocol type、response/error PTYPE 和 Lua coroutine session 表作为运行机制的一部分。

## 三、Timer、协程与异步任务

| GSR 概念/API | GSR 阶段 | Skynet 对应概念 | 关系 | 关键区别 |
| --- | --- | --- | --- | --- |
| `After(duration, command, args...)` | Core 2 计划 | `skynet.timeout(ti, func)` 和 C `skynet_timer.c` | 借鉴后重构 | Skynet 到期后恢复一个 Lua callback coroutine；GSR Timer 只向目标 Mailbox 投递普通 Command，Source 固定为 `ServiceID(0)`，创建者由 Timer owner 表达。 |
| `CancelTimer(TimerID)` | Core 2 计划 | 没有同等的 TimerID 取消 API；通常通过 coroutine/session 状态、`skynet.killthread()` 等方式停止等待 | GSR 新增约束 | GSR Timer 是拥有者可识别的资源，取消与触发竞争最多产生一次结果；不暴露通用协程杀线程语义。 |
| `TimerID uint64` | Core 0/2 计划 | timeout 使用的 session 和 `sleep_session`/`session_id_coroutine` 映射 | 借鉴后重构 | GSR TimerID 与 SessionID 分离，零值无效、单调分配；Timer 不拥有高于普通 Message 的 admission 优先级。 |
| Service 内 Call 等待 | Core 3 已完成 | `skynet.call()` → `coroutine.yield("SUSPEND")` → response/error 恢复 | 直接对应但执行模型不同 | GSR 挂起当前 Service runner；Skynet 挂起当前 Lua coroutine，允许同一 service 的其它 coroutine 继续推进。 |
| 业务异步任务 | Core 4 计划 | `skynet.fork()`、`skynet.sleep()`、`skynet.wait()`、`skynet.wakeup()` | 有意不照搬 | GSR 业务不能直接启动 goroutine，也没有公共 `Runtime.Go/Fork`；并发必须由明确 owner 管理取消、上限、panic 和等待。 |
| Result Command + `Revision`/`OperationID` fencing | Core 4/业务层 | Lua coroutine 继续执行或由 session 关联结果 | GSR 新增约束 | GSR 外部异步结果必须回到 Mailbox，由 Service 自己校验领域版本；Core 不用共享可变引用绕过 owner。 |

## 四、Cluster、远端寻址与网络连接

| GSR 概念/API | GSR 阶段 | Skynet 对应概念 | 关系 | 关键区别 |
| --- | --- | --- | --- | --- |
| Cluster（Core 之外） | Cluster 0–4 计划 | Harbor、`skynet.cluster`、`clusterd`、`clusterproxy`、`clustersender`、`clusteragent` | 直接对应架构方向 | GSR 明确禁止 NodeID、Envelope、Transport 进入 Core Message 或 ServiceID；Skynet 通过远端 handle/Harbor 和服务组合完成路由。 |
| `NodeID` | Cluster 计划 | `cluster.call(node, address, ...)` 的 node 名称、Harbor 编号 | 直接对应 | GSR NodeID 是独立路由维度，不能从 ServiceID 推断；远端目标必须由 Cluster 层解析。 |
| `ClusterClient`（绑定 Source ServiceID） | Cluster 计划 | `cluster.call()`、`cluster.send()`，以及按 node 缓存 sender | 借鉴后重构 | GSR 使用初始化时绑定 Source 的 per-Service client；禁止全局变量、goroutine-local 或调用栈推断当前 Service。 |
| `ClusterEnvelope` | Cluster 计划 | `cluster.packrequest/packresponse`、`PTYPE_HARBOR`、socket channel 帧 | 借鉴后重构 | GSR Envelope 显式区分 request/response/error、版本、ClusterSessionID、Target、Command 和结构化错误；本地 Message 不变成网络包。 |
| `ClusterSessionID` | Cluster 计划 | cluster sender 的 session、`cluster.unpackresponse()` 返回的关联号 | 直接对应但独立 | GSR ClusterSessionID 与 Core SessionID 分属不同 owner，不要求数值相同；零值仍表示 Send。 |
| `NodeSender` | Cluster 计划 | `clusterd.lua` 的 `node_sender[node]` + `clustersender.lua` | 直接对应 | 每个远端 Node 一个 outbound sender，合并并发建连；断线自动重连，但不自动重放结果不确定的业务请求。 |
| `ConnectionAgent` | Cluster 计划 | `clusterd.lua` 为每条 inbound fd 创建的 `clusteragent` | 直接对应 | 每条入站 socket 独立 agent；响应沿原连接返回，不与同一 Node 的 outbound sender 混成一个所有者。 |
| 远端 Service 地址 | Cluster 计划 | 远端 handle、`@name`、`cluster.proxy(node, name)` | 有意不照搬 | GSR 的 ServiceID 永远是本地临时地址；远端稳定身份使用 NodeID + ServiceName/Discovery，具体实例解析留在 Cluster；不引入 Epoch/Lease/Generation，也不由 Runtime 判断业务资源所有权、一致性或过期。 |
| 断线、重连和重放 | Cluster 计划 | `socketchannel` 的 reconnect/changehost、`clusterd` sender 重建 | 借鉴后重构 | GSR 重连只恢复未来通信，不宣称 exactly-once，也不自动重放业务 Call/Send；幂等、RequestID 和补偿属于业务。 |
| Cluster 关闭顺序 | Cluster 计划 | socket close/free、cluster sender/agent service 退出、Harbor release | 借鉴后重构 | 外层 Node/Host 先停止入站和新的出站 admission，再 `Runtime.Close`；Core 完成后再关闭 Transport 和连接 goroutine。 |

## 五、最容易误译的 Skynet API

| 看到的 Skynet API/机制 | 不要直接翻译成 | 在 GSR 中应理解为 |
| --- | --- | --- |
| `skynet.start()` | “启动整个 Runtime” | 一个 Lua service 注册初始化回调；GSR 由 `Runtime.Start()` 启动基础设施，再由 `Service.OnStart()` 启动业务。 |
| `skynet.dispatch()` | “注册一个全局函数” | 为 protocol 设置 dispatch handler；GSR 是带只读 Source 的 Service Handler，普通业务可在其内部嵌入 per-Service `CommandDispatcher`。 |
| `skynet.ret()` / `skynet.response()` | `Reply()` 公共 API | GSR Handler 返回值自动完成 Reply，不允许异步或重复回复。 |
| `skynet.fork()` | 公共 `Runtime.Go()` | GSR 不给业务逃生式 goroutine 入口；明确并发 owner 才能创建并负责回收。 |
| `skynet.timeout()` | 定时器回调 | GSR `After` 只投递 Command；状态变化仍必须在 Service Handler 中发生。 |
| `skynet.redirect()` | 任意业务 Service 都能指定 source 的公共转发 API | GSR 只在明确受信任的 Runtime/Adapter relay 中转发 Runtime-owned Message；普通业务用 Send/Call，Call 代理发起新的下游 Call。 |
| `skynet.send()` 返回成功 | 远端业务已执行 | GSR 和 Cluster 都只承诺本地 admission；远端执行和持久化需要 Call 或业务协议确认。 |
| `PTYPE_*` | GSR 的 CommandID 或错误码 | PTYPE 是 Skynet 的消息类型/协议层标记；GSR Core 用 CommandID，wire 层另有 codec 和 Envelope。 |
| 远端 handle `@name`/Harbor 位 | 全局 ServiceID | GSR ServiceID 不携带 Node；使用 NodeID、ServiceName 和 ClusterSessionID 分层表达远端请求。 |
| `skynet.killthread()` | 可安全强杀 Go goroutine | Go 不能强杀任意 Handler/`OnStop`；GSR 用 context、owner 取消、隔离和残留任务观测。 |

## 六、结论

GSR 可以概括为：**保留 Skynet 的 Service + Message + Mailbox + Scheduler + Timer + Cluster 分层思想，替换为 Go 的显式类型、结构化错误、固定生命周期、有限 admission、消息快照和 owner 并发模型。**

因此，遇到新 API 时应先问三个问题：

1. 它属于 Core 还是 Cluster/业务外层？
2. 它是在表达 Message/Command，还是在表达 wire/Transport？
3. 它是否会绕过 Service owner、Mailbox、PendingCall 或生命周期协调器？

如果答案会把网络身份塞进 ServiceID、把 Reply 做成第二条消息管道、把 Timer 变成任意回调，或把清理继续追加到单一 `closeFn`，就不是 GSR 当前的设计方向。
