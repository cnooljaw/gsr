# Cluster 可选入站组装

状态：资源归属、公开签名、配置字段、默认值、重复调用规则及停止接线约定已获用户确认；公开 Open、统一停止与入站诊断最小切片已实现。

应用入口已按 [ADR 0011](adr/0011-service-first-application-api.md) 简化为 `self.Cluster().Open(name)`。下文显式接收 Runtime/ListenConfig 的签名属于 `cluster` 包基础设施接口，不是业务需要手工组装的另一套启动流程。

## 应用层用法

MainService 创建业务 Service 后，使用当前上下文开放入站：

```go
cluster := self.Cluster()
if err := cluster.Register("echo", id); err != nil {
    return err
}
if err := cluster.Open("battle"); err != nil {
    return err
}
self.Exit()
return nil
```

共同节点表由 `gsr.Config.Cluster.Nodes` 提供。应用层 Open 取 `battle` 对应地址的端口，固定绑定 `0.0.0.0`；映射中的 host 用于其它节点拨号，不要求是本机可绑定地址。此模式面向可信 IPv4 内网，未认证监听不得暴露到不可信网络。端口零仅供本机测试动态分配，不进入出站路由。

Runtime、名称解析与关闭接线由 gsr.Run 内部完成。入口 Exit 不关闭监听或其它 Service；不调用 Open 仍可主动 Send/Call。`ListenAddress()` 返回实际绑定地址供观测，不负责发布地址。

## 当前依据

内部 tcpListener 已实现停止接纳与最终取消分离，入站请求通过 localRequestAdapter 落地到显式传入的本地 Runtime。公开 Cluster 已通过 Open 管理可选入站监听及其连接。

只读参考本地 Skynet：

- `lualib/skynet/cluster.lua` 的 cluster.open 单独调用 clusterd 的 listen，主动 Send/Call 不依赖它。
- `service/clusterd.lua` 的 command.listen 创建 gate Service，再调用 gate.open；数字端口路径返回实际监听地址与端口。
- `lualib/snax/gateserver.lua` 通过 C socketdriver 监听，Lua coroutine 等待监听结果；一个 gate 保存一个监听 socket，客户端连接另行管理。clusterd 每次 listen 可以新建 gate，因此不是全局只允许一个 listener。

GSR 沿用“显式开放入站、框架负责接线”，不照搬 Lua 模块单例和新建 gate Service 的管理方式。Go 的监听和连接任务必须由明确 owner 取消并等待。

## 已确认：归现有 Cluster 管理

以 Cluster.Open 作为可选组装动作，第一版一个 Cluster 最多持有一个 listener，不返回需要应用单独关闭的第二个资源 owner。

- NewCluster 和仅出站 Client 继续不要求本地 Runtime、NodeID 或监听。
- 底层 Open 显式接收监听配置与目标 Runtime；应用层入口由 gsr.Run 自动接线。名称解析仍属于 Core 之外的能力。
- Cluster 管理 listener、已接纳连接及其任务，但不负责启动、停止或关闭 Runtime。
- 第一版不支持多个 listener、运行中换绑 Runtime 或监听热切换；多 listener 并非 Skynet 限制，而是 GSR 当前没有对应需求时的范围约束。

例如战斗节点只主动调用大厅，可以不 Open；大厅要接收调用才 Open，并把请求落到大厅自己的 Runtime。两者均不需要通过本地 NodeID 表明身份。

战斗节点 A 的 Call 回复沿 A 主动建立的连接返回，不要求 A Open。大厅 B 若要反向主动调用 A，才需要 A Open，并由 B 建立独立出站连接。一个监听可接纳多个连接并访问目标 Runtime 的多个 Service，不是一条连接或一个 Service 的限制。Runtime 由应用启动并创建 Service；传给 Open 只指定投递目标，不转移其生命周期管理权。

## 已确认：基础设施签名

```go
func (cluster *Cluster) Open(target *runtime.Runtime, config ListenConfig) (string, error)
```

`target` 显式指定入站请求投递到的本地 Runtime；`ListenConfig` 承载入站监听配置，字段及默认值见下文。成功返回实际监听地址，格式为 `host:port`，不是需要独立关闭的资源对象。例如配置 `127.0.0.1:0` 时，由系统分配端口并返回实际地址；监听失败通过 error 返回。此签名已落地为 Go API。

Skynet 的数字端口路径返回实际地址与端口，GSR 沿用这一用途，以单个 Go 地址字符串表达，不照搬 Lua 的多种参数形式。

## 已确认：基础设施 ListenConfig 字段

实现进度：ListenConfig 类型及内部默认值/校验已落地；Open 已实现，配置已接入监听生命周期。地址格式校验复用出站规则，但监听允许端口 0，出站仍要求实际端口；校验阶段不做 DNS、绑定或名称解析回调。

```go
type ListenConfig struct {
    Address            string
    MaxConnections     int
    MaxConcurrentCalls int
    ResolveService     func(ServiceName) (runtime.ServiceID, error)
}
```

- 底层 Address 必填，按传入地址监听；应用层按节点名 Open 则明确构造 `0.0.0.0:port`。
- MaxConnections 限制整个 listener 接纳的入站连接数。
- MaxConcurrentCalls 限制每条连接同时等待的 Call 数，满载暂停该连接读取，不创建无界等待任务。
- ResolveService 是可选的外层名称解析函数；只使用 ServiceID 时无需提供。沿用现有入站适配器约束：解析器须并发安全且不执行无限阻塞 I/O，未提供时名称目标返回 Service 不存在错误，不影响 ServiceID 目标。

统一 Codec、帧长上限及写超时复用现有 ClusterConfig，不在 ListenConfig 重复配置，不维护 Command 编码注册表。入站本地 Call 的最大等待由目标 Runtime 控制。

已确认默认规则：Address 为空时报配置错误；MaxConnections 为零时使用 128，MaxConcurrentCalls 为零时使用 32，两个容量字段为负数时报错，为正数时使用指定值；ResolveService 可为 nil。默认最多 4096 个入站 Call 等待任务，按需创建，并非总 goroutine 数或内存上限，也不是压测得出的最佳容量。

Skynet gate 有连接数上限，默认 maxclient 为 1024；它依赖 Lua coroutine 等待处理。GSR 的入站 Call 使用连接 owner 管理的 Go 任务，因此同时限制连接数和每连接 Call 数，不直接照搬 Skynet 的容量作为 Go 的默认值。

## 已确认：注册与发现属于外部能力

底层保留 `Open(target, ListenConfig)` 与显式 Address；应用层 `gsr.Cluster.Open(name)` 从共同节点表选择端口。两者都只开放监听和入站接纳，不自动注册、续约、发布地址或参与集群成员管理。Core 与 Cluster 不判断业务资源的唯一所有权、一致性或过期。

外层 Registry/Discovery 维护 `NodeID → 可访问地址`，应用决定何时发布节点及服务。Cluster 使用目标路由管理投递与连接恢复；静态 `ClusterConfig.Nodes` 是当前已实现的地址来源，不是节点存活或业务就绪证明。动态发现结果如何更新 Cluster 路由仍待后续切片，本轮不预建通用发现接口，也不宣称已有运行期路由更新能力。

静态部署共用节点说明表，业务按名字 Open 即可，不手工构造 ListenConfig。此前仅提供显式 Address 的应用使用约定已由 ADR 0011 替代；底层接口仍保持显式地址语义。两层接口并非可互换的业务写法。

监听地址与发布地址用途不同：例如本机监听 `0.0.0.0:7000`，外部发布 `battle-17 → 10.0.0.37:7000`。底层 Open 的返回值与应用层 ListenAddress() 都是实际绑定地址，不保证其它节点可访问，也不自动把通配地址转换为发布地址。当前无需为此在 Cluster 增加 LocalNode 或发布地址配置。

## 已确认：重复调用与失败重试

- 首次 Open 成功后开始接纳入站请求；成功后再次 Open 返回已打开错误，即使传入相同配置也不作为幂等成功，不比较函数字段或替换 Runtime。
- Open 失败时清理本次创建的资源，允许应用修正配置或环境后重试；不自动重试监听，也不影响原有出站能力。
- 多个 Open 并发调用时只允许一个打开流程，其余返回错误，不排队创建第二个监听。
- Cluster 已 Quiesce 或 Close 后拒绝 Open，不允许重新开放；失败可重试不绕过此停止边界。

Skynet 的 clusterd 每次 listen 创建新的 gate，而单个 gate 通过 `assert(not socket)` 拒绝重复打开。GSR 已明确单监听 owner，因此采用成功后拒绝重复打开、失败回滚的规则，以 Go error 表达，不照搬 Lua assertion 或多 gate 组装。

## 收益与代价

应用不必自行组装 listener、连接任务、适配器和资源等待；Cluster 继续是网络资源的统一 owner。代价是 Cluster 内部要覆盖 Open 与停止的竞争、监听失败回滚和入站诊断生产者收尾，不能简单把一个监听调用塞进现有构造函数。

独立 Listener 对象便于未来多监听管理，但会让当前应用同时持有两个关闭 owner；本轮不推荐为尚未需要的能力扩大日常组装接口。

## 已确认：统一两阶段停止

沿用 Quiesce / Close 两个公开入口，不增加 StopListener。Quiesce 统一阻止新入站与出站请求，关闭监听，保留已接纳入站 Call 的连接及回复机会，不等待 Handler 或连接任务退出。监听关闭错误保存至最终 Close 合并返回，不改变 Quiesce() 签名。这替代原“不进行网络 I/O”的约定，准确边界为“允许关闭监听，不主动关闭已建立连接，不等待任务退出”。

gsr.Run 内部按 Cluster.Quiesce → Runtime.Close → Cluster.Close(ctx) 收尾，最后写入停止日志并关闭 Logger。底层 Close 自动发起 Quiesce，最终取消剩余等待、断开连接并等待网络任务，仍不管理 Runtime 生命周期；调用方 context 只限制本次等待，不中断唯一清理流程。此公开入站接线已实现，详见 [两阶段关闭](RFC-CLUSTER-SHUTDOWN.md)。

## 已确认：统一入站诊断

入站合入现有 Cluster.Diagnostics，新增 DiagnosticInboundFailure 类别及 RemoteAddress 字段；入站 RemoteNode 留空，可信请求信息可带 Session/Target/Command，不记录业务载荷。共用容量 256 与丢弃计数，不增加转发 goroutine。最终 Close 等全部入站及出站生产者退出后关闭出口；Handler 错误仍归 Runtime，Open 同步失败直接返回 error，成功后监听异常进入诊断。详见 [统一诊断出口](RFC-CLUSTER-DIAGNOSTICS.md)。此方案已实现。

认证仍是后续能力；Open 本身不构成可信网络边界，未认证监听不应暴露给不可信网络。

## 实现与验证

Open 在锁内预留唯一打开名额，在锁外解析及绑定地址，再与停止门禁竞争移交监听。停止先发生则关闭本次 socket 并返回错误；Close 等待正在打开的流程退出后才关闭诊断出口。监听任务使用已有 tcpListener，不新增独立公共 Listener 或诊断转发任务。

公开入口测试覆盖真实 TCP Call、重复打开、端口占用后重试、并发打开、Open/Close 竞争无监听残留、Quiesce 保留旧 Call 回复、关闭不接管 Runtime、入站请求关联字段及监听异常退出不提前关闭共享出口。共享诊断测试另覆盖容量溢出归属；既有内部入站测试继续保护连接名额与停止门禁。

公开边界回归另覆盖：Send Handler 失败只进入目标 Runtime 诊断，Call 返回领域错误且不重复产生 Cluster 网络诊断；检查两端生产者全部退出后的完整事件流。外层名称解析暂未返回时，Close 等待超时不得关闭诊断出口，解析返回后可继续等待同一清理流程完成。此测试不表示允许无限阻塞解析器，也不承诺强杀扩展代码。

公开协议故障回归覆盖完整坏载荷之后的同连接恢复，以及仅发送超限长度前缀时立即断开该连接、其它连接仍可调用。完整请求诊断保留可信字段，无法解析出请求的长度错误不填充 Session/Target/Command。这里的有界队列仍不是按来源限频机制。
