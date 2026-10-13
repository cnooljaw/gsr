# GSR 文档

本目录记录 `gsr` 当前设计。项目按新实现推进，兄弟目录中的旧工程和 Skynet 源码只用于比较与学习，不构成本项目的兼容契约。

当前版本定为 GSR 1.0，已进入正式内部发布阶段。版本与接入范围见 [1.0 内部发布说明](RELEASE-1.0.md)，初次接入也可从根目录 [README](../README.md) 开始。

## 阅读顺序

客户端接入见 [WebSocket Gate、客户端协议与 Agent](RFC-WEBSOCKET-GATE.md)：WSS、固定 33 字节二进制握手、Protobuf Envelope（msg_id、uint32 request_id、payload）、X25519/HKDF 上行 MAC，以及请求/响应/通知/推送独立编号；首版不实现旧头兼容或 DES。生产 Gate 的完整验收仍按 RFC 的失败矩阵和压力计划推进。

默认业务入口是 `gsr.Run + MainService`：宿主管运行与停机，MainService 用 `self` 创建、命名和调用其它 Service。具体裁决见 [ADR 0011：以 Service 为中心的应用入口与统一 Command](adr/0011-service-first-application-api.md) 和 [双节点示例](../examples/twonode/README.md)：gsr.Run 启动 MainService，业务通过 self.Cluster() 通信，不需要 Host。

初次运行先看 [最小 CounterService 示例](../examples/counter/README.md)，只涉及本地 Service、Send、Call 和关闭。

在另一个仓库开发业务时，从 [独立业务项目开发指南](APPLICATION-DEVELOPMENT.md) 和 [业务项目 AGENTS.md 模板](templates/AGENTS.application.md) 开始，固定 GSR 依赖版本并明确业务状态、协议和资源 owner。

1. [项目术语](../CONTEXT.md)：ServiceID、NodeID、Message、ClusterEnvelope、Session、Source 和 Revision 的准确含义。
2. [API 设计规范](API-DESIGN.md)：Runtime 边界、消息管道、Cluster API 和 Skynet 风格。
3. [ADR 0001](adr/0001-local-service-id-and-cluster-routing.md)：Runtime 局部 ServiceID、Cluster 路由及 Source 绑定的裁决。
4. [ADR 0002](adr/0002-message-ownership-and-call-semantics.md)：未编码参数所有权、同步 Call 与 Reply completion 的裁决。
5. [ADR 0003](adr/0003-bounded-admission-and-send-contract.md)：有界 Mailbox/队列与 Send 接受语义的裁决。
6. [ADR 0004](adr/0004-service-runner-lifecycle-and-timer.md)：per-Service runner、动态创建、停止策略与 Timer 的裁决。
7. [ADR 0005](adr/0005-cluster-wire-and-connections.md)：Cluster wire、codec、NodeSender、入站连接 owner 和重连策略的裁决。
8. [ADR 0006](adr/0006-runtime-ids-and-reply-errors.md)：uint64 运行期 ID 与结构化 Reply error 的裁决。
9. [ADR 0007](adr/0007-command-context-call-and-timeout-stop.md)：CommandContext、Call 最大等待时间、启动期自身消息和无法强杀任务的裁决。
10. [ADR 0008](adr/0008-runtime-component-lifecycle.md)：Runtime 组件生命周期、启动回滚与 Core/Cluster 关闭顺序的裁决。
11. [ADR 0009](adr/0009-controlled-redirect-and-source.md)：受控 Redirect/Relay、Source 来源和 Call 代理边界的裁决。
12. [GSR 与 Skynet 概念对照表](SKYNET-COMPARISON.md)：本项目术语、Skynet 对应机制及有意不照搬的边界。
13. [实现路线图](IMPLEMENTATION-PLAN.md)：Core、Cluster、参考实现使用方式、纵向切片和每阶段验证范围。
14. [开发、审查与提交工作流](DEVELOPMENT-WORKFLOW.md)：单人单分支下的修改、测试、diff 审查和提交授权边界。
15. [Go 编码规范](GO-CODE-STYLE.md)：代码结构、错误、并发、接口、配置和评审门禁。
16. [统一 FIFO 日志服务](RFC-LOGGING.md)：记录处采集时间位置、共享队列顺序、终端和滚动文件输出、故障与最后关闭；外部 SLS 采集待部署验证。
17. [当前阶段验证清单](VALIDATION-STATUS.md)：已覆盖运行链、测试证据、当前验证结果与尚未验收的范围。
18. [最小双进程示例](../examples/twonode/README.md)：独立配置与日志目录、Send/Call、停机与恢复演练。
19. [本地规模基线](PERFORMANCE-BASELINE.md)：10,000 Service/Timer 与 Send/Call 测量结果、复现命令及口径限制。
20. [GSR 1.0 内部发布说明](RELEASE-1.0.md)：版本、代码基线、发布范围、接入边界和发布前验证结果。
21. [Cluster 节点间调用教程](CLUSTER-CALL-TUTORIAL.md)：双节点启动、Send/Call、参数与返回值类型、集合规范化、错误及测试覆盖。
22. [独立业务项目开发指南](APPLICATION-DEVELOPMENT.md)：依赖版本、项目布局、可运行 Service 骨架、所有权/生命周期、接入与验证，以及 AGENTS.md 模板。

后续出现需要长期保留的重大架构取舍时，新增 ADR 或 RFC，并从这里建立索引。

## 示例选择

- 普通业务：先看 Counter，再看双节点、WebSocket Echo、Redis/MySQL Service 示例。统一使用 `gsr.Run + MainService` 和当前 Service 的 `self` 能力。
- HTTP 管理入口：先看 [HTTP 示例](../examples/http-inbound/README.md) 中的 MainService 和 `HTTPIngress` 接线，再看底层测试夹具。
- 基础设施验证：显式 Runtime、ClusterClient、Transport 等组装只用于理解或测试框架边界，不是业务启动必经步骤。
