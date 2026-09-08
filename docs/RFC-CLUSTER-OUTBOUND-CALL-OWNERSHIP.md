# 出站连接与 Call 等待归属

状态：已获用户确认。采用连接范围的等待归属与断线清理；不新增业务 API 或 wire 字段。

## 原有接线风险

`ClusterClient.Call` 先在共享 `ClusterCallRegistry` 登记等待，再把 Request 接纳到 NodeSender 队列。此前 `receiveResponses` 在 socket 终止时调用 `failNode`，删除所有目标 NodeID 相同的等待。

因此，登记等待不等于已经使用旧连接。一个尚在出站队列的 Call 也可能被旧连接的 EOF 完成；如果后续直接接入重连，该 Request 还可能随后由 worker 写入新连接，而调用方早已收到旧连接错误。

此前文档要求“先清理旧连接，再接纳替代连接调用”，但仅串行化 Transport 建连/写入不够：登记等待发生在 Client 层，在出站队列之前。原 `failNode` 测试证明的是节点范围清理，不是连接范围隔离，现已由连接归属测试取代。

## Skynet 的实际机制

本地参考 `lualib/skynet/socketchannel.lua`：

- `channel:request` 先 `block_connect(self, true)`，写入 socket，再通过 `wait_for_response/push_response` 将等待登记到该 channel 的 `__thread`。
- `dispatch_by_session` 从该 channel 读响应并唤醒对应 coroutine；读写失败时 `wakeup_all(self)` 清理它自身登记的等待，不扫描全局同名目标的等待。
- `block_connect` 使用 `__connecting` 合并并发建连等待；这依赖 Lua coroutine 与 Skynet 调度。channel 可以更换底层 socket，不能将它误读成每个 fd 都有独立的全局 Session 表。

GSR 可以沿用“等待有明确的传输 owner”，但 Go 的读写 goroutine 可同时运行，不能照搬“先写再登记”而留下快速响应抢先完成的窗口。GSR 仍保留共享 Session 分配、Source 记录和有限超时，不把全局等待表拆回每 Service 一套。

## 已确认方案

1. Client 仍先登记 PendingCall，以覆盖出站排队和网络等待的总超时；尚未交给连接的等待不归任何旧连接。
2. worker 确定将使用的出站连接后，在尝试 socket 写入前，将该 Call 原子地绑定到这个连接的内部等待 owner。若 Call 已经取消/超时，则不再发出尚未写入的请求；若连接已经关闭，则不允许新增绑定。
3. socket 断线只失败属于该连接的等待。仍在队列、尚未绑定的 Call 继续受原 deadline 约束；处理到它时可尝试连接并首次发送。已经尝试写入、结果不确定的 Request 不自动转投新连接。
4. 网络回复除校验 Session、目标节点、Target 和 Command 外，还必须属于当前连接的等待 owner，防止旧连接完成不属于它的等待。绑定、回复、取消、断线竞争时只有一个完成者。
5. 建连失败只完成当前交给 Transport 的 Call，并保留底层原因；Send 仍只进入 A 侧基础设施诊断，不向 B 索要执行结果。
6. 重连更换内部 owner，不新增 NodeID 握手、Version、Epoch、Lease 或业务字段。归属可以使用进程内对象引用，不需要另发一套公开 ID，也不参与业务资源一致性判断。

本次只裁决“断线清理范围与等待归属”。建连超时、连接恢复的调度方式以及公开 TCP 组装 API 在后续切片明确，不引入无限业务重试。

## 代价与验证

代价是 PendingCall 增加一份私有连接归属，绑定与完成要在同一个同步边界处理；收益是旧连接清理不依赖全局暂停同节点 Client，也不会误伤排队或新连接的等待。

本轮实施最小内部绑定路径，并以以下行为作为门禁：

- 旧连接 EOF 只完成已绑定 Call，同节点未绑定及新连接 Call 不受影响。
- 写入前取消不发包；写入已经开始的取消不承诺撤销远端执行。
- 快速回复、取消、断线并发时恰好完成一次，不泄漏等待。
- 新旧连接的迟到或错归属回复不能完成其它等待。
- 后续 Dial 切片门禁：建连失败保留错误原因，Send 失败不改变已成功 admission 的返回值。

## 实施状态

内部 `connectionCalls`、`outboundConnection`、响应读循环和真实 TCP 测试夹具已接通；公开 `Complete` 仅保留未绑定连接的逻辑/内存链路用途，不能绕过网络归属校验。绑定是发送尝试的线性化点，不代表远端已接收。后续内部 outboundPeer 已实现单目标 Dial 与按需恢复，见 [按需建连方案](RFC-CLUSTER-ON-DEMAND-CONNECT.md)；公开 TCP 组装接口尚未实现。
