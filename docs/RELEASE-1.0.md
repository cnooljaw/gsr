# GSR 1.0 内部发布说明

## 版本与阶段

- 对内版本名称：GSR 1.0。
- 完整版本号：`1.0.0`，统一记录在根目录 [VERSION](../VERSION)。
- 阶段确认日期：2026-10-12。
- 阶段：正式内部发布，供内部项目接入、集成验证与问题反馈。
- 当前代码基线：`b2a9f16`（为核心框架类型和示例补充编译期接口断言）。本轮补充版本与发布文档，未修改生产实现。

本说明记录进入内部发布阶段的范围和证据。Git 封版标识为本地附注标签 `v1.0.0`，定位包含版本文件、发布说明及验证记录的最终提交。封版流程为复核并提交发布资料，再建立标签；远端推送单独执行，不包含在本次本地封版中。

## 发布范围

| 模块 | 1.0 能力 |
| --- | --- |
| 应用入口 | `gsr.Run`、普通 MainService、来源绑定的 Service/Command 上下文、节点内名称表、Cluster 与 Logger 能力 |
| Core Runtime | 本地 Send/Call、自动 Reply、串行 Mailbox/runner、动态 Service 生命周期、一次性 Timer、panic recovery、结构化错误与只读观测 |
| Command 分发 | 构造时绑定强类型 Handler 的 Dispatcher，显式 `Service.Handle` 入口及核心角色的编译期接口断言 |
| Cluster | 静态节点路由、统一 ValueCodec、按需 TCP 连接、有界出站队列、原连接回复、断线后的新请求恢复、Quiesce/Close 与诊断 |
| Gate | 统一 Plain/MAC 模式、可选直连 TLS、连接与字节队列上限、认证/序号校验、关闭与连接级诊断 |
| Protocol | Protobuf Envelope、生成消息目录、类型化客户端 Handler、Response/Push 与通用错误响应 |
| 日志 | 共享有界 FIFO、来源绑定、终端/滚动文件输出、错误归属与最后排空关闭 |
| 外围接入 | HTTP 监听与窄入站能力、原生 Redis/MySQL 客户端创建，以及本地、双节点、WebSocket、HTTP、Redis/MySQL 示例 |

公开契约以 [API-DESIGN.md](API-DESIGN.md)、[CONTEXT.md](../CONTEXT.md)及现有行为测试为准。本文不新增接口或运行语义；历史路线图中的 Core v0.1 和切片编号保留其历史定位，不作为当前 GSR 版本号。

## 内部接入边界

- 普通业务只获得 ServiceContext，在 Handler 内同步使用；Service 之间通过消息请求状态变化，不持有彼此实现指针或自行启动 goroutine。
- 本地引用型 Args 交接消息独占快照；Gate 的字节 Send 在接纳时复制快照。Send 成功不等于送达或业务执行成功。
- Call 等待期间当前 Service 不处理下一条消息；必须设置有限等待。超时、重试、幂等和补偿由拥有操作语义的业务决定，Cluster 不自动重放请求。
- Cluster 当前用于可信网络，未提供节点身份认证或动态发现接线。客户端 Gate 的传输认证不代替玩家身份校验；业务 Agent 负责 token 鉴权，Echo 示例未实现登录业务。
- Plain 仅用于回环或私有 IP。公网客户端入口使用 WSS，可由 Ingress 终止 TLS 或由 Gate 直连终止；连接数、出站队列、Mailbox 和业务 I/O 等待预算由接入项目明确配置。
- 默认跨节点值类型遵循 ValueCodec 的规范化约定，业务结构体需在消息边界展开或编码，详见 [双节点参数约定](../examples/twonode/README.md#跨节点参数约定)。
- Go 不能强杀不返回的 Handler、OnStart、OnStop 或 Writer；外部资源仍由明确 owner 负责关闭，Logger 在 Runtime/Cluster 收尾后最后关闭。

## 发布前验证记录

环境：Go 1.24.13，darwin/arm64。Go module 的最低版本为 1.24.0；本轮未验证其它 Go 工具链或操作系统。

| 验证 | 本轮结果 |
| --- | --- |
| `go test ./... -timeout=2m` | 已通过；复用同日接口断言切片的结果，Go 代码与 `b2a9f16` 相同 |
| `go vet ./...` | 已通过 |
| `GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m` | 已通过；包含双进程正常停机与强杀后恢复，双节点包约 18 秒、Gate 包约 35 秒 |
| WebSocket 示例目录下 `npm run test:crypto` | 已通过；TypeScript 编译及 5 条认证测试，包括跨语言固定向量与序号耗尽 |
| `git diff --check` 与发布相关 Markdown 本地链接检查 | 已通过 |

详细的历史证据见 [验证清单](VALIDATION-STATUS.md)。默认 Go 测试及上述命令不会开启负载、Gate 压力或真实 MySQL 的显式开关；本轮不重新生成已跟踪的协议代码。

首次 race 执行因沙箱禁止 TCP 监听而出现 `bind: operation not permitted`；在允许本机监听的环境中运行同一命令后完整通过，未跳过网络测试或延长 timeout。部分未变化 package 使用 Go 测试缓存。

## 后续验证与维护

1. 内部接入项目验证实际业务负载、队列配置、超时和日志频率，记录延迟、拒绝率及关闭结果。历史开发机轻量 Handler 样本只提供测量参考。
2. 按部署环境补充跨机器断网/分区、进程故障、慢网络/慢磁盘、WSS 证书与外部日志采集验证；真实 Redis/MySQL 环境和长时容量测试按项目需求开展。
3. 收集内部接入问题，优先修复可复现缺陷并补充最相关的行为测试。公开 API、错误身份、wire 格式或生命周期变化继续遵守设计裁决流程，不以内部发布为由静默改变契约。
4. 封版使用明确提交与 `v1.0.0` 标签定位代码；后续版本记录相对该基线的变化。标签和推送沿用 [提交工作流](DEVELOPMENT-WORKFLOW.md)的授权边界。
