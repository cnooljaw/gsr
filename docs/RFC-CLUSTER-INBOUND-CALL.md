# Cluster 入站 Call 的本地接线

状态：已获用户确认。采用 Runtime.Call 基础设施入口；正式契约已同步到 API-DESIGN、CONTEXT 和 ADR 0009。下文保留裁决时的背景与取舍。

当前接线为私有 `inboundConnection → localRequestAdapter → Runtime.Send/Call`，公开 ConnectionAgent 已移除。下文的 RequestHandler/ConnectionAgent 仅描述裁决时的测试替身，不是当前推荐 API。

## 裁决时的缺口

- `cluster.ConnectionAgent` 已能将 Request 交给 RequestHandler，并沿原连接写回结果；当前集成测试仍使用逻辑处理函数。
- `Runtime.Send` 支持基础设施以 `Source == 0` 注入本地消息。
- 本地 Call 目前只通过 `ServiceContext.Call` 发起。`runtime.call` 和 Service dispatcher 都要求有效的当前 Service 实例，不能将 nil 当作基础设施来源。
- 因此，真实入站 Cluster Call 尚不能直接接入本地 Runtime。不得直接调用业务 Handler，也不得在网络 goroutine 中借用某个 Service 的 ServiceContext。

## 参考实现核对

Skynet 的 `service/clusteragent.lua` 在入站处理时，单向请求调用 `skynet.rawsend`，需要结果的请求调用 `skynet.rawcall`；后者完成后使用原连接写回 Cluster session 的响应。`lualib/skynet.lua` 中的 `yield_call` 将本地 session 关联到当前 Lua coroutine，等待期间让出协程。agent 自身是 Skynet Service，本地调用有真实的 agent 来源身份。

GSR 可以沿用“远端请求转成本地新调用，结果沿原连接返回”的分层，以及本地 Session 与 ClusterSession 分离的设计。不能直接照搬 Lua 协程等待：GSR Service 使用 run-to-completion，一个 Service 的 Handler 等待 Call 时不会处理下一条消息。

旧 GameServiceRuntime 的 `cluster.Open` 通过 `runtime.OpenCluster` 组装传输。这是参考实现的边界选择，不能据此把 NodeID、Transport 或 wire 封包重新放入本项目 Core。

## 已确认方案：补充基础设施 Runtime.Call

确定的签名：

```go
// Call 供宿主或受信任基础设施调用本地 Service，Source 固定为零。
// 普通 Service 仍使用 ServiceContext.Call。
func (runtime *Runtime) Call(ctx context.Context, target ServiceID, command CommandID, args ...any) (any, error)
```

行为契约：

- 对称于已有 Runtime.Send，显式表示宿主/基础设施调用；不接受任意 source 参数。
- 创建新的本地 Session，经同一 Dispatcher、Mailbox、Service runner 和 PendingCall 完成路径执行；Handler 仍通过 return 回复。
- Source 固定为零。远端 NodeID、认证主体、原始业务身份保留在 Cluster/显式消息参数中，不伪装成本地 ServiceID。
- 最大等待时间使用 Runtime.Config.MaxCallWait，调用方 context 只能缩短；停止时由 Runtime 完成剩余等待。
- 不通过 `caller == nil` 隐式选择此路径；内部使用独立命名的基础设施分派入口，共享等待与完成机制。
- 普通 Service 继续使用绑定自身的 ServiceContext.Call，保留当前来源检查、自调用拒绝和 run-to-completion 规则。

入站接线将成为：

```text
Cluster Request
  → 外层解析 ServiceTarget、查找 codec 并解码参数
  → Runtime.Call(ctx, 本地 ServiceID, CommandID, args...)
  → 目标 Mailbox → Handler return → 本地 PendingCall 完成
  → 外层编码结果 → 原连接 Response/Error（原 ClusterSessionID）
```

代价：增加一个公开 Core 方法，并明确一种不归属于 Service 的本地 Call 来源。Go 包无法阻止持有 Runtime 指针的业务代码调用它，仍须依靠组装边界约束：业务 Service 只获得 ServiceContext，基础设施持有 Runtime。调用本身不创建 goroutine；网络层仍须限制并发请求数并管理取消、关闭和等待。

## 不新增 Core API 的替代方案

使用实际 Bridge Service 发起本地 Call，目标看到 Bridge 的 ServiceID。此方案保留现有 API，但需要把网络请求投递给 Bridge，再将 Bridge 的结果送回连接组件；单个 Bridge 等待时会串行阻塞后续请求。若使用多个 Bridge，还需管理分片、容量与关闭顺序。

已选择 Runtime.Call：与已有 Runtime.Send 对称，接线更短；不会把 Cluster 概念带入 Core。Bridge Service 适合未来确实需要由一个 Service 拥有网关业务状态的场景。

## 裁决后的第一轮实施范围

1. 同步 API-DESIGN、CONTEXT 和 Source 相关 ADR，锁定基础设施 Call 的显式入口。
2. 先测试 Source 为零、目标串行执行、有限超时、取消与停止收尾，再实现共享的本地 Call 路径。
3. 将 ConnectionAgent 的测试适配器接到真实 Runtime，验证本地 Session 与 ClusterSession 分离。
4. 入站网络并发上限和请求调度另按实际接线推进；本裁决不授权无限创建 goroutine，也不改变同一 Service 的执行顺序。
