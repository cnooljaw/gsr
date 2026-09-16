# Cluster 公开组装入口

状态：出站组装、可选 Open、两阶段关闭和诊断已实现。本文描述 `cluster` 包的基础设施组装；普通应用使用 `gsr.Run` 与 `self.Cluster()`，不手工创建 Client。

当前应用入口及统一编码以 [ADR 0011](adr/0011-service-first-application-api.md) 为准；入站资源归属见已确认的 [Open 方案](RFC-CLUSTER-OPEN.md)。认证和动态路由尚未实现。

## 本轮只确认一个方向

由一个显式的 Cluster 组装对象管理 Cluster 资源，并为每个 Service 提供已有的 `ClusterClient`，不让应用逐个拼接 PendingCall、NodeSender、连接和响应读循环。此方向已确认：收口易用性，不新增业务调用能力。

这不是新的 Core Runtime，也不是通用组件容器。它只收回当前 TCP 集成夹具已验证的固定组装关系；每个子组件仍负责自己的执行和收尾。

## Skynet 的实际做法

只读依据：`../skynet/lualib/skynet/cluster.lua`、`../skynet/service/clusterd.lua`。

- `cluster.call/send` 获取并缓存目标 sender，业务不自己创建 socketchannel 或响应循环。
- `clusterd` 保存 `node_address` 与 `node_sender`，按目标管理 sender，处理地址配置及更新。
- `cluster.open` 单独请求 `clusterd` 创建 gate 并监听；主动调用路径不要求先 open。
- `cluster.reload` 可以更新地址；未知目标在未启用 `nowaiting` 时还可以等待后续配置。此机制依赖 Lua coroutine 的 wait/wakeup 与服务间消息调用。

GSR 可以直接借鉴“集中组装、按目标管理、主动调用与监听分离”。不照搬 Lua 模块单例、协程等待表或等待未知节点配置的行为：Go 使用显式依赖、已有有界队列和有限 Call 等待，不增加另一层隐式缓存队列。

## 推荐分工

| 使用者 | 负责什么 | 不需要接触什么 |
| --- | --- | --- |
| Go main | 向 gsr.Run 提供配置和普通 MainService | 手工创建 Cluster、Client 或排列关闭步骤 |
| MainService | 通过 self 创建业务 Service，使用 self.Cluster() 注册名称、按节点名 Open | socket、Session 表、来源绑定接线 |
| 普通业务 Handler | 使用自身上下文的 Cluster().Send / Call | NodeSender、Transport、PendingCall、手动重连 |
| gsr.Run 内部适配层 | 为当前 Self 绑定 Client，管理固定关闭顺序 | 全局当前 Service 或 goroutine-local |

绑定 ServiceID 的入口是受信任组装能力，不授予普通业务每条消息任意选择 Source 的能力。本文不引入 ServiceRef，也不通过全局变量或 goroutine-local 推断当前 Service。

## 已实现的出站组装边界

基础设施 Cluster 保持以下边界：

- 启动时接受并复制静态 `NodeID → TCP 地址` 表，codec 和容量配置经校验后不可变。
- 所有 client 共享一个 ClusterCallRegistry；每个已使用目标最多一个 NodeSender 和 outboundPeer。
- 目标第一次使用时创建本地发送组件，但解析 sender 不做网络 Dial；拨号仍在已有 NodeSender worker 的投递阶段发生，保留 Send 的 admission 语义。
- 目标不在配置表中立即返回稳定路由错误，不等待未来配置、不创建未知目标的等待队列。
- sender 创建与关闭必须互斥，避免并发 client 为同一目标创建多个 worker。资源规模受配置目标数及各目标有界队列约束。
- 仅出站实例不要求本地 Runtime、NodeID 或监听。可选入站由 Open 单独开启，不让创建出站对象隐式 open。

地址热更新、Discovery、认证和多个 listener 不属于当前能力；公开诊断形式已实现。内部 Transport 替换边界继续用于测试，但不作为普通业务的日常组装入口。

## 代价与边界

新增一个固定的资源 owner，可以减少应用接线错误，并让共享等待表、唯一 sender 和关闭归属一致。代价是需要明确这个 owner 的生命周期；不能只提供一个构造函数而把关闭责任重新推回使用者。

公开创建/绑定签名与关闭阶段已确认。gsr.Run 内部先停止入站与新的出站 admission，再停止 Core，最后释放 Transport 和连接任务；具体见 [两阶段关闭](RFC-CLUSTER-SHUTDOWN.md)。不要求业务持有 Host 对象。

第一版静态地址表意味着不支持运行中修改目标映射；DNS 地址解析仍可由标准 Dialer 在拨号时执行，这不等同于 GSR 提供配置热更新。

## 后续验证要求

- 多个 client 访问同一目标只创建一个发送器，访问不同目标互不阻塞。
- 未配置目标的 Send/Call 在 admission 前失败，不启动 Dial 或遗留等待。
- 修改调用方原始配置 map 不改变已构造对象的路由。
- 共享 Session 不冲突，Source 只保存在本地等待记录中。
- 保留已有真实 TCP Send/Call、断线恢复、不重放和关闭测试。

创建/绑定类型名、签名、首轮 Config、默认超时、两阶段关闭与诊断出口均已确认。本文不新增另一种业务 Call。

## 创建与绑定签名（已确认）

沿用现有 `NewClusterClient`、`NewNodeSender` 的显式命名风格：

```go
func NewCluster(config ClusterConfig) (*Cluster, error)
func (cluster *Cluster) Client(source runtime.ServiceID) (*ClusterClient, error)
```

`NewCluster` 只校验配置、复制静态路由并建立共享本地管理状态。不拨号、不监听、不创建每目标 worker，也不需要传入 Runtime 或本地 NodeID。没有阻塞 I/O，因此不添加 context 参数；有限 Dial 仍发生在后续请求投递阶段。

`Client` 只创建绑定来源的轻量 client，复用 Cluster 的 codec、sender resolver、等待表和有限 MaxCallWait。它不创建 socket、队列或独立等待表，不需要独立 Close。重复获取同一 ServiceID 的 client 不创建第二个 sender；不承诺 client 指针相同，也不为此增加 client 缓存。

返回 error 用于明确拒绝零 ServiceID、无效或已停止接纳的 owner；Quiesce 后不再允许创建 client。不提供 Must 变体或另一套 BindClient 别名。

### 来源绑定的使用边界

Client(source) 是受信任基础设施入口。gsr.Run 的 Service 适配器在 OnStart 前从 Core 上下文的 Self() 取值，创建来源绑定入口；业务仅通过 self.Cluster() 使用，不手工创建或注入 client，也不保存 ServiceContext 或 CommandContext。

选择 ServiceID 而不是把整个 ServiceContext 交给 Cluster，是因为 Cluster 只需身份值，不应顺带获得本地 Send、Call、Timer 能力。ServiceContext 是接口，换成这个参数也不能自动成为不可伪造的安全凭据。

仅出站 Cluster 不依赖 Runtime，因此它只能校验非零 ID，不能证明这个 Service 当前存活、属于哪个 Runtime 或是否已停止。组装层必须使用同一 Runtime 的真实身份，不能把不同 Runtime 中数值相同的 ID 混进一个 Cluster 的来源空间。此限制沿用现有 ClusterClientConfig.Source 的信任边界，不新增跨 Runtime 身份协议或存活检查。

普通 Handler 只获得已经绑定好的 gsr.Cluster 入口，不获得创建任意来源 client 的管理能力。Source 仍不进入 wire，也不能为单次 Send/Call 覆盖。

### 与现有入口的关系

常规应用只推荐 `gsr.Run → MainService → self.Cluster().Send/Call`。`NewCluster → Client` 和 `NewClusterClient(config)` 都是基础设施接线入口，不作为普通业务的第二套推荐写法；保留低层构造器，不添加兼容别名。

ClusterConfig 与默认值见下文；Quiesce()、Close(ctx) 与 Open 均已实现。应用层 Open 按节点表名称开放监听，底层 Open 显式接收 Runtime 和 ListenConfig。

## 当前基础设施配置

只包含以下字段，不加入本地身份、Runtime、监听地址、Dial 回调或可注入的 PendingCall 表：

```go
type ClusterConfig struct {
    Nodes                 map[NodeID]string
    Codec                 PayloadCodec
    Logger                *logging.Logger
    MaxCallWait           time.Duration
    DialTimeout           time.Duration
    WriteTimeout          time.Duration
    OutboundQueueCapacity int
    MaxFrame              int
}
```

| 字段 | 已确认规则 |
| --- | --- |
| Nodes | 静态目标地址表；构造时复制，允许 nil/空表，不读取配置文件或环境变量。 |
| Codec | nil 使用默认 ValueCodec；所有 Command 统一编码，不登记命令准入清单，由目标 Handler 判断命令。 |
| Logger | 借用日志入口，Cluster 不负责关闭；应用使用 gsr.Run 时由 Logging 配置统一创建并注入，不自行设置。 |
| MaxCallWait | 默认 5 秒，覆盖编码、排队与远端等待；调用方 context 只能缩短。 |
| DialTimeout | 默认 3 秒，每次拨号的上限，包含标准拨号器的地址解析等待。 |
| WriteTimeout | 默认 3 秒，每次 socket 写入的上限，不包括排队或远端执行。 |
| OutboundQueueCapacity | 沿用每目标 1024 条的默认有界队列。 |
| MaxFrame | 沿用 4 MiB 的完整帧默认上限，包含四字节长度前缀。 |

五个数值配置统一为零使用默认值，负数拒绝，不支持用零或负数表示无限。Dial/Write 的 3 秒是本次确认的新公开默认值，不是现有内部实现的默认值；内部 outboundPeer 目前要求显式传入正数。MaxCallWait 的 5 秒与现有 Core 默认值保持一致，但 Cluster 不读取或绑定 Runtime 配置。

MaxCallWait 是调用方等待预算，不是 socket 的总执行预算。比如调用方 1 秒后放弃等待时，尚在进行的连接级 Dial 仍可在自身 3 秒上限内结束；实际写入前会检查等待是否已取消，不因此补发已取消 Call。已经开始写入的请求不能承诺撤销远端执行。

地址只接受非空 host 与数字端口组成的 host:port（IPv6 使用方括号），端口范围 1–65535。构造只做格式校验，不进行 DNS 查询或连接探测；格式正确不等于目标可达。NodeID 不得为空，地址不作为本地身份。第一版不接受 URL、多地址候选或特殊布尔值，避免多套配置写法。

Nodes 的 map 副本归 Cluster 所有；自定义 Codec 必须遵守并发安全契约，不做通用深拷贝。两个 NodeID 即使配置相同地址，也仍按两个路由目标管理，不额外做地址去重或身份推断。gsr.Run 额外允许共同节点表中的端口零用于本机测试监听，并在交给底层 Cluster 前从出站路由中剔除；它不是可拨号地址。

这些默认值用于开箱即用，不是容量承诺：队列条数乘以帧上限可能产生较大内存占用，而且每目标都有独立队列；部署时应按目标数和消息体大小调低上限。首轮不增加另一套全局字节预算，以免在本次配置中引入新的 admission 语义。

两阶段关闭与 [统一诊断入口](RFC-CLUSTER-DIAGNOSTICS.md) 已实现。底层公开入口集中在 cluster/cluster.go，配置校验在 cluster/config.go，诊断值与队列在 cluster/diagnostic.go；NodeSender、outboundPeer 和共享等待表继续承担各自职责。已有真实 TCP 往返、断线恢复、来源共享、Quiesce/绑定竞争、Close 取消等待、Open 和诊断溢出测试。当前验证范围见 [验证清单](VALIDATION-STATUS.md)。
