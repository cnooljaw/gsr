# Cluster 统一诊断出口

状态：已获用户确认，首轮出站公开诊断已实现；入站统一接线方案已确认、尚未实现。不改变已确认的 Send/Call 结果语义。

## 已确认入口

```go
func (cluster *Cluster) Diagnostics() <-chan Diagnostic
func (cluster *Cluster) DroppedDiagnosticCount() uint64
```

每个 Cluster 一个固定容量 256 的共享队列，沿用现有诊断队列容量，不增加 ClusterConfig 字段。所有内部生产者非阻塞投递，满载丢弃新事件并累加计数。不提供业务 callback、不绑定日志库、不写日志文件；应用的基础设施日志消费组件自行决定输出位置。

队列是单一消费流，不是广播订阅：重复调用 Diagnostics 返回同一通道，多消费者会分摊事件。推荐应用只安排一个日志消费 owner，负责等待退出与自身错误处理，不能由普通 Service 启动 goroutine。

## Skynet 的实际做法及取舍

只读参考 `service/clustersender.lua`、`lualib/skynet/socketchannel.lua` 与 `lualib/skynet.lua`：clustersender 报告请求失败；socketchannel 在未知 session、对端关闭和连接失败等路径调用 skynet.error；Lua 层的 skynet.error 绑定到 C 模块的 c.error。

GSR 可以沿用基础设施在故障边界报告的分工，不需要业务为每次 Send 接一个失败 callback。Skynet 的日志入口依赖其运行环境；GSR 使用显式、结构化的有界通道交给宿主，不照搬日志运行时，也不让外部日志函数在 NodeSender/读循环内运行。

代价是允许诊断丢失；优点是日志消费不会阻塞投递。需要可靠审计的业务必须独立建设持久化通道，不能把 Diagnostics 当成业务交付凭证。

## 事件内容

已确认字段与类别如下，其中 InboundFailure 和 RemoteAddress 是本轮待实现的入站扩展：

```go
type DiagnosticKind uint8

const (
    DiagnosticDeliveryFailure DiagnosticKind = iota + 1
    DiagnosticConnectionFailure
    DiagnosticResponseRejected
    DiagnosticInboundFailure
)

type Diagnostic struct {
    Kind       DiagnosticKind
    RemoteNode NodeID
    RemoteAddress string
    Session    ClusterSessionID
    Target     ServiceTarget
    Command    runtime.CommandID
    Err        error
}
```

| 类别 | 含义 | 元数据 |
| --- | --- | --- |
| DeliveryFailure | admission 后一次请求投递失败，包括拨号/写入失败、发送侧 panic 和关闭时丢弃排队请求 | 目标节点、请求 Session/Target/Command；Send 的 Session 为零。 |
| ConnectionFailure | 已建立出站连接异常结束或响应读循环 panic | 目标节点；无单个请求归属时 Session/Target/Command 为零值。 |
| ResponseRejected | 收到完整但不可接受的响应或坏帧，例如未知/迟到 Session、错归属、解码失败 | 目标节点；仅在已有可信解码结果时附请求关联字段，否则零值。 |
| InboundFailure | 入站接收、处理适配、回复发送失败或成功 Open 后监听异常退出 | 对端 socket 地址；监听故障无对端时为空。仅在已有可信请求信息时附 Session/Target/Command，RemoteNode 留空。 |

Kind 只用于诊断分类，错误判断继续用 errors.Is/errors.As，不通过字符串猜测“对方节点未启动”。保留底层错误链，不新增重复的错误码体系。RemoteNode 是出站目标，不是本地身份；入站 RemoteAddress 只作诊断标签，不是认证身份，不据其推断 NodeID。出站不要求填充 RemoteAddress。不预留 LocalNode、Epoch 或连接 ID。

不携带 Payload、Args、业务对象或完整 wire frame，不生成业务 OperationID，不把本地诊断回传远端。Err 可能包含底层地址或自定义错误文本，不能宣称天然脱敏；宿主输出日志时仍需按部署要求处理敏感内容。

## 与调用结果的边界

- admission 前失败已经由 Send/Call 同步返回，不再复制一条 DeliveryFailure。
- Send admission 后投递失败只报告诊断；原先的 nil 返回不改变，B 侧 Handler error 不反馈 A。
- Call 的业务成功/远端领域错误走 Call 返回值，不额外作为基础设施诊断。正常 caller cancellation/Call timeout 也由调用方处理；由此产生的迟到回复仍可记录 ResponseRejected。
- Call 的拨号、写入或连接异常可以同时导致 Call 返回错误与对应基础设施诊断；它们不是两个业务结果。
- 同一次底层断线可能产生一条连接事件及若干受影响的投递事件，不承诺按根因全局去重，不新增复杂的去重缓存。
- 主动 Close 所引发的预期 socket 关闭不报告为 ConnectionFailure；需要依据 owner 的关闭意图判断，不能把所有 net.ErrClosed 都一概忽略。Quiesce/Close 丢弃排队 Send 的 DeliveryFailure 仍保留。最终资源释放错误仍从 Close 返回。

入站事件归接收节点 B，不回传 A，也不改变 A 的 Send 返回值。B 的业务 Handler error 仍由 Runtime 诊断记录，不重复记为 Cluster 网络故障；Call 的远端错误回复语义不变。Open 当场失败通过 error 返回，不复制诊断；成功后监听异常退出进入 InboundFailure。正常停机不是基础设施故障，监听关闭错误仍按两阶段关闭约定由最终 Close 合并返回。

Skynet 的 gateserver 在监听错误边界调用 skynet.error，连接错误交给 handler；clusteragent 也在自身故障边界记录日志。GSR 沿用故障归属，不依赖 Lua/C 日志运行环境，入站直接写现有共享有界出口，不增加业务 callback 或日志转发任务。

## 内部组装与关闭

组装模式下 NodeSender 与响应读循环直接写入 Cluster-owned 的同一内部诊断组件。它是受控的非阻塞队列，不是外部 callback，不为每个目标保留额外诊断缓冲或转发 goroutine。现有独立 NodeSender.Failures 用于低层独立组装，不能在 Cluster 模式与 Diagnostics 双写，也不能让应用同时消费两套出口。

响应路径通过包内具体类型 `responseDiagnostics` 集中适配独立与共享出口，读循环只报告事件。该类型仅借用出口，不拥有队列或关闭责任：共享出口自行累计丢弃数，独立出口将丢弃数返回连接 owner，避免重复计数。共享出口过滤 owner 确认的预期关闭；底层独立响应出口继续保留全部终止错误。此调整不增加公共接口、转发 goroutine 或通用诊断框架。

Quiesce 不关闭 Diagnostics；Close 只有在全部生产者退出后才关闭通道。Close 不等待日志消费者，不要求缓冲区先被读空；关闭后的缓冲事件仍可读取。Host 可在 Close 完成后等待自己的日志消费组件读到通道结束，但消费组件的磁盘/网络输出不属于 Cluster 清理责任。

公开 Open 组装后，listener 与全部入站连接直接借用同一个 Cluster-owned 出口，共享容量 256 与丢弃计数，不另设入站公开通道或转发 goroutine。listener 退出不得关闭该共享通道，最终 Cluster.Close 等全部入站与出站生产者退出后才关闭。现有内部独立组装路径的队列所有权需明确保留，不与共享模式双写。

DroppedDiagnosticCount 单调累计队列满导致的丢弃次数，Close 后仍可读取，不包含主动过滤的正常关闭事件。共享队列不提供每节点公平性，噪声目标可能挤占其它目标诊断；当前有界性不等同于已有完整限频或可靠日志保证。

## 实施门禁

入站扩展需覆盖真实 TCP 坏帧与可信请求字段、回复写入失败、连接上限拒绝、成功 Open 后监听异常、正常停机过滤、Runtime Handler 错误不重复诊断、共享队列满载计数及入站退出不提前关闭共享出口。Open 同步失败不重复诊断，不把已确认契约写成已完成实现。

测试门禁包含无消费者时的连续失败调用、队列满载丢弃计数、发送失败元数据、真实 TCP 未知响应/连接异常、预期关闭过滤、Close 提前返回不关闭出口、全部生产者退出后关闭通道，以及组装模式不再分配每目标旧诊断队列。普通业务 Service 不获得 Diagnostics 或创建日志 goroutine 的权限。共享上报组件不创建 goroutine，也不调用外部日志函数。
