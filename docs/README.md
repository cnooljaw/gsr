# GSR 文档

GSR 是 Go 游戏服务器 Service Runtime。已发布版本为 **v1.0.3**，包含有界 Service 池。固定依赖接入时使用对应标签的文档，不能把开发分支 API 当成已发布能力。

## 1. 运行与接入

建议按以下顺序阅读：

1. [项目简介与快速开始](../README.md)：运行最小 Counter 示例。
2. [独立业务项目开发指南](guides/APPLICATION.md)：依赖版本、项目结构、MainService 和状态归属。
3. [Cluster 调用指南](guides/CLUSTER.md)：双节点、参数编码、返回值与错误。
4. [客户端协议接入](guides/CLIENT-PROTOCOLS.md)：Protobuf / Sproto、Agent 和 TS 副本同步。
5. [Service 池](guides/SERVICE-POOLS.md)：固定并行成员、普通 Call、容量与失败语义；自 v1.0.3 提供。

| 场景 | 可运行示例 |
| --- | --- |
| 本地 Send/Call、名称与退出 | [Counter](../examples/counter/README.md) |
| 跨节点调用、停机与恢复 | [双节点](../examples/twonode/README.md) |
| 多调用者访问同一服务池 | [Service 池](../examples/service-pool/README.md) |
| WebSocket、协议与连接 Agent | [WebSocket Echo](../examples/websocket-gate/README.md) |
| HTTP 管理入口 / 出站调用 | [入站](../examples/http-inbound/README.md)、[出站](../examples/http-outbound/README.md) |
| 外部数据资源 | [Redis](../examples/redis-user-cache/README.md)、[MySQL](../examples/mysql-product/README.md) |

## 2. 理解契约与设计

先查 [术语](../CONTEXT.md) 和 [API 契约](API-DESIGN.md)，再按问题查看具体设计：

- [Cluster](design/CLUSTER.md)：组装、连接归属、背压、关闭及诊断。
- [Gate 与客户端协议](design/GATE-PROTOCOL.md)：字节边界、握手、认证、协议分发与连接生命周期。
- [日志](design/LOGGING.md)：共享 FIFO、快照、有界队列、文件滚动和失败状态。
- [Runtime 诊断](design/RUNTIME-DIAGNOSTICS.md)：Handler 事件、截断、丢弃与关闭。
- [Skynet 对照](design/SKYNET-COMPARISON.md)：概念及运行机制差异。
- [设计裁决索引](adr/README.md)：已接受的取舍及后续替代关系。

## 3. 修改与验证

- [开发工作流](development/WORKFLOW.md)：修改、验证、审查、提交和临时资料归属。
- [Go 编码规范](development/GO-CODE-STYLE.md)：代码与评审规则，以及按风险选择验证。
- [业务项目 AGENTS.md 模板](development/AGENTS.application.md)：在独立业务仓库使用。
- [验证覆盖](validation/COVERAGE.md)：测试入口、最近验证基线及尚未验收范围。
- [性能测量](validation/PERFORMANCE.md)、[混合 Send 负载](validation/MIXED-LOAD.md)：历史环境、数据、复现方式和结论限制。
- [后续工作](ROADMAP.md)：当前缺口与按需求推进的能力。

## 4. 版本与迁移

发布说明记录发布当时的能力与验证，当前契约以上述文档为准。

| 版本 | 主要内容 |
| --- | --- |
| [v1.0.3](releases/v1.0.3.md) | 有界 Service 池、可运行示例与文档重组 |
| [v1.0.2](releases/v1.0.2.md) | Gate SecurityMode 命名、独立 Sproto 接入与正式依赖 |
| [v1.0.1](releases/v1.0.1.md) | Node 组装、来源绑定的 Cluster 与窄日志入口 |
| [v1.0.0](releases/v1.0.0.md) | 首次内部发布及接入边界 |

## 文档维护

指南回答如何接入，API 定义行为，设计文档解释机制，ADR 保留重要取舍。示例 README 记录运行步骤；验证文档只保留覆盖范围、可复现证据与限制。执行流水、临时审批故障、逐轮提交状态和已完成任务清单不作为长期文档维护；历史版本可从 Git 标签和提交查看。
