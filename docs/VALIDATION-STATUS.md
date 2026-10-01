# 当前阶段验证清单

本清单整理 Core、Cluster、日志及已落地外围示例的验证证据，不替代 API 契约，也不表示生产发布或兼容性冻结。实施路线图保留各轮历史切片；判断当前覆盖范围时先看这里，再进入对应测试。

下文历史记录中的 `examples/websocket-gate/prodgate`、`examples/websocket-gate/debuggate` 是当时运行测试的路径。通用 Gate 现位于 `gate/prodgate`、`gate/debuggate`，重跑历史命令时使用新路径；历史结果和耗时不因搬迁而改写。

## 已覆盖的运行链

应用 Timer 身份切片：TestServiceTimerPreservesOwnIdentity 经 gsr.Run 在 OnStart 登记零延时 Timer，确认回调 Source 为零、Self 为拥有者；回调发起的本地 Call 使用拥有者来源，绑定 ClusterClient 仍使用 Self，实际排空的 JSON 日志也记录拥有者 ServiceID。相关测试、全仓测试、vet 和 race 通过；未修改生产代码，此测试不包含远端 Timer 调用或计时精度测量。

应用子 Service 创建失败隔离切片：TestServiceChildStartupFailureIsolation 从 gsr.Run / self.CreateService 覆盖 OnStart 返回错误、panic 和 typed nil。失败不发布有效 ID，已进入启动的实例停止一次，无效实例不调用生命周期；后续健康实例获得不同 ID，Call 成功且 Source 为创建者，最终正常停止。相关测试、全仓测试、vet 和 race 通过；未修改生产实现，未重跑跨进程演练。

应用节点配置切片：新增 Run 非法节点表测试，覆盖空名称、缺失 host/port、非数字或越界端口、非法 host 分隔符及未加括号的 IPv6，确认业务入口不会执行。节点表拆分测试覆盖域名、IPv6、端口上下界、端口零仅保留在监听表，以及输入表/监听表/出站表互不共享 map。相关测试、全仓测试和 vet 通过；未修改生产实现或并发逻辑，本轮不重复运行 race 和跨进程演练。

应用入口故障收尾切片：TestRunCleanupAfterOpen 覆盖真实监听建立后入口启动失败，以及正常取消时子 Service 的 OnStop 返回错误。验证入口和子 Service 各停止一次、启动与停止错误均保留、子 Service 停止日志先于最终日志且全部排空，并通过重绑原端口确认监听释放。全仓测试、vet 和 race 通过；未修改生产代码，未重跑跨进程演练，不覆盖无法返回的 OnStop 或阻塞 Writer。

Counter 入口统一切片：最小本地示例也改用 gsr.Run 和应用层 ServiceContext。三个命令由简单 Handle 校验并分派，不增加 Core dispatcher 的应用层适配框架；有限演示在 OnStop 通知宿主取消等待，Exit 本身仍不停止整个节点。真实运行输出保持 count = 5 / count = 0，新增输出失败保留错误测试。全仓测试、vet 和 race 通过；本轮未重跑跨进程演练，未修改 Runtime/Cluster 生产实现。

应用入口迁移切片：移除公开 app.Host，新增 gsr.Run 与应用层 ServiceContext/CommandContext，self.Cluster() 自动绑定 Self，self.Logger() 绑定同一来源。MainService 创建业务 Service、Register、按节点表 Open 后 Exit；双节点示例不再持有 Host 或 Client。测试覆盖多 Service 并发来源隔离、转调不继承上一跳、同进程两节点名称与连接独立、纯调用节点无需 Open、启动失败回滚，以及双进程正常停机/强杀后的恢复。全仓 go test、go vet、启用 GSR_PROCESS_TEST=1 的全仓 race 通过。Open 取节点表端口绑定所有接口；测试端口零仅供动态监听，不进入出站路由。下方 Host 描述属于历史切片，不代表当前应用接口。

统一 Command 编码切片：移除 CommandCodecRegistry 和 app.Config.Commands，Cluster 默认使用统一 ValueCodec；合法请求交给目标 Service 判断 Command。新增 adapter 与真实 TCP 测试，覆盖跨 Service 复用同一编号、未知 Send 无回包、未知 Call 返回 ErrUnknownCommand，以及后续正常调用。原未知命令坏帧测试改为坏载荷诊断，保留限频、编解码 panic、错误回包和连接恢复验证。全仓测试、vet、启用 GSR_PROCESS_TEST=1 的全仓 race（含双进程正常停机及强杀恢复）通过。self.Cluster() 和应用入口替换尚待落地；下方历史切片的 Command 清单描述不再代表当前接口。

应用宿主首个切片：新增 app.Host，集中默认 codec 组装、共享 Logger、初始化失败回滚、入口启动和固定顺序关闭。测试覆盖正常取消、入口失败、构造回滚、重复关闭及未注册 Command 拒绝；双节点示例已改用 Host。全仓测试、vet，以及启用 GSR_PROCESS_TEST=1 的全仓 race（包含正常停机和强杀恢复）通过。按节点名称 Open 尚未落地，当前宿主保留显式 ListenConfig；不宣称已完成全部易用性方案。

双节点启动组装切片：宿主只创建普通 MainService，由其创建 Echo/Caller 后 Exit，监听和资源关闭仍归宿主。新增 `TestMainServiceCreatesEcho` 验证入口关闭接纳后 Echo 仍可调用；改造前后均运行跨进程正常停机与强杀恢复门禁。改造后全仓测试、vet，以及启用 `GSR_PROCESS_TEST=1` 的全仓 race 通过；不改变 wire 协议、业务命令或关闭顺序。

退出失败隔离切片补充 `TestServiceExitDoesNotHidePanic`：OnStart 或 Handler 请求 Exit 后 panic，仍返回 ErrServicePanicked，而非正常退出成功；启动失败不返回有效 ID，Handler 失败后拒绝新消息。两条路径均只执行一次 OnStop、清空 PendingCall，健康 Service 仍可调用。相关测试、Runtime 全包、全仓测试、vet 和 race 通过；未修改生产代码。

退出与整体停止交错切片补充 `TestServiceExitOverlapsRuntimeStop`：Handler 已请求 Exit 但未返回时，Runtime Stop 以停止错误完成在途 Call；取消一次 Stop 等待不取消关闭流程。释放 Handler 后正常 Stop/Close，OnStop 仅执行一次，PendingCall 归零。相关测试、Runtime 全包、全仓测试、vet 和 race 通过；使用同步屏障固定交错顺序，不是随机压力或强杀测试，未修改生产代码。

后续退出隔离切片补充 `TestServiceCreateInHandlerAndExitWithSlowStop`：普通 Handler 创建并调用新 Service 时 Source 正确；自身 Exit 后，即使 OnStop 受控阻塞，当前 Call 已返回、新实例仍可调用，退出者拒绝新消息，释放 OnStop 后只清理一次且 PendingCall 归零。相关测试、Runtime 全包、全仓测试、vet 和 race 通过；本轮未修改生产实现，不代表能强杀阻塞的 OnStop。

2026-09-16：ServiceContext.CreateService / Exit 和 MainService Counter 示例已通过全仓 `go test ./... -timeout=2m`、`go vet ./...`、`go test -race ./... -timeout=5m`。首次沙箱运行的 TCP bind 被拒绝，授权后完整通过；部分未变化 package 使用缓存。新增 [退出测试](../runtime/service_exit_test.go) 覆盖启动期退出、当前 Call 正常返回后退出、排队 Call 失败、Timer 清理，以及创建者启动失败或退出不停止已创建的 Service。这不是强杀或生产容量验证。

| 能力 | 主要测试证据 | 证据边界 |
| --- | --- | --- |
| 本地 Service 生命周期、Send、Call、Timer | [Service](../runtime/service_test.go)、[Call](../runtime/call_test.go)、[Timer](../runtime/timer_test.go) | 本地行为与关闭契约，不代表真实业务负载 |
| 规模与关闭后的资源状态 | [规模测试](../runtime/scale_test.go) | 包含 10,000 个空闲 Service 和 10,000 个 Timer；不是吞吐、尾延迟或长期内存稳定性承诺 |
| 公开 Cluster 调用、Open 与关闭 | [公开出站](../cluster/cluster_test.go)、[Open](../cluster/open_test.go)、[关闭边界](../cluster/open_boundary_test.go) | 同一测试进程内的真实 TCP 回环，不是独立进程部署验收 |
| 断线、按需恢复与不重放 | [出站连接](../cluster/outbound_peer_test.go)、[TCP 集成](../cluster/tcp_integration_test.go) | 覆盖半写、断线和下一请求恢复；不承诺结果不确定请求可安全重试 |
| 远端 Service panic 与新实例隔离 | [TCP 故障链路](../cluster/tcp_service_failure_test.go) | 结构化 panic 错误不泄露内部文本；健康 Service 可继续调用，新实例获得新 ID，旧 ID 不误投；同进程真实 TCP，不是进程崩溃测试 |
| Call 取消与迟到回复 | [TCP 迟到回复](../cluster/tcp_late_reply_test.go) | 远端 Handler 已进入后取消本地等待；迟到回复报告未知 Session 并丢弃，同一连接前后健康调用成功，两端 PendingCall 清理；不覆盖业务副作用撤销或 deadline 精度 |
| 目标 Mailbox 满载与恢复 | [TCP Mailbox](../cluster/tcp_mailbox_test.go) | 受控阻塞目标并本地填满默认队列，远端 Call 返回 ErrMailboxFull；健康 Service 继续调用，排空后原连接及原目标恢复；不是远端 Send 失败回执或吞吐测试 |
| 名称映射更新与旧 ID 隔离 | [名称映射](../cluster/name_rebinding_test.go) | 外层发布新映射后，后续名称 Call 到达新实例，旧 ID 仍返回已关闭；不提供注册发现、不迁移在途请求 |
| 坏帧、超限与残帧隔离 | [协议故障](../cluster/open_protocol_test.go)、[响应读取](../cluster/response_reader_test.go) | 合法后续调用继续运行；不可恢复帧边界关闭连接，不把残帧送入 Handler |
| 重复坏帧诊断与日志 | [双连接日志](../cluster/open_logging_test.go)、[入站连接](../cluster/inbound_connection_test.go) | 每连接同类首条、抑制计数和结束汇总；不是全局限流或可靠审计 |
| FIFO 日志、文件滚动及失败收尾 | [日志顺序](../logging/logger_test.go)、[文件](../logging/file_test.go)、[输出器生命周期](../logging/writer_lifecycle_test.go) | 覆盖短写、Flush 故障、保留历史和借用所有权；不保证 fsync、磁盘崩溃恢复或云端收到 |
| Service 内 MySQL 产品 CRUD 示例 | [业务 Service 测试](../examples/mysql-product/service_test.go)、[入口与可选真实 MySQL 测试](../examples/mysql-product/main_test.go) | 默认使用 SQL mock；显式启用的本机专用库测试已完成真实驱动的创建、查询、更新和删除，不覆盖生产部署、并发库存一致性或断线后写入结果判定 |
| Service 内 Redis Hash 缓存示例 | [缓存测试](../examples/redis-user-cache/cache_test.go)、[业务 Service 测试](../examples/redis-user-cache/service_test.go) | 使用自动启动的本机临时 Redis 验证 Hash、TTL、故障和 Service 隔离；缓存不是权威数据源，不覆盖跨节点一致性 |
| Service 内 HTTP 出站示例 | [资料刷新测试](../examples/http-outbound/service_test.go) | 使用本机测试 HTTP 服务验证有界等待、错误映射和状态保留；不覆盖真实第三方服务或自动重试策略 |
| 基础设施 HTTP 入站示例 | [房间人数接口测试](../examples/http-inbound/handler_test.go)、[管理路由业务测试](../examples/http-inbound/admin_test.go) | 使用本机 HTTP 验证只读查询、有界 Call、安全错误响应，以及三条 REST 路由经本地业务 Service 汇总真实 TCP 远端结果；不提供认证、`gsr.Run` 入站接线或公网接入能力 |

## 当前检查结果

2026-10-01 应用名称表跨节点收尾：`TestRunNodesCommunicateWithoutSingleton` 增加真实 TCP 调用后的目标 Service 退出检查；远端首次按名 Call 正常返回，目标 `OnStop` 完成后再次按同名 Call 得到 `ErrServiceNotFound`，不再解析到旧 ServiceID。该定向测试的 race 检查通过；本轮只扩展验收，不修改生产实现，也不代表动态发现或在途请求迁移。

同日应用入口故障收尾补强：`TestRunConstructionRollback` 分别覆盖 Runtime 配置无效、Cluster 配置无效和 typed-nil 入口 Service，确认构造错误可识别、组装失败不运行业务生命周期、共享 Logger 最终排空；`TestRunCleanupAfterOpen` 在已创建子 Service 且 Open 后覆盖 OnStart 返回错误或 panic、正常停机时子 Service 返回错误、入口 OnStop panic，确认原错误与收尾错误均保留、子 Service/入口各停止一次、日志顺序及监听端口释放。两项定向 race 测试通过；只加测试，不修改生产实现。

同日应用 Open 组装边界：已有 Cluster 测试覆盖占端口后重试，已有 Run 测试覆盖未知节点、重复 Open 和退出释放监听；新增 `TestRunOpenRetryAfterBindFailure` 只验证按节点名 Open 在真实绑定失败后不发布 `ListenAddress`，释放占用后同一应用可重试成功。夹具使用与生产 Open 相同的 TCP 地址解析，避免 IPv4 与 IPv6 通配地址不相冲突造成假阴性。定向 race 测试通过；不重复扩大底层测试矩阵。

同日补充同名重绑定：旧目标退出且远端确认名称失效后，仍在运行的 Service 创建并注册新目标；远端在原连接上再次按名称 Call，得到新实例回复。定向 race 测试通过；只验证后续请求重新解析，不迁移已经发出的请求。

同日补充名称注册与退出竞争：`TestServiceNameRegistrationRacesCleanup` 同时发起 128 次注册和目标清理，允许注册在清理前成功或在清理后被拒绝；全部结束后确认名称及反向索引无残留，已退出 ID 不能再次注册。定向 race 测试重复 10 次通过；这是名称表同步边界测试，不代表完整 Service 生命周期压力。

同日补充远端按名 Send：同名新实例注册后，调用端在原连接上 Send，测试等待新目标 Handler 明确处理，再执行原有按名 Call；真实 TCP 定向 race 测试通过。Send 的 nil 返回仍仅表示本地接纳，成功处理是本测试的额外观测，不改变协议保证。

2026-09-15 名称映射切片：权限恢复后 TestClusterNameRebindingAfterServiceFailure 的真实 TCP 测试及 race 均通过，关闭此前审核服务容量故障造成的未验证项。通过公开 Open 的外层 ResolveService 和 ClusterClient.Call 验证故障实例、未更新映射的错误、新实例映射发布及旧 ID 不误投。仅测试使用原子值模拟外层注册表，不新增发现能力，不承诺映射变更会迁移在途请求；也不将该正确性验证当作名称查询的性能测量。

2026-09-15 Mailbox 背压切片：TestTCPCallMailboxFullAndRecovery 使用真实 TCP Call，确定性填满默认容量 Mailbox，检查满载结构化错误、健康目标隔离、已接纳消息按序排空、拒绝消息不执行、原目标恢复，以及两端 PendingCall 清理。Runtime admission 拒绝计数恰为一次，不改容量、不自动重试、不修改生产实现。

2026-09-15 迟到回复切片：TestTCPCallCancellationDiscardsLateReply 使用 channel 控制远端 Handler 进入与释放，确认调用方取消后先恢复、其他 Service 可在旧 Handler 返回前完成 Call；随后等待真实迟到响应的诊断，再验证新 Call 的结果及原连接继续可用。只有测试使用阻塞控制，不新增生产 API、不强杀 Handler、不改变取消或回复协议，也不重复运行持续压力测试。

2026-09-15 Call 故障切片：TestTCPCallServicePanicAndReplacement 默认验证一次完整故障/重建链路；`GSR_CALL_STABILITY=1` 额外持续两分钟，每 10ms 尝试一轮串行闭环检查（迟到不追赶）。首轮 120.001 秒完成 11,803 轮：旧 ID 预期返回 ErrServiceClosed 11,803 次，健康 Service 与新实例合计成功回复 23,606 次，无意外错误、旧 ID 误投或 PendingCall 残留。最初 panic 仅执行一次，并校验远端 ErrServicePanicked 不含私有 panic 文本；这些数值不包括最初的一次性检查。未修改生产代码。

这是故障后的持续正确性检查，不是两分钟一万条/秒的 Call 容量测试；同一测试进程内走真实 TCP，并非前一轮独立进程混合负载。默认行为测试及其 race 已通过，持续部分以非 race 运行。专项允许 150 秒命令超时以容纳两分钟观察及初始化/清理，不扩大普通测试门禁。

```sh
go test -race ./cluster -run '^TestTCPCallServicePanicAndReplacement$' -v -timeout=2m
GSR_CALL_STABILITY=1 go test ./cluster -run '^TestTCPCallServicePanicAndReplacement$' -v -timeout=150s
```

2026-09-15 规模切片：已实际运行现有空闲 Service/Timer 规模测试及固定次数 benchmark，结果见 [本地规模基线](PERFORMANCE-BASELINE.md)。这是单机小样本，不替代持续负载或生产验收；该切片未修改 Go 代码。

2026-09-28 Gate 多慢连接背压样本：`TestProdGateMultipleSlowClientsDoNotBlockHealthyRequests` 在本机以四条不读取数据的客户端和一条健康连接运行 5 秒。四条慢连接均成功写入 12 条后持续遇到有界队列拒绝，健康连接完成 50 次请求/响应；合计 `OutboundQueueFull=19857`，`RejectedConnections=0`、`InboundMailboxFull=0`，五个 Agent 均收到断开通知。定向测试与对应 `-race` 检查通过；这是短时行为验收，不是目标部署容量或长时稳定性结论。

2026-09-22 WebSocket Gate 短时压力切片：在 `GSR_GATE_STRESS=1` 下运行 3 秒慢客户端、16 连接持续发送和 8 轮坏握手/坏 MAC。慢客户端发送 12 条、出站队列满载拒绝 2,976 次；16 连接发送与 Agent 处理均为 4,622 条，`InboundMailboxFull=0`、连接拒绝为 0；失败阶段 8 次全部拒绝且 8 条已认证连接均正常回收。命令为 `GSR_GATE_STRESS_SECONDS=3 GSR_GATE_SLOW_CLIENT_SECONDS=3 GSR_GATE_FAILURE_ATTEMPTS=8 go test ./examples/websocket-gate/prodgate -run '^TestProdGate(LongSlowClientBackpressure|MixedConnectionStress|FailurePhaseStress)$' -count=1 -v`。这是本机短样本，只验证有限队列、连接回收和失败阶段的当前行为，不构成多连接长时容量结论。

同日 30 秒长样本：16 条连接发送与 Agent 处理均为 47,586 条，`InboundMailboxFull=0`、连接拒绝为 0；慢客户端样本先观察到队列满载，随后因写超时关闭连接，`ClientSession.Send` 返回 `ErrClientSessionClosed`，按连接隔离契约计为正常终止，不计为业务错误。

同日 2 分钟多连接样本：`GSR_GATE_STRESS=1 GSR_GATE_STRESS_SECONDS=120 go test ./examples/websocket-gate/prodgate -run '^TestProdGateMixedConnectionStress$' -count=1 -v -timeout=150s` 使用 16 条连接，发送与 Agent 处理均为 189,983 条，`InboundMailboxFull=0`、连接拒绝为 0，测试耗时约 120 秒。该样本证明当前机器和测试负载下连接/Agent 链路可持续运行，不是部署容量、长期稳定性或生产限流结论。

2026-09-23 Gate 压力验收断言收紧：多连接测试现在等候最多 5 秒排空，并要求客户端成功写入数与 Agent 处理数相等、Mailbox 满载和连接拒绝均为零；失败阶段测试在坏握手与坏 MAC 后额外执行一次健康请求/响应。3 秒短样本中非 race 处理 4,784 条、race 处理 4,783 条，均与各自成功写入数相等；两轮 Mailbox 满载与连接拒绝均为零。失败阶段各重复 3 次坏握手与坏 MAC，计数为 `rejected=3`、`accepted=4`，随后健康往返成功。目标测试、对应 race、prodgate 全包和 vet 通过；这次仅加固验收，不改生产 Gate。

同日慢连接隔离切片：`TestProdGateSlowClientDoesNotBlockHealthyConnection` 持续向不读取数据的客户端发送，确认出站队列已发生满载后，在压力窗口内通过另一条真实 WebSocket 连接完成认证请求、Agent 投递及 Protobuf 响应往返；慢连接持续发送至测试结束，校验其成功发送和队列拒绝计数均大于零、连接未提前关闭。专项重复 10 次、prodgate 全包和全仓 race 通过；只验证短时双连接隔离，不作为慢网络长时或容量测试，未修改生产 Gate。

同日跨连接重放切片：`TestProdGateRejectsCrossConnectionReplayBeforeDelivery` 先在源连接发送并成功处理一条认证请求，再将完全相同的 `seq | Envelope | MAC` 帧送到另一条独立握手连接。目标连接以 `1008 policy_violation` 关闭，目标 Agent 未收到该帧；第三条新连接使用自己的密钥和序号 1 成功完成请求/响应。专项重复 10 次、prodgate 全包、全仓测试、全仓 race 和 vet 通过；只验证当前 Gate 实现的连接间帧隔离，不扩展为客户端身份认证或业务防重。

同日非法 X25519 握手集成切片：`TestProdGateRejectsInvalidX25519KeyAndRecovers` 通过真实生产 Listener 发送全零低阶公钥，连接上限设为 1，确认握手以协议错误关闭、Agent 未创建、连接计入拒绝；随后有效客户端仍能占用归还的名额并完成认证请求/响应。专项重复 10 次、prodgate 全包、对应 race 通过；只验证握手拒绝与容量恢复，不构成密码协议审计。

同日同连接重放切片：`TestProdGateRejectsSameConnectionReplayBeforeDelivery` 先在生产 Gate 上成功处理 seq=1 请求并读取响应，再重发完全相同的认证帧。Gate 以 `1008 policy_violation` 关闭连接，Agent 未重复收到消息并收到断开通知。专项重复 10 次、prodgate 全包、全仓测试、全仓 race 和 vet 通过；和跨连接重放测试一起覆盖生产 Gate 的两种旧帧拒绝路径，不代表完整安全审计。

同日认证后超限帧切片：`TestProdGateRejectsOversizedAuthenticatedFrame` 在完成握手后发送完整长度超限的认证业务帧，确认 Gate 返回 `1009 message_too_big`、累计 `FrameTooLarge`、不投递 Agent 并通知断开；随后新连接可正常完成请求/响应。专项重复 10 次、prodgate 全包、全仓测试、全仓 race 和 vet 通过；只验证单连接帧上限与故障恢复，不是吞吐或内存压力测试。

同日认证后短帧切片：`TestProdGateRejectsMalformedAuthenticatedFrame` 在完成握手后发送短于序号与 MAC 最小长度的二进制帧，确认 Gate 返回 `1002 protocol_error`、不投递 Agent 并通知连接断开。定向测试 `go test ./examples/websocket-gate/prodgate -run '^TestProdGateRejectsMalformedAuthenticatedFrame$'` 通过；本轮只新增独立集成测试，未重复运行全包或全仓验证。

同日慢客户端回压长窗口试验：设置 `GSR_GATE_SLOW_CLIENT_SECONDS=120` 后，慢客户端成功写入 12 条，队列累计拒绝 9,994 次；Gate 在约 15 秒时因写超时关闭连接，Agent 收到稳定关闭结果。定向压力测试 `GSR_GATE_STRESS=1 GSR_GATE_SLOW_CLIENT_SECONDS=120 go test ./examples/websocket-gate/prodgate -run '^TestProdGateLongSlowClientBackpressure$' -timeout=130s -v` 通过。这里的 120 秒是 Agent 压力循环的最长窗口，并非实际 soak 时长；本次验证写超时与有界回压，不代表两分钟慢客户端稳定性验收。

同日失败阶段压力扩展：设置 `GSR_GATE_FAILURE_ATTEMPTS=100`，连续执行 100 次坏握手和 100 次坏 MAC；故障连接未投递坏帧，随后健康连接仍完成请求/响应。测试耗时约 1.52 秒，指标为 `rejected=100`、`accepted=101`、`inbound_mailbox_full=0`。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=100 go test ./examples/websocket-gate/prodgate -run '^TestProdGateFailurePhaseStress$' -timeout=90s -v` 通过；这是短时恢复压力样本，不代表长时故障洪泛容量验收。

同日失败阶段 1,000 轮扩展：连续执行 1,000 次坏握手和 1,000 次坏 MAC 后，健康连接仍完成请求/响应，指标为 `rejected=1000`、`accepted=1001`、`inbound_mailbox_full=0`；测试耗时约 15.38 秒。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=1000 go test ./examples/websocket-gate/prodgate -run '^TestProdGateFailurePhaseStress$' -timeout=90s -v` 通过。连接按序执行，本结果不代表并发故障洪泛容量或长时稳定性。

2026-09-23 失败阶段 3,000 轮扩展：连续执行 3,000 次坏握手和 3,000 次坏 MAC 后，健康连接仍完成请求/响应，指标为 `rejected=3000`、`accepted=3001`、`inbound_mailbox_full=0`；测试耗时约 46.39 秒。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=3000 go test ./examples/websocket-gate/prodgate -run '^TestProdGateFailurePhaseStress$' -v` 通过。连接按序执行，本结果不代表并发故障洪泛容量或长时稳定性。

2026-09-23 失败阶段 5,000 轮扩展：连续执行 5,000 次坏握手和 5,000 次坏 MAC 后，健康连接仍完成请求/响应，指标为 `rejected=5000`、`accepted=5001`、`inbound_mailbox_full=0`；测试耗时约 76.22 秒。定向测试 `GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=5000 go test ./examples/websocket-gate/prodgate -run '^TestProdGateFailurePhaseStress$' -timeout=110s -v` 通过。连接按序执行，本结果不代表并发故障洪泛容量或长时稳定性。

2026-09-23 并发故障隔离：8 条已认证坏 MAC 连接同时发送故障帧期间，健康连接完成 16 次请求/响应，坏帧均未进入 Agent，`inbound_mailbox_full=0`。定向测试与对应 `-race` 测试均通过；此样本验证有限并发下的连接隔离，不代表高并发容量。

2026-09-23 并发非法握手隔离：8 条连接同时提交全零低阶 X25519 公钥并按协议错误关闭时，健康连接完成 8 次认证请求/响应；无效握手未创建 Agent，`rejected=8`、`inbound_mailbox_full=0`。专项重复 5 次、对应 `-race` 测试和 prodgate 包全测通过；只验证有限并发下握手失败的连接隔离，不代表高并发容量或密码协议审计。

2026-09-23 Agent 构造器 panic 恢复：构造器 panic 只拒绝当前连接并返回 WebSocket 内部错误关闭；后续连接成功创建 Agent 并完成请求/响应。定向测试和对应 `-race` 测试均通过。

2026-09-23 Agent 启动失败恢复：Agent `OnStart` 返回错误时 Runtime 执行该实例的 `OnStop` 清理，Gate 拒绝当前连接后仍能接受后续连接并完成请求/响应。定向测试、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 Gate 监听启动失败恢复：占用监听地址时创建 Gate Service 失败但 Runtime 继续可用；释放地址后可在同一 Runtime 创建并启动新的 Gate。定向测试、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 Gate 关闭资源回收：`Runtime.Close()` 关闭活动客户端并等待 Gate I/O goroutine 退出，之后 Agent 的 `ClientSession.Send` 返回 `ErrClientSessionClosed`，且测试立即成功重绑原监听地址。相关关闭与 Agent 启动失败用例 race 检查、`prodgate` 包全测通过；两类服务端主动关闭连接的 TLS 清理用例重复运行 10 次通过。

2026-09-23 Gate Stop 资源释放：单独调用 `Runtime.Stop()` 后可立即重绑 Gate 原监听地址，再调用 `Runtime.Close()` 完成最终释放。定向测试、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 Stop 中断待握手连接：Gate 握手超时设为 30 秒、客户端完成 WebSocket Upgrade 后不发送应用握手；`Runtime.Stop()` 在 2 秒门限内主动发送 `going_away` 并完成收尾，握手超时数和已接受连接数均为零，Agent 未创建。专项重复 3 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-24 并发待握手连接 Stop：8 条已完成 WebSocket Upgrade、尚未发送应用握手的连接同时由 `Runtime.Stop()` 关闭；8 条均收到 `going_away`，Gate 等待所有处理 goroutine 退出，`HandshakeTimeouts=0`、`AcceptedConnections=0`、`RejectedConnections=8`，Agent 未创建。专项重复 3 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-24 并发活动客户端 Stop：8 条已认证连接由 `Runtime.Stop()` 同时关闭，每条都收到 `going_away`，对应 `ClientSession.Send` 均返回 `ErrClientSessionClosed`；指标为 `AcceptedConnections=8`、`RejectedConnections=0`。专项重复 3 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-24 Agent/Gate 幂等关闭：活动连接关闭时，Gate 的 `OnStop` 和 Agent 的 `OnStop` 均关闭同一个 `ClientSession`；`Runtime.Stop()` 成功完成，客户端连接关闭，重复调用 `ClientSession.Close()` 返回成功。专项重复 5 次、对应 `-race` 测试和 prodgate 包全测通过。

2026-09-23 Gate 连接容量回收：连接上限为 1 时，未完成 TLS 握手的 TCP 连接占用名额，超限连接被关闭；占用连接关闭后，新的 WSS 连接可完成 HTTP Upgrade。定向测试重复运行 10 次、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 握手超时容量回收：连接上限为 1 时，已完成 WebSocket Upgrade 但未完成应用握手的连接继续占用名额，期间第二条 TCP 连接被拒绝；首条握手超时关闭后，后续 WSS 客户端完成认证和请求/响应。专项重复 5 次、对应 `-race` 测试和 prodgate 包全测均通过。

2026-09-23 上行序号耗尽边界：Go 服务端接纳一次 `uint64` 最大值序号后拒绝回绕到 1；TypeScript 客户端能生成最大序号帧，之后保持序号不变并返回耗尽错误。Go 专项测试与 `npm run test:crypto`（含 TypeScript 编译）通过。协议规定上限后结束当前连接并重新握手；本切片只补测试和说明，不改生产算法。

2026-09-23 C/S 请求编号上限：协议分发测试将最大 `uint32 request_id` 编码进 Envelope，确认 Handler 收到原值且 Response 原样回传。该测试、protocol 全包测试和 vet 通过；客户端 request_id 分配器仍由业务客户端实现，示例 CLI 固定使用 1。

同日 Agent 内部超时切片：`TestProdGateReturnsErrorAfterAgentCallTimeout` 通过真实生产 Gate 验证 Agent 调用本地目标 Service 超时后，客户端收到原 `request_id` 的 `INTERNAL`；目标 Service 释放后产生的迟到 Core 回复不会再次写回客户端。重复运行 5 次、prodgate 全包、vet 和 race 均通过。该切片只确认 Agent/Runtime 的内部 Call 超时归属，不把它扩展为客户端业务请求的统一超时或重试策略。

后续热点切片新增 TestRuntimeHotMailboxLoad（GSR_LOAD_TEST=1 显式启用）：4 个发送者、10 万次固定尝试、容量 64/1024，逐条核对接纳与处理结果并记录到达延迟分位数。它是短突发测试，不替代下文的持续负载验收；未修改生产实现。

Timer 集中到期切片新增 TestRuntimeTimerBurstLoad，沿用 GSR_LOAD_TEST=1：10,000 个 Timer 同轮到期，校验接纳/拒绝计数、消息唯一性和零 Source。实测见规模基线；使用 fake clock，不代表真实时钟精度或定时业务必达保证。

日志测量切片新增 TestLoggingBatchAccounting 与 BenchmarkLoggingBatch：比较禁用、io.Discard、临时文件输出，分别报告成功接纳、满载丢弃及关闭排空成本。本次固定小字段单发送者样本无丢弃；多生产者、慢盘和真实业务日志频率仍未测量。

后续 TestLoggerConcurrentFileLoad 补充 8 个生产者共享文件 Logger 的 10 万条突发，逐条核对接纳、唯一性、文件顺序与丢弃计数。首轮写入 97,097 条、满载丢弃 2,903 条，符合有界非阻塞契约但不代表零丢日志容量达标；慢盘、真实 Service 组合和持续负载仍未覆盖，具体口径见性能基线。

日志隔离切片新增 TestRuntimeLoggingFailureIsolation：受控阻塞 Writer 填满队列以及终止输出失败时，业务返回值、领域错误、Runtime 独立诊断与资源关闭均不被日志基础设施改变。测试不包含真实慢盘性能测量；业务若主动将日志错误返回给调用方，仍属于业务自己的选择。

双进程示例切片补充：已在临时工作目录启动两个独立进程，观察 Send 接收和 Call 成功；停止接收端后调用端继续运行并记录 connection refused，重启接收端后后续新操作恢复成功；最后两个进程均以退出码 0 结束并写出 process stopped。此为本机手动演练，不等同于自动化跨进程门禁或长期负载测试。

- 全仓 `go test ./... -timeout=5m` 与 `go vet ./...` 在前一测试切片通过；本轮仅整理文档，没有改动 Go 代码。
- 本轮补跑 `go test -race ./... -timeout=5m` 通过，关闭了前一轮权限审核超时导致的未验证项；部分未变化 package 使用 Go 测试缓存。
- `git diff --check` 用于文档与补丁格式检查。
- 前述故障验证切片未运行 benchmark；后续 2026-09-15 规模切片的独立测量见本节链接，不把普通测试总耗时当作性能结果。

## 仍需单独验证的范围

当前用户暂定吞吐目标：单进程 10,000 条消息/秒。已新增 TestRuntimePaced10000Load，10 秒样本实际输入约 9,861 条/秒、零满载拒绝，不能据此宣称严格达到目标或完整业务容量验收通过；具体口径见性能基线。

后续逐条文件日志对照的实际输入降至约 6,422 条/秒，出现 200 条 Mailbox 满载拒绝与 332 条日志丢弃；同期关闭日志项也仅约 8,237 条/秒且存在较大尾延迟。保留全部结果，不以最初较好样本代替当前证据；尚不能单独归因于日志，也未达到容量验收条件。

再补生成器唤醒/批次及 GC 聚合观测后，同一生产实现的新样本文件模式约 9,989 条/秒、无拒绝或日志丢弃；旧抖动未复现。该结果不构成性能修复或根因定位，完整原始指标与限制见性能基线。

已验证 Go 原生 `-trace` 对单个文件日志定速子项的采集及完整解析，命令与判读边界见性能基线。采集样本计数守恒且无丢弃，未复现旧抖动；它只验证诊断入口，不替代无采集开销的容量测量。

TestRuntimePacedMultiServiceLoad 已补 8 个真实 Service 共享文件 Logger 的总量定速场景：实际约 9,959 条/秒，每个 Service 正确处理 12,500 条，总计 100,000 条且日志无丢弃。它是无业务计算的 10 秒样本，不代表每个 Service 一万条、长期容量或 Service 内 Call 负载；完整指标见性能基线。

1. **独立进程运行链**：已有 [双进程示例](../examples/twonode/README.md) 和显式启用的 TestTwoNodeProcessRecovery、TestTwoNodeProcessCrashRecovery 门禁，覆盖独立配置、日志目录、Send/Call、正常停机或 SIGKILL 后调用端继续运行与重启后的新请求恢复。强杀发生在首轮 Call 完成后，不覆盖在途执行或日志持久性。默认集成测试仍主要在单进程内运行；跨进程测试不覆盖命令行参数解析、跨机器部署或网络分区。
2. **持续负载和容量**：已有空闲规模基线，但持续热点调用、日志慢盘、Timer 集中到期等组合负载还需目标机器上的容量测算。不能据此决定预先更换 Scheduler 或 Timer 架构。
3. **部署与运维**：进程退出信号、外部日志采集器、磁盘配额与告警还没有完整部署验收；SLS 属于外层采集，不给 Core 添加云 SDK。
4. **安全与发现**：认证协议、动态发现仍按已有设计边界留待实际需求。当前可信内网模式不提供对端身份保证，不能直接当成公网服务使用。

## 下一步建议

用户确认的 10,000 个 Service、9,800 Send/s、80% 本地 + 20% 跨节点、5 秒预热后正式运行 3 分钟已完成。权限恢复后短跑、短跑 race 及正式计数/关闭检查通过：正式处理 176.4 万条，MailboxFull 为零，各组 P99 为 0.874～1.242ms，目标平均使用 0.0476 核。完整资源数据、尖峰与测量边界见 [混合负载验证](MIXED-LOAD-VALIDATION.md)。可暂不优化 Scheduler，转回正确性与业务迁移；这不是重 Handler、多核扩展或生产内网容量的证明。

基础空载测量已包含 Send、Service 内本地 Call、Timer 突发、单/多 Service 定速共享日志及多生产者日志突发。本地 Call 最新固定次数复核约 1.496μs/op，但不是每秒一万条定速或跨节点容量验收。已完成的 80/20 混合场景全部是 Send；下一项容量工作仍需由业务确定 Call 占比、处理成本、日志频率和验收门槛，不继续以重复空 Handler 样本替代业务验收。

最小双进程运行示例及启动说明已提供：应用用启动参数选配置，每个进程配置独立日志目录，调用端无需 Open，接收端显式 Open；业务只演示 Send 与 Call，资源按既有顺序关闭。

这一步先证明框架能被应用清楚地组装和运行，不引入通用配置框架、发现服务、网关或新 Runtime 生命周期 API。自动化跨进程正常停机、强杀与重启恢复门禁已补充；持续负载、实际部署和其它故障场景仍需独立验收，不宣称生产验收完成。

2026-09-24 Gate 断开收尾切片：新增 `TestProdGateAgentStopAfterClientDisconnectKeepsServing`，验证客户端先断开后 Gate 关闭 `ClientSession`、Agent 收到断开通知并在 `OnStop` 再次关闭时不报错；随后第二个客户端仍能完成认证、请求/响应往返。定向测试重复 5 次、定向 race 重复 3 次及 `go test ./examples/websocket-gate/prodgate` 均通过；只增加测试和文档，未修改生产实现或运行全仓验证。

2026-09-24 Gate 并发停机切片：新增 `TestProdGateClientDisconnectOverlapsRuntimeStop`，并发触发客户端断开和 Runtime.Stop，验证运行时停机完成、Agent.OnStop 只执行一次、重复关闭后写回返回 `ErrClientSessionClosed`。定向测试重复 10 次、定向 race 重复 5 次、`go test ./examples/websocket-gate/prodgate` 与该 package 全量 race 均通过；只增加测试和文档，未修改生产实现或运行全仓验证。

2026-09-24 Gate 停机错误切片：生产 Gate 与 DebugGate 原先都会忽略 `ClientSession` 底层真实关闭失败，导致 Runtime.Stop 无法报告该资源错误。两处 OnStop 现在聚合并返回各连接的关闭错误，继续处理后续清理；新增 `TestProdGateStopReportsClientCloseFailure` 和 `TestDebugGateStopReportsClientCloseFailure` 通过注入返回关闭错误的测试连接验证。定向测试重复 3 次，`go test ./examples/websocket-gate/prodgate ./examples/websocket-gate/debuggate`、对应两包全量 race 与 `go vet` 均通过；不改变正常断开错误归一化规则，未运行全仓验证。

2026-09-24 Gate 多连接错误聚合切片：扩展两个停机测试，为两条连接分别注入不同 Close 帧写错误和 socket close 错误，确认 Gate 关闭所有连接且 `OnStop` 返回的错误保留四类原因。定向测试重复 3 次、`go test ./examples/websocket-gate/prodgate ./examples/websocket-gate/debuggate` 与对应两包全量 race 均通过。只强化测试与文档，没有生产代码改动。

2026-09-24 Gate Close 帧错误切片：生产 Gate 和 DebugGate 现在同时保留 WebSocket Close 帧写入与底层 socket 关闭的真实错误；对端已断开错误仍归一化为正常收尾。两个定向测试同时注入 Close 帧写入和 socket 关闭错误，确认 `errors.Is` 可分别识别两种原因。

2026-09-24 Gate 停机关闭错误聚合复核：扩展生产 Gate 和 DebugGate 的停机测试，在两条连接上同时注入各自的 Close 帧写入错误与 socket 关闭错误，确认 `OnStop` 通过 `errors.Is` 保留所有原因并继续关闭全部连接。两个定向测试重复 3 次、对应定向 race 重复 3 次通过；只扩展测试与文档。

2026-09-24 Gate 出站写失败可观测性：生产 Gate 的连接写循环现在将 deadline 设置和 WebSocket 写入错误返回给 Gate，由现有连接级错误日志记录；不改变 `ClientSession.Send` 仅表示队列接纳的语义。定向测试、prodgate 全包测试和全量 race 均通过，日志字段保留错误原因。

2026-09-24 Gate 写 deadline 错误复核：Gorilla `Conn.SetWriteDeadline` 只保存 deadline 并始终返回 nil，内部写入时底层 deadline 错误也会被忽略。生产 Gate 现在在 WebSocket 握手、客户端写循环和 Close 帧写入前显式检查底层连接的 deadline 设置结果；DebugGate 的写循环和 Close 帧也检查该结果。失败会关闭连接，生产 Gate 还会记录连接故障。定向测试分别注入底层 deadline 与消息写入错误，prodgate、debuggate 全包测试及两包 race 均通过；未运行全仓。

2026-09-24 Gate 握手写 deadline 测试：新增生产握手路径定向测试，注入底层 socket 写 deadline 失败，确认握手响应不再尝试写入，错误保留且连接关闭。定向测试及对应 race 测试通过。

2026-09-24 Gate 握手读 deadline 测试：新增定向测试模拟底层读 deadline 设置失败，确认握手保留错误并立即以内部错误码关闭连接。prodgate 全包测试及全包 race 均通过。

2026-09-24 Gate 握手 deadline 清理测试：注入握手响应发送后的读 deadline 清除失败，确认 33 字节响应先送达，随后连接以内部错误码关闭，避免遗留握手 deadline 影响业务读循环。prodgate 全包测试和新增用例 race 通过；全包 race 在既有并发坏握手测试中读到 7/8 条拒绝计数后失败，待单独核对该测试的计数观察时序。

2026-09-24 Gate 并发握手计数同步：客户端读到关闭帧时，ServeHTTP 可能尚未更新拒绝计数；测试改为通过现有等待辅助函数观察稳定计数。定向 race 与 prodgate 全包 race 均通过。

2026-09-24 Gate 握手响应写失败测试：注入底层 socket 写错误，确认握手返回原始错误且客户端收不到响应帧。定向测试、prodgate 全包测试及定向 race 均通过。

2026-09-24 Gate 超大握手计数断言：超限握手集成测试现在同时确认帧超限、连接拒绝、未接纳且未创建 Agent。定向测试、prodgate 全包测试及该用例 race 均通过。

2026-09-24 Gate 非法握手接纳计数：非二进制及格式非法握手测试也明确断言连接未计入接纳数。两个定向测试、prodgate 全包测试及定向 race 均通过。

2026-09-24 Gate 断开通知持续重试：生产 Gate 不再因 5 秒上限放弃 Agent Mailbox 满载时的断开通知；连接处理协程在通知入队前继续占用 `MaxConnections` 名额，使待处理数量有界。DebugGate 改用 Infrastructure Call 确认 Agent Mailbox 接纳，并保留满载时的连接映射；单独停止 Gate 时，`OnStop` 在其生命周期上下文内重试通知剩余 Agent。生产回归测试将 Mailbox 满载保持超过旧 5 秒上限后再恢复，DebugGate 集成测试覆盖 Mailbox 恢复后收到通知。两 Gate package 全测与对应 race 均通过；未运行全仓验证。

2026-09-25 Gate 停机通知公平性：修复 Gate 单独停止时逐个等待 Agent 的队头阻塞；现在每轮都会尝试所有待通知 Agent，仅将 Mailbox 满载者留到下一轮。生产 Gate 与 DebugGate 新增双 Agent 测试，确认一个 Agent 卡住时健康 Agent 仍及时收到断开通知，随后满载 Agent 恢复也能完成收尾。两个 package 全量测试与 race 均通过；未运行全仓验证。

2026-09-25 Gate 停机入站屏障：客户端帧接纳现在与 Gate 的停机状态切换共享读写锁；停机标记生效后，生产 Gate 与 DebugGate 都拒绝继续投递帧，避免逐条关闭连接期间仍有新消息进入 Agent Mailbox。两个定向测试、两包全量测试及仓库级 `go test -race ./...` 均通过。

2026-09-25 生产 Gate 握手帧配置下限：配置的 `MaxFrameBytes` 小于 33 字节时无法容纳固定客户端握手帧；`NewGateService` 现在在启动前返回 `ErrInvalidConfig`，并增加边界测试覆盖 32 字节拒绝、33 字节接受。定向测试和 prodgate 全包测试通过。

2026-09-28 Gate HTTP 空闲连接回收：普通 HTTP 请求返回 404/405 后，keep-alive 连接此前可能无限占住 `MaxConnections` 名额。生产 Gate 为未升级 HTTP 连接设置 5 秒 idle timeout；新增真实 TCP 测试确认 404 响应仍可 keep-alive，随后连接按时关闭。定向测试与 `go test ./examples/websocket-gate/prodgate` 通过；WebSocket 升级连接不受该 idle timeout 影响。

2026-09-28 DebugGate HTTP 空闲连接回收：真实 TCP 回归测试先确认 404 后的 keep-alive 连接超过 5 秒仍不关闭，再验证设置 5 秒 idle timeout 后按时关闭。定向测试与 `go test ./examples/websocket-gate/debuggate` 通过。DebugGate 的 `MaxConnections` 只统计进入 `/ws` GET Handler 的连接，仍不限制原始 TCP 接纳数；它只供受信任环境调试。

2026-09-28 Gate 停机断线通知交接：两种 Gate 的连接处理协程原先在 Agent Mailbox 满载时无限重试断线通知，`OnStop` 等待协程退出便可能越过自身 context 期限。两条受控测试先复现该阻塞；现在停机标记生效后连接协程结束重试，由 `OnStop` 对剩余映射按有界上下文补发。定向测试、两 Gate 包全测及对应包的 race 测试均通过；满载 Agent 在期限内仍不接纳时，`OnStop` 按预期返回 deadline 错误。

2026-09-28 DebugGate 连接拒绝计数：Agent 构造失败时，Gate 内部创建路径与 HTTP 入口原先各计数一次，一条连接得到 `RejectedConnections=2`。回归测试先复现，再验证构造返回 nil 和 Agent `OnStart` 失败都只计一次；定向测试与 DebugGate 包测试通过。

2026-09-28 Gate Origin 部署约束（历史记录，已由下一条裁决取代）：生产 Gate 与 DebugGate 的真实 WebSocket 升级测试确认，浏览器式跨主机 `Origin` 收到 HTTP 403，同 Host 的 `Origin` 可以升级；这是当时 WebSocket 库的默认策略，非应用密钥握手错误。两条定向测试及两 Gate 包测试通过；当时尚未新增配置或放开来源。

2026-09-28 Gate Origin 接入裁决：用户决定不以 `Origin` 作为安全边界，两种 Gate 均显式允许任意来源，保留原生、小程序和 Web 客户端一致的接入语义。业务 token 鉴权仍归 Agent，Echo 示例不声称已实现登录。两条跨主机 Origin 测试先复现旧 403，修改后验证升级成功；生产 Gate 还完成应用密钥握手。两 Gate 包测试通过。

2026-09-28 双 Gate 公共流程重构：示例内部 `gatecommon` 统一 Agent 构造 panic/空值处理、正常断线重试、停机时公平补发和 Runtime 停止错误判断；生产与调试 Gate 保留各自的 WebSocket 入口、监听、认证、容量及日志行为。相关定向测试、两 Gate 包测试及两包 race 测试通过，未运行全仓验证。

2026-09-28 生产 Gate 握手前容量诊断：TCP 接纳上限拒绝连接时，原先只有 `RejectedConnections` 计数、没有共享 Logger 记录。定向测试先复现日志缺失；现在每次拒绝都会尝试向共享 Logger 接纳一条绑定 Gate ServiceID 的 `reason=capacity` 结构化日志，Logger 满载时仍遵守有界非阻塞与丢弃计数契约，不读取或记录未认证帧。同步收紧既有坏 MAC 日志测试，直接检查完整输出不含原始 payload。生产 Gate 包测试与容量诊断定向 race 测试通过，未运行全仓验证。

2026-09-28 生产 Gate 日志故障隔离：在 Gate 运行中提前关闭共享 Logger 后，握手前 TCP 连接上限仍拒绝超额连接、`RejectedConnections` 仍准确增加，`Runtime.Stop` 正常完成。新增定向测试及对应 race 测试通过；本轮未修改生产实现，也不改变应用通常应在 Runtime 停止后再关闭 Logger 的资源顺序。

2026-09-28 生产 Gate 容量拒绝与健康流量组合：一条已认证连接占用唯一连接名额，四轮共 16 条额外原始 TCP 连接被拒绝；每轮拒绝后同一健康连接仍完成一次认证请求/响应，Agent 收到的请求字节匹配，`AcceptedConnections=1`、`RejectedConnections=16`、`InboundMailboxFull=0`。定向测试及对应 race 测试通过；本轮只增加验收，不修改生产实现，也不代表持续故障洪泛容量结论。

2026-09-28 生产 Gate 不完整 HTTP 请求头回收：普通 HTTP WebSocket 入口的唯一 TCP 名额由只发部分请求头的客户端占用，额外连接被拒绝并计数；约 5 秒 `ReadHeaderTimeout` 后该连接关闭，新的 HTTP 请求恢复得到 404。定向测试和对应 race 测试通过；本轮只验证已有超时配置，不修改生产实现，也不等同于公网 Slowloris 防护验收。

2026-09-28 DebugGate 不完整 HTTP 请求头回收：只发部分请求头的 TCP 连接约 5 秒后由 `ReadHeaderTimeout` 关闭；在其等待期间，另一条 WebSocket 连接仍成功创建 Agent 并收到断开通知。计数为 `AcceptedConnections=1`、`RejectedConnections=0`，确认 DebugGate 的连接上限只覆盖进入 `/ws` Handler 的请求，不覆盖原始 TCP 接纳。定向测试及对应 race 测试通过，本轮未修改生产实现。

2026-09-28 双 Gate 停机中断不完整 HTTP 请求头：生产 Gate 的普通 HTTP 入口与 DebugGate 均先确认连接正等待剩余请求头，再调用带 2 秒期限的 `Runtime.Stop`；两者都在期限内返回，客户端随后读到 EOF，无需等待 5 秒 `ReadHeaderTimeout`。两条定向测试及对应 race 测试通过；本轮未修改生产实现，不覆盖 TLS 握手中的原始 TCP 连接。

2026-09-28 生产 Gate 停机中断原始 TLS 握手：直连 WSS 入口接纳 TCP 连接后客户端不发送 ClientHello；确认连接仍等待握手，再以 2 秒期限停止 Runtime。停机完成后客户端读到 EOF，未创建 Agent，也未进入应用握手超时计数。定向测试及对应 race 测试通过；本轮未修改生产实现，不证明任意网络半开连接的跨机器行为。

2026-09-28 生产 Gate 原始 TLS 握手自然超时：直连 WSS 入口的唯一 TCP 名额由不发送 ClientHello 的连接占用，额外 TCP 连接被拒绝；约 5 秒后该握手连接关闭，新的 TLS 握手成功，说明容量名额已释放。定向测试及对应 race 测试通过；本轮只验证已有 `ReadHeaderTimeout` 的 TLS 阶段效果，不修改生产实现，也不把它混同于 WebSocket 升级后的应用握手超时计数。

2026-09-28 生产 Gate 的 HTTP/TLS 内部日志收口：真实坏 TLS 首包先复现 `net/http` 将握手错误写到标准日志、共享 Logger 没有对应记录；现通过 `http.Server.ErrorLog` 将安全的错误类别写入 Gate 来源绑定的共享 Logger，未配置 Logger 时标准错误也只输出类别。适配器不转录原始错误行，合成 panic 文本测试确认未认证内容不会出现在结构化日志。定向测试、生产 Gate 包测试和定向 race 测试通过；本轮未改 DebugGate 的 `net/http` 内部日志入口。

2026-09-28 DebugGate 的 HTTP 内部日志入口：测试先确认其 `http.Server.ErrorLog` 为空；现与生产 Gate 共用安全类别适配器，保持各自 Gate 来源绑定。定向测试覆盖 DebugGate 的接线与敏感文本过滤，并复验生产 Gate 的真实坏 TLS 首包和合成 panic 文本。两种 Gate 的受影响包测试通过；本轮未改变业务或客户端帧协议。

2026-09-28 无共享 Logger 的 Gate HTTP 内部日志：共用适配器的定向测试覆盖 TLS、Accept、Handler panic 和未知错误类别；Go 标准日志中只保留类别，不包含测试中的未认证原文。文档澄清标准日志默认写标准错误，但可由宿主重定向；本轮不改变生产实现。

2026-09-28 生产 Gate 在共享 Logger 已关闭时的 TLS 错误：真实坏 TLS 首包触发 `net/http` 错误日志后，连接仍关闭并归还容量；随后健康 TLS 连接仍可建立，Runtime 停机正常。定向测试及其 race 测试通过，验证日志失败不会把单条坏连接扩大为 Gate 接入故障；本轮不修改生产实现。

2026-09-28 Gate HTTP 内部日志满载：阻塞底层日志写入并填满容量为 1 的共享队列后，HTTP 错误日志适配器立即返回，丢弃计数增加 1；日志输出恢复后能正常关闭 Logger。定向测试及其 race 测试通过；本轮只验证满载 admission，不修改 Gate 或 Logger 的生产实现。

2026-09-28 生产 Gate 失败阶段压力样本扩展：`TestProdGateFailurePhaseStress` 在原应用握手/MAC 失败前加入坏 TLS 首包，确认 TLS 阶段没有创建 Agent，三类故障之后健康连接仍可请求/响应。显式开关下默认每阶段 32 次样本通过；每阶段 8 次样本及其定向 race 通过。坏 TLS 首包本身不计入应用层拒绝数，独立的 TCP 容量拒绝仍会计数；这是本机短时样本，不代表目标部署容量或完整故障矩阵。本轮不修改生产实现。

2026-09-28 客户端 WebSocket Close 原因脱敏：真实生产 Gate 握手前 Close 帧测试先复现客户端自填文本进入结构化日志；修复后连接故障诊断保留 `peer_close_code`，不记录对端文本。DebugGate 的相同日志边界用定向测试覆盖。两种 Gate 共用安全字段选择，受影响包测试和定向 race 测试通过；不改变关闭或 Agent 投递语义。

2026-09-28 ProdGate TLS 配置收口：构造测试先复现 `GetConfigForClient` 可替换根配置，现在启动前以 `ErrInvalidConfig` 拒绝；`GetCertificate` 动态选证书的真实 TLS 握手仍成功，Gate 最低版本为 TLS 1.2，实际协商 HTTP/1.1。定向测试和 `go test ./examples/websocket-gate/prodgate -timeout=2m` 通过，未运行全仓验证。首次包测试中既有的客户端 Close/停机竞态使一次运行返回 `websocket: close sent`，该用例单独运行及包测试复核通过；本轮未修改该关闭路径。

2026-09-28 Gate 重复关闭收尾：公共出站路径在客户端 Close 后可能再次尝试发送 Close，Gorilla 返回 `ErrCloseSent`；现在与 EOF、连接已关闭等情况一样视为关闭完成，仍保留其它真实 Close 帧和 socket 关闭错误。定向回归测试先复现错误归类，再验证生产 Gate 的客户端 Close/停机以及两 Gate 的注入关闭错误；公共包、ProdGate、DebugGate 包测试通过。未修改数据帧协议或关闭顺序，未运行全仓和 race 验证。

2026-09-28 动态证书失败隔离：新增真实 TLS 测试，让 `GetCertificate` 首次返回错误、第二次返回有效证书；首条握手失败后健康连接仍成功，Runtime 正常停止，共享日志不包含回调错误原文。定向测试和 ProdGate 包测试通过；本轮只新增验收与 RFC 说明，未修改生产实现，也未运行全仓或 race 验证。

2026-09-29 动态证书 panic 隔离：把同一真实 TLS 回归测试扩展为返回错误和 panic 两种子场景。证书回调首次 panic 时仅当前握手失败，下一条健康连接成功，Runtime 正常停止，Gate 日志不含 panic 原文。定向测试、定向 race 和 ProdGate 包测试通过；本轮只扩展验收与契约说明，未修改生产实现，也未运行全仓验证。

2026-09-29 Go/TypeScript 生产 Gate 联调：以 `go run ./cmd/debugserver --prod --addr 127.0.0.1:0` 启动现有示例，TypeScript CLI 使用 `--secure` 实际完成固定 X25519 握手及上行 MAC。普通 Echo 请求返回原 `request_id=1` 和文本，Notify 返回 Push，空文本返回业务 `TEXT_REQUIRED` 对应响应；三次 CLI 运行均成功，服务端停止后回环端口释放。本次走 `ws://`，模拟 Ingress 已终止 TLS 的内段，不是跨语言直连 WSS 或目标部署验收；未修改生产代码或客户端协议。

2026-09-29 Go/TypeScript 直连 WSS 示例：`debugserver --prod` 新增成对的 `--tls-cert`/`--tls-key` 文件参数，默认 WS 行为不变。使用临时自签名且包含 `127.0.0.1` SAN 的证书启动 `wss://`，TypeScript CLI 通过 `NODE_EXTRA_CA_CERTS` 信任该测试证书后，以 `--secure` 完成 TLS、X25519、上行 MAC 和 Echo 往返，返回原 `request_id=1`。服务端停止后端口释放，临时证书与私钥已删除；这是本机联调，不代表公网部署、证书管理或目标容量验收。
