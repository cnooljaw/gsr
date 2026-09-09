# Cluster 两阶段关闭

状态：已获用户确认，首轮公开出站 Cluster 生命周期已实现。

## 已确认入口

```go
func (cluster *Cluster) Quiesce()
func (cluster *Cluster) Close(ctx context.Context) error
```

Quiesce 表示停止接纳和启动新工作，不表示全部任务已结束；Close 负责最终释放及等待。沿用 CONTEXT 中已有 Quiesce 术语，不增加另一组 Stop/Shutdown 别名。

## Skynet 依据与 Go 的区别

只读参考 `lualib/skynet/socketchannel.lua`、`service/clustersender.lua` 和 `service/clusterd.lua`：

- socketchannel 的 `channel:close()` 标记关闭、终止 dispatch 并关闭 socket。
- clustersender 的 `changenode` 在目标被关闭时调用 channel:close；clusterd 通过消息驱动目标更新与连接 agent 退出。
- 入站 socket close/error 事件由 clusterd 转成对应 clusteragent 的 exit 消息。

这依赖 Skynet 的 C socket 事件与 Lua coroutine/Service 调度。可以借鉴明确的连接 owner 和禁止关闭后继续连接，但不能把发送 exit 消息当作已经等待 Go goroutine 全部退出。所查路径也没有直接提供与 GSR Host/Core 停机顺序相同的两阶段接口。

## 第一阶段：Quiesce

首轮出站组装定义如下：

- 同步关闭 Cluster admission，拒绝新 Client 和已有 client 的新 Send/Call，禁止创建 sender 和开始新的 Dial/重连。重复调用幂等，不允许恢复接纳。
- 尚未开始投递的 Send 丢弃并进入 A 侧基础设施诊断；其原先成功 admission 的返回值不改变，不承诺排空发送。
- 尚未绑定连接的 Call 以停止原因结束等待，包括仍在排队或拨号中的请求；拨号取消，不能在晚到的 Dial 成功后把该 Call 发出去。
- 已绑定连接的 Call 保留原等待上限，可以接收正常结果、远端错误或连接错误；不因 Quiesce 统一立即失败，不重置 deadline。
- 已开始的单次 socket 写入可在原有限写超时内自然结束，不能承诺撤销远端执行。Quiesce 不主动关闭已建立的 socket，不等待 Handler 或读写循环结束。

Quiesce 不接收 context，也不进行网络 I/O 或等待 worker；它完成接纳门禁与停止信号的发布。队列诊断和 worker 收尾可以随后完成，不能据其返回宣称已释放资源。无返回错误是因为它只发起不可逆且幂等的本地停止动作；最终释放错误由 Close 返回。

绑定与 Quiesce 必须有统一的竞争边界：先绑定者可继续等回复，先 Quiesce 者不得新增绑定。不得把“先查一个 bool，再无锁写入”的检查当作该保证。

## 第二阶段：Close(ctx)

- 自动发起 Quiesce，因此仅出站应用也可以直接 Close。
- 结束剩余 Cluster Call 等待，取消 Dial，关闭 socket 解除 Read/Write 阻塞，并等待 sender、响应读循环和诊断生产者退出；不等待远端 Handler 真正返回，也不强杀本地业务 Handler。
- 清理必须先向全部目标发出停止/取消/关闭信号，再等待各目标；不能在第一个目标阻塞时漏掉其它目标的关闭。
- 同一 Cluster 只启动一次最终清理。调用方 context 只限制这次等待，取消后清理继续，后续 Close 可以继续等待同一流程，不重新创建关闭任务。
- nil context 拒绝；context 超时不等于资源已释放。完成后重复调用返回同一最终清理结果，错误合并保留，诊断出口在全部生产者退出后关闭。

这里 Close(ctx) 与 Core Close() 的签名不同：Cluster 使用调用方显式给出的等待预算，不在已确认 ClusterConfig 中再增加 ShutdownTimeout。它不要求 Runtime，也不代替 Host 调用 Runtime.Stop/Close。具体公开状态/诊断查询形式留在下一项，不提前增加状态枚举。

## Host 调用顺序与业务影响

正常 Host 停机维持已有顺序：停止可选入站 admission，Cluster.Quiesce，Runtime.Stop/Close，最后 Cluster.Close(ctx)。本方案首轮只实现出站组装；现有 tcpListener 的取消会同时关闭入站连接，不能宣称它已具备保留连接的独立入站 Quiesce，公开 Open 阶段必须另行接线和验证。

例：A 正在 Call B 查询背包，随后 A 开始停机。若 Call 已绑定连接，Quiesce 后仍可收到结果，让当前 Handler 结束；若还在队列中，则直接以停止原因返回。最后 Close 会结束仍未完成的等待，不无限等待 B。

停机后 OnStop 也不能发起新的 Cluster Call/Send。因此“退出时最后存一次远端数据”必须在 Quiesce 之前由应用的业务退出流程完成；框架不为 OnStop 开旁路，也不自动补发丢弃消息。

如果 Runtime 停止超时，Host 不能因此跳过 Cluster 的资源收尾；它应记录残留业务任务并执行有限等待的最终清理，不把异常残留误报为正常停机完成。该异常路径不改变 Core 无法强杀 Handler 的规则。

## 代价与验证

代价是增加一个明确的中间阶段，以及一次受管理的最终清理流程；收益是业务停机与连接释放顺序清楚，不把每个子组件的 Close 细节暴露给应用。

测试门禁覆盖 Quiesce 与绑定竞争、排队 Send 诊断、未绑定 Call 结束、已绑定 Call 正常回复、关闭后不拨号、多个目标先取消后等待、Close 调用方取消后继续收尾、重复/并发 Close，以及诊断出口最后关闭。Quiesce 通过共享等待表的同一锁阻止后续绑定，sender 的接纳关闭与等待分开，peer 的取消/关闭通知与等待分开；只有最终清理 owner 关闭共享诊断出口。
