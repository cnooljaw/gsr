# 设计裁决索引

ADR 保留重要决定、参考依据和代价；按编号标识，不因目录整理重编号。当前调用方式先查 [API 契约](../API-DESIGN.md)，避免把历史上下文中的旧接口当成接入示例。

## Core 与消息

| ADR | 决定 |
| --- | --- |
| [0001](0001-local-service-id-and-cluster-routing.md) | Runtime 局部 ServiceID 与外层 Cluster 路由 |
| [0002](0002-message-ownership-and-call-semantics.md) | 消息所有权、串行 Call 和自动 Reply |
| [0003](0003-bounded-admission-and-send-contract.md) | 有界接纳及 Send 成功的含义 |
| [0004](0004-service-runner-lifecycle-and-timer.md) | 每 Service runner、生命周期和 Timer |
| [0006](0006-runtime-ids-and-reply-errors.md) | ID 单调分配及稳定 Reply error |
| [0007](0007-command-context-call-and-timeout-stop.md) | CommandContext、启动期消息、超时后任务归属 |
| [0008](0008-runtime-component-lifecycle.md) | 显式 Core 生命周期计划 |
| [0009](0009-controlled-redirect-and-source.md) | 来源绑定与受控 Redirect |

## 应用与扩展

| ADR | 决定与后续关系 |
| --- | --- |
| [0005](0005-cluster-wire-and-connections.md) | Cluster wire 与连接 owner；当前机制汇总于 [Cluster 设计](../design/CLUSTER.md) |
| [0010](0010-per-service-command-dispatcher.md) | Service 显式 Handle 与强类型 Dispatcher |
| [0011](0011-service-first-application-api.md) | Run/MainService 入口及统一命令编码；组装方法由 0012 更新 |
| [0012](0012-node-capabilities-and-service-logging.md) | Node 组装、来源绑定 Cluster、窄日志能力与唯一节点对象 |
| [0013](0013-client-protocol-boundaries.md) | SecurityMode 与业务协议分离，独立 Sproto 编解码职责 |
| [0014](0014-bounded-service-pools.md) | 通用有界 Service 池；尚未发布 |
