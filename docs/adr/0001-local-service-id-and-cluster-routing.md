# ADR 0001: Runtime 局部 ServiceID 与 Cluster 路由分离

## Status

Accepted

## Context

GSR 需要同时保持 Core 简单和未来 Cluster 可扩展。一个候选方案是让 Core ServiceRef 从第一天携带 Node、Epoch、ServiceID 和 Generation；另一个方案遵循 Skynet，把 Service 地址限制在当前 Runtime，并由 Cluster 单独携带节点与传输上下文。

将全局身份加入 Core 会提前引入节点重启、发现、重建和 fencing 语义，并使本地 Runtime 依赖尚未实现的 Cluster 概念。Skynet 的现代 Cluster API 使用 `cluster.call(node, address, ...)` 和 `cluster.send(node, address, ...)`，远端收到网络消息后再调用目标节点的本地 Service 地址。

## Decision

- Core 地址类型固定为 `type ServiceID uint64`。
- ServiceID 只在当前 Runtime 生命周期内有效，不包含 Node、Epoch 或 Generation。
- 同一 Runtime 生命周期内不复用已经发放的 ServiceID；Service 重建获得新 ID。
- ServiceID 不持久化，也不能跨 Runtime 重启继续使用。
- Core 使用未编码的本地 Message，承载 Source ServiceID、Target ServiceID、SessionID、CommandID 和参数。
- 本地 Send 与 Call 共用同一个 Message、Dispatcher、Mailbox 和 Scheduler 路径；只通过 SessionID 是否为零区分是否等待 Reply。
- Send/Call 的 Source 由 Runtime 自动设置为当前发起 Service 的 ServiceID，业务调用者不能伪造。
- Cluster 内部使用显式 source 的 `sendFrom` / `callFrom` 入口；业务使用初始化时绑定当前 ServiceID 的 per-Service client，因此公开调用仍是 `Send(node, serviceID, ...)` 和 `Call(node, serviceID, ...)`。
- 不使用全局变量、goroutine-local 或调用栈推断当前 ServiceID。
- Cluster 使用独立的可编码 ClusterEnvelope；NodeID、连接、编码和远端会话不进入 Core Message 或 ServiceID。
- ClusterEnvelope 显式区分 request、response 和 error frame；Session 只负责关联，不能单独表达方向。
- 远端 Cluster agent 解码 ClusterEnvelope，创建新的本地 Message，再通过目标节点的 LocalDispatcher 调用本地 ServiceID。
- 稳定服务身份由 Core 外层的 ServiceName、Registry 或 Discovery 解决。

## Consequences

- Core 可以在没有 Cluster 的情况下独立实现和测试。
- 本地 Message 不进行网络序列化；ClusterEnvelope 可以独立选择编码和 Transport。
- 公开身份模型保持为一个简单标量，不需要提前设计 NodeEpoch 和 ServiceGeneration。
- Cluster 内部的数据流仍能显式审计 Source，业务调用方则不需要重复传参或获得伪造 Source 的能力。
- 数字形式的远端 ServiceID 是临时地址；节点断线、Runtime 重启或重新连接后必须重新解析。
- 旧 ServiceID 在同一 Runtime 生命周期内不会命中新建 Service；同一 Service 实例内部的迟到异步结果仍需领域 Revision、OperationID 或 RequestID fencing。
- 如果未来出现“跨 Runtime 重启持久引用具体 Service 实例”的真实需求，应在 Cluster/Discovery 层新增身份协议并建立新的 ADR，而不是静默改变 ServiceID 语义。
