# ADR 0003: 有界 admission 与 Send 的接受语义

## Status

Accepted

## Context

Mailbox 或 Cluster 出站队列如果无限扩容，突发流量会转化为不可控内存增长；如果 Send 无限阻塞，又会把背压隐蔽地传播到 Service Handler，甚至造成调度停滞。

网络 Send 还存在语义歧义：函数返回时只能可靠确认本地编码和排队是否成功，不能证明远端 Handler 已执行或业务状态已经持久化。

## Decision

- 每个 Mailbox 和 Cluster 出站队列都有明确、可配置的有限容量。
- admission 不无限阻塞，也不自动无限扩容。
- 本地 Mailbox 满时，Send 返回 `ErrMailboxFull`；Call admission 失败时立即返回错误并清理 PendingCall。
- Timer 投递遵守相同 admission 规则；失败时记录结构化 Runtime 诊断并结束本次触发，不静默无限重试。
- Cluster Send/Call 对无路由、已知断连、编码失败、出站队列满或关闭返回稳定 Runtime Error。
- Cluster Send 返回 nil 只表示请求已被本地 Cluster 边界接受或入队，不表示远端执行、业务成功或持久化成功。
- 需要业务结果时使用 Call；面对可能已经发出的网络请求，业务通过 RequestID、幂等或补偿处理不确定结果。

## Consequences

- 系统在过载时快速、明确地失败，内存上限与背压位置可配置、可观测。
- Send 的调用方必须决定丢弃、降级或上报，Runtime 不替业务偷偷重试。
- Timer 满载失败需要进入诊断和指标，否则可能表现为静默丢失。
- 仅依靠 Send 的 nil 返回值不能宣称业务成功；这与 Call 的响应语义保持清晰边界。
