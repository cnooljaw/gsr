# Cluster 组装、连接与生命周期

本文定义 Cluster 基础设施的当前机制。普通业务使用 `gsr.Run → MainService → self.Cluster().Send/Call`，接入示例见 [Cluster 指南](../guides/CLUSTER.md)。Core 不持有 NodeID、编码或连接，外层 Node 维护名称。

## 1. 资源与身份

每个 Run 持有一个 Cluster，多个 Service 使用各自来源绑定的轻量视图，共享路由、编码器、ClusterCallRegistry 与每目标唯一 NodeSender。不同 Run 的资源相互隔离。

- 来源绑定取当前 Service 的 Self，不继承收到消息的 Source，不通过全局变量或 goroutine-local 推断身份。
- Source 只留在本地等待记录，不进入 wire；远端请求通过 `Runtime.Send/Call` 注入，目标看到 Source 为零。需要业务身份时使用显式参数。
- 仅出站节点无需本地 NodeID、Runtime 或监听；Open 只表示接收入站调用，不赋予或认证节点身份。
- Call 回复沿原连接返回。B 主动调用 A 是另一条业务请求，需 A 监听，并由 B 使用自己的出站连接。
- 一条入站连接由一个私有 inboundConnection 管理，不按对端地址推断 NodeID，不暴露业务 ConnectionAgent。

基础设施组装入口：

```go
func NewCluster(config ClusterConfig) (*Cluster, error)
func (cluster *Cluster) Client(source runtime.ServiceID) (*ClusterClient, error)
func (cluster *Cluster) Open(target *runtime.Runtime, config ListenConfig) (string, error)
func (cluster *Cluster) Quiesce()
func (cluster *Cluster) Close(ctx context.Context) error
```

NewCluster 只校验并复制配置，不 Dial、不监听、不创建每目标 worker。Client 绑定非零来源并借用共享组件，没有独立 Close，也不缓存同一 ID 的 client 指针。它是受信任组装接口，只能校验非零 ID，不能证明其存活或所属 Runtime；Run 负责使用真实来源，普通业务不调用该构造入口。

## 2. 配置与监听

底层 ClusterConfig 的数值零值采用有限默认，负值拒绝：

| 字段 | 默认与约束 |
| --- | --- |
| Nodes | 静态 `NodeID → host:port`，构造时复制，允许空表；NodeID/host 非空，出站端口 1–65535；不接受 URL 或多地址候选 |
| Codec | nil 使用 ValueCodec；所有命令统一编码，自定义实例须并发安全 |
| Logger | 借用共享 Logger，不负责关闭；Run 统一注入，应用配置不暴露此字段 |
| MaxCallWait | 5 秒，覆盖编码、排队和远端等待，调用方 context 只能缩短 |
| DialTimeout | 3 秒，限制一次 Dial，含标准拨号器地址解析 |
| WriteTimeout | 3 秒，限制一次 socket 写入，不含排队或远端处理 |
| OutboundQueueCapacity | 每目标 1024 条，满载返回 ErrOutboundQueueFull |
| MaxFrame | 4 MiB，包含四字节长度前缀 |

构造只做格式校验，不做 DNS 查询或连接探测。两个 NodeID 即使使用相同地址仍是两个路由目标。配置表不证明节点存活、业务就绪或资源所有权。队列条数和帧上限应结合目标数与消息大小调整，默认值不是容量承诺。

底层 ListenConfig：

```go
type ListenConfig struct {
    Address            string
    MaxConnections     int
    MaxConcurrentCalls int
    ResolveService     func(ServiceName) (runtime.ServiceID, error)
}
```

- Address 必填，必须有显式 host 和数字端口，监听允许端口 0。成功返回实际绑定地址 `host:port`，不保证外部可达。
- MaxConnections 默认 128，MaxConcurrentCalls 默认每连接 32；零采用默认、负值拒绝。最多 4096 个入站 Call 等待任务不等于总 goroutine 或内存上限。
- ResolveService 可为 nil；此时名称目标不存在，具体 ServiceID 目标仍可使用。解析器须并发安全，不执行无限阻塞 I/O。
- Codec、帧上限和写超时复用 ClusterConfig，本地 Call 等待上限由目标 Runtime 控制。
- 每 Cluster 最多一个监听；成功后重复 Open 报错，不比较配置。失败回滚后允许重试；并发只允许一个打开流程；Quiesce/Close 后不能重新 Open，不支持换绑 Runtime。

应用通过 `self.Node().OpenCluster(name)` 从共同节点表取端口并监听 `0.0.0.0:port`，通过 ClusterListenAddress 观测实际地址。Run 允许表中的端口零用于本机测试，但不会把它加入出站路由。底层 Open 不管理 Runtime 生命周期，也不自动注册、续约或发布 Discovery 地址。

## 3. 请求路径与背压

```text
当前 Service 的 Cluster 视图
  → ClusterClient 编码 → 每目标 NodeSender 有界队列
  → outboundPeer → 出站连接 → TCP
  → tcpListener → inboundConnection → localRequestAdapter
  → Runtime.Send/Call → 目标 Mailbox → Handler return
  → 原连接 Response/Error → 发起端 ClusterCallRegistry
```

Send 返回 nil 仅表示本地接纳。编码、路由、队列或停止错误在接纳前同步返回；接纳后的 Dial/写入失败归发送侧基础设施诊断，目标 Handler 错误归接收侧 Runtime。Send 不要求业务回执；需要结果使用 Call。

入站读循环按帧读取并校验：

1. Send 直接完成本地接纳，不等待 Handler，同连接 Send 保留接纳顺序。
2. Call 取得当前连接的有限名额后，由连接 owner 的任务调用 Runtime.Call，并沿原连接写回结果。
3. 名额耗尽暂停读取，不再创建任务或额外无界队列；后续 Send 也可能等待。它不伪装成目标 Mailbox 满载错误。
4. 多个 Call 按完成顺序回复，使用 ClusterSessionID 关联。并发任务调度可能改变本地接纳顺序，不承诺混合 Send/Call 的 wire 顺序等于执行顺序；依赖前一步结果时应先等其 Call 完成。
5. 连接结束时关闭 socket、取消本连接等待并等待任务退出。取消 context 不会自行解除 socket Read；owner 必须同时解除 I/O、容量背压和停止等待。

Runtime.Call 新建本地 Session，Source 为零，复用有界 Mailbox、自动 Reply 和有限 PendingCall，不创建 goroutine。ClusterSession 与本地 Session 分开，不借用业务 ServiceContext，不直接执行目标 Handler。未知合法 Command 交由目标解释；帧损坏与编码错误在网络/适配边界拒绝。

## 4. 连接恢复与等待归属

每目标一个 NodeSender worker 顺序推进发送，outboundPeer 负责 Dial、连接替换、唯一响应读循环、取消和等待，不另建第二层业务队列。

- 没有连接时，当前尚未发送的请求触发一次有限 Dial；空闲断线不启动永久重连循环。未知路由立即失败，不等待未来配置。
- Dial 失败只结束当前发送尝试。后续排队请求轮到时可再次 Dial，这是它的首次发送；已尝试写入的请求不重连重放。
- Call 的总 deadline 不因建连重置。调用方取消不直接强杀连接级 Dial，但写入前再次检查；连接 owner 关闭或 DialTimeout 能终止该 Dial。
- 每次替换先等待旧读循环退出，不能用新连接掩盖未收尾任务。逐请求 Dial 可能造成失败密集时的连接开销，不隐式增加熔断或冷却策略。

Call 预算在编码前建立，Client 在排队前登记 PendingCall，以覆盖总等待；worker 在 socket 写入前原子地将等待绑定到选定连接的内部 owner。先写再登记会使 Go 并发读循环中的快速响应丢失，因此不能照搬该顺序。

断线只结束已绑定该连接的等待，不扫描并失败整个 NodeID 的等待。尚未绑定的请求保留原 deadline；旧连接的迟到、重复或错归属响应不能完成新等待。completion 同时校验 Session、RemoteNode、Target、Command 与连接 owner；绑定、回复、取消和断线竞争时只能完成一次。逻辑/内存链路的公开 Complete 仅处理未绑定连接等待，不能绕过真实 TCP 的归属校验。

## 5. 两阶段关闭

### Quiesce：停止接纳

Quiesce 幂等且不可恢复，不等待任务结束：

- 拒绝新 Client/Send/Call、sender、Dial 和 Open；关闭监听，也禁止已有入站连接接纳新请求。
- 尚未投递的 Send 丢弃并诊断，已返回的接纳结果不改变；未绑定 Call 以停止原因完成。
- 已绑定出站 Call、已接纳入站 Call 保留原等待期限与回复机会，不主动关闭已建立 socket。已开始的写入沿原有限超时结束，不承诺撤销远端执行。
- 绑定和停止按统一同步边界竞争，不能用无锁的先检查再写入代替。入站 admission 边界为完整帧读取后、交给请求任务前的门禁，区别于 Runtime Mailbox 接纳。
- 停止后暂停读取；未接纳 Call 由原超时或最终断线结束，不新增停机拒绝 wire 协议。监听关闭错误由最终 Close 合并返回。

### Close：释放与等待

Close 自动 Quiesce，先向所有目标发出取消/关闭信号，再等待 sender、连接、入站任务和诊断生产者。它取消剩余等待、关闭 socket 解除 I/O，不等待远端 Handler 或强杀本地业务代码。

唯一清理流程不会因等待者取消而撤销。nil context 拒绝，context 只限制这次等待；后续 Close 继续等待同一流程，完成后返回同一合并结果。超时不代表资源已释放；不新增 Cluster ShutdownTimeout 或第二个 Listener owner。

Run 的关闭顺序为：停止 HTTP → Cluster.Quiesce → Runtime.Close → Cluster.Close → 最终日志 → Logger.Close。Cluster 与 Logger 分别使用五秒等待预算，Runtime 使用自身预算；各阶段错误合并，前一阶段失败不跳过后续清理。MainService Exit 本身不结束节点。需要远端保存的业务退出流程须在 Quiesce 前完成，OnStop 没有新的跨节点调用旁路。

## 6. 诊断与日志

每 Cluster 一个容量 256 的非阻塞 Diagnostics 队列，共享于入站与出站。重复获取返回同一通道，不是广播；满载丢弃新事件并累计 DroppedDiagnosticCount。事件不保留 Payload/Args，Err 保留底层错误链，但文本不保证脱敏。

| DiagnosticKind | 触发与可信元数据 |
| --- | --- |
| DiagnosticDeliveryFailure | 接纳后的 Dial/写入/发送任务失败，或关闭丢弃；记录目标节点、Session/Target/Command |
| DiagnosticConnectionFailure | 已建立出站连接异常或读循环 panic；无单请求归属时关联字段为零 |
| DiagnosticResponseRejected | 未知/迟到/错归属响应或坏帧；只有可信解码结果才附请求关联字段 |
| DiagnosticInboundFailure | 入站接收、适配、回复或成功 Open 后监听失败；RemoteAddress 为对端地址，RemoteNode 留空 |
| DiagnosticBadFramesSuppressed | 连接结束时重复坏帧汇总；只带 Suppressed 数，不重复错误明细或请求字段 |

RemoteNode 是出站目标，RemoteAddress 只作诊断，不是身份。程序判断用 errors.Is/As，不比较文本；不新增 LocalNode、Epoch、业务 OperationID 或载荷副本。

- admission 前同步失败不复制 DeliveryFailure；业务结果、领域错误、正常 Call 取消/超时走原结果路径。迟到回复仍可诊断。
- 一次断线可产生连接事件及受影响请求事件，不承诺全局根因去重。主动 Close 的预期连接结束不报 ConnectionFailure，丢弃 Send 仍保留诊断。
- B 的 Handler error 归 Runtime，不重复记为 Cluster 故障。Open 当场失败直接返回，成功后的监听异常才记 InboundFailure。
- Runtime/Cluster 在产生事件处直接写共享 Logger，Diagnostics 仅作观测，不能再转发到同一主日志。组装模式内部直接共享出口，不为每目标保留额外转发队列或 goroutine；独立 NodeSender.Failures 只用于底层独立接线，不双写。
- Quiesce 不关闭出口。Close 在全部生产者退出后关闭，不等待消费者读空；关闭后缓冲事件和丢弃计数仍可读取。不提供可靠审计或逐节点公平性。

完整帧格式错误、请求载荷解码失败、错误入站请求类型按“连接 × 固定类别”保留首条，重复项只累计 SuppressedDiagnosticCount，连接结束且任务退出后尝试汇总。首条满载不重试，不按秒重复或按任意错误文本建表。合法未知 Command、业务错误、Send 投递失败、断线及迟到响应不适用此规则。抑制数、Diagnostics 丢弃数与日志队列丢弃数各自独立；这不是全局防攻击或可靠审计机制。

## 7. 参考与取舍

依据为只读 Skynet 源码：

| 实现 | 可沿用的思想 | Go 中的差异 |
| --- | --- | --- |
| `lualib/skynet/cluster.lua`、`service/clusterd.lua` | 按目标管理 sender，主动调用与 Open 分离 | 显式 owner 与静态配置；不复制 Lua 模块单例或等待未知节点配置 |
| `service/clustersender.lua`、`lualib/skynet/socketchannel.lua` | 按需连接、连接复用、失败不重放、等待归属 | Go 的读写任务并行，写入前绑定等待；有界队列、Dial、取消与等待由 owner 管理 |
| `service/clusteragent.lua`、`lualib/snax/gateserver.lua` | 入站转本地请求、原连接回复、监听与连接分离 | Lua rawcall 挂起 coroutine；GSR Runtime.Call 阻塞当前基础设施任务，显式限制数量，不增加业务 Bridge Service |
| `lualib/skynet.lua` | 在故障边界记录来源与错误 | 不依赖 C 日志运行时或回调；共享有界事件与 Logger |

静态路由、单监听、按需连接和固定并发上限降低组装复杂度，代价是首请求建连延迟、满载阻塞读包、可能局部排队及诊断丢失。动态发现、认证、备用地址、多监听和自动业务重试不属于当前实现。验证入口见 [覆盖矩阵](../validation/COVERAGE.md)。
