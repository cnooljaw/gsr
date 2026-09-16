# GSR 文档

本目录记录 `gsr` 当前设计。项目按新实现推进，兄弟目录中的旧工程和 Skynet 源码只用于比较与学习，不构成本项目的兼容契约。

## 阅读顺序

初次运行可先看 [最小 CounterService 示例](../examples/counter/README.md)，只涉及本地 Service、Send、Call 和关闭。

1. [项目术语](../CONTEXT.md)：ServiceID、NodeID、Message、ClusterEnvelope、Session、Source 和 Revision 的准确含义。
2. [API 设计规范](API-DESIGN.md)：Runtime 边界、消息管道、Cluster API 和 Skynet 风格。
3. [ADR 0001](adr/0001-local-service-id-and-cluster-routing.md)：Runtime 局部 ServiceID、Cluster 路由及 Source 绑定的裁决。
4. [ADR 0002](adr/0002-message-ownership-and-call-semantics.md)：未编码参数所有权、同步 Call 与 Reply completion 的裁决。
5. [ADR 0003](adr/0003-bounded-admission-and-send-contract.md)：有界 Mailbox/队列与 Send 接受语义的裁决。
6. [ADR 0004](adr/0004-service-runner-lifecycle-and-timer.md)：per-Service runner、动态创建、停止策略与 Timer 的裁决。
7. [ADR 0005](adr/0005-cluster-wire-and-connections.md)：Cluster wire、codec、NodeSender、ConnectionAgent 和重连策略的裁决。
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

后续出现需要长期保留的重大架构取舍时，新增 ADR 或 RFC，并从这里建立索引。
