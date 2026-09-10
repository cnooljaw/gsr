# Cluster 可选入站组装

状态：资源归属、公开签名、配置字段及默认值已获用户确认；公开 Open 尚未实现，重复调用规则及停止接线仍待逐项确认。

## 当前依据

内部 tcpListener 已实现停止接纳与最终取消分离，入站请求通过 localRequestAdapter 落地到显式传入的本地 Runtime。公开 Cluster 当前仅管理出站资源；不能把内部能力已完成等同于公开 Open 已完成。

只读参考本地 Skynet：

- `lualib/skynet/cluster.lua` 的 cluster.open 单独调用 clusterd 的 listen，主动 Send/Call 不依赖它。
- `service/clusterd.lua` 的 command.listen 创建 gate Service，再调用 gate.open；数字端口路径返回实际监听地址与端口。
- `lualib/snax/gateserver.lua` 通过 C socketdriver 监听，Lua coroutine 等待监听结果；一个 gate 保存一个监听 socket，客户端连接另行管理。clusterd 每次 listen 可以新建 gate，因此不是全局只允许一个 listener。

GSR 沿用“显式开放入站、框架负责接线”，不照搬 Lua 模块单例和新建 gate Service 的管理方式。Go 的监听和连接任务必须由明确 owner 取消并等待。

## 已确认：归现有 Cluster 管理

以 Cluster.Open 作为可选组装动作，第一版一个 Cluster 最多持有一个 listener，不返回需要应用单独关闭的第二个资源 owner。

- NewCluster 和仅出站 Client 继续不要求本地 Runtime、NodeID 或监听。
- 只有调用 Open 时显式提供监听配置与目标 Runtime；名称解析仍属于外层能力。
- Cluster 管理 listener、已接纳连接及其任务，但不负责启动、停止或关闭 Runtime。
- 第一版不支持多个 listener、运行中换绑 Runtime 或监听热切换；多 listener 并非 Skynet 限制，而是 GSR 当前没有对应需求时的范围约束。

例如战斗节点只主动调用大厅，可以不 Open；大厅要接收调用才 Open，并把请求落到大厅自己的 Runtime。两者均不需要通过本地 NodeID 表明身份。

战斗节点 A 的 Call 回复沿 A 主动建立的连接返回，不要求 A Open。大厅 B 若要反向主动调用 A，才需要 A Open，并由 B 建立独立出站连接。一个监听可接纳多个连接并访问目标 Runtime 的多个 Service，不是一条连接或一个 Service 的限制。Runtime 由应用启动并创建 Service；传给 Open 只指定投递目标，不转移其生命周期管理权。

## 已确认：公开签名

```go
func (cluster *Cluster) Open(target *runtime.Runtime, config ListenConfig) (string, error)
```

`target` 显式指定入站请求投递到的本地 Runtime；`ListenConfig` 承载入站监听配置，字段及默认值见下文。成功返回实际监听地址，格式为 `host:port`，不是需要独立关闭的资源对象。例如配置 `127.0.0.1:0` 时，由系统分配端口并返回实际地址；监听失败通过 error 返回。此签名尚未落地为 Go API。

Skynet 的数字端口路径返回实际地址与端口，GSR 沿用这一用途，以单个 Go 地址字符串表达，不照搬 Lua 的多种参数形式。

## 已确认：ListenConfig 字段

```go
type ListenConfig struct {
    Address            string
    MaxConnections     int
    MaxConcurrentCalls int
    ResolveService     func(ServiceName) (runtime.ServiceID, error)
}
```

- Address 必填，显式指定监听地址，不默认开放所有网卡。
- MaxConnections 限制整个 listener 接纳的入站连接数。
- MaxConcurrentCalls 限制每条连接同时等待的 Call 数，满载暂停该连接读取，不创建无界等待任务。
- ResolveService 是可选的外层名称解析函数；只使用 ServiceID 时无需提供。沿用现有入站适配器约束：解析器须并发安全且不执行无限阻塞 I/O，未提供时名称目标返回 Service 不存在错误，不影响 ServiceID 目标。

编码注册表、帧长上限及写超时复用现有 ClusterConfig，不在 ListenConfig 重复配置。入站本地 Call 的最大等待由目标 Runtime 控制。

已确认默认规则：Address 为空时报配置错误；MaxConnections 为零时使用 128，MaxConcurrentCalls 为零时使用 32，两个容量字段为负数时报错，为正数时使用指定值；ResolveService 可为 nil。默认最多 4096 个入站 Call 等待任务，按需创建，并非总 goroutine 数或内存上限，也不是压测得出的最佳容量。

Skynet gate 有连接数上限，默认 maxclient 为 1024；它依赖 Lua coroutine 等待处理。GSR 的入站 Call 使用连接 owner 管理的 Go 任务，因此同时限制连接数和每连接 Call 数，不直接照搬 Skynet 的容量作为 Go 的默认值。

## 已确认：注册与发现属于外部能力

保留 `Open(target, ListenConfig)` 与显式 Address，不新增按 NodeID 打开的公共重载。Open 只开放监听和入站接纳，不自动注册、续约、发布地址或参与集群成员管理。Core 与 Cluster 不判断业务资源的唯一所有权、一致性或过期。

外层 Registry/Discovery 维护 `NodeID → 可访问地址`，应用决定何时发布节点及服务。Cluster 使用目标路由管理投递与连接恢复；静态 `ClusterConfig.Nodes` 是当前已实现的地址来源，不是节点存活或业务就绪证明。动态发现结果如何更新 Cluster 路由仍待后续切片，本轮不预建通用发现接口，也不宣称已有运行期路由更新能力。

静态部署可以共用一份节点说明表，由应用按名字取配置，再填入 ListenConfig.Address。可信 IPv4 内网若希望只取端口监听 `0.0.0.0`，也由应用显式构造该地址，不让 Open 隐式忽略配置中的 host。此前讨论的“Open 按节点名查表并固定绑定所有网卡”不作为公共 API 契约。

监听地址与发布地址用途不同：例如本机监听 `0.0.0.0:7000`，外部发布 `battle-17 → 10.0.0.37:7000`。Open 返回的是实际绑定地址，不保证它是其它节点可访问的地址，也不自动把通配地址转换为发布地址。当前无需为此在 Cluster 增加 LocalNode 或发布地址配置。

## 收益与代价

应用不必自行组装 listener、连接任务、适配器和资源等待；Cluster 继续是网络资源的统一 owner。代价是 Cluster 内部要覆盖 Open 与停止的竞争、监听失败回滚和入站诊断生产者收尾，不能简单把一个监听调用塞进现有构造函数。

独立 Listener 对象便于未来多监听管理，但会让当前应用同时持有两个关闭 owner；本轮不推荐为尚未需要的能力扩大日常组装接口。

## 后续逐项收口

重复调用规则下一步再定。停止入口必须同时满足已确认的 Host 顺序和入站保留回复规则；现有 Cluster.Quiesce 不做网络 I/O，而内部 tcpListener.quiesce 会关闭监听，接线时必须明确处理这一区别，不能静默改变公开契约。

入站诊断目前有独立内部队列，合入公开诊断出口时需要明确字段及关闭归属，不增加诊断转发 goroutine。认证仍是后续能力；Open 本身不构成可信网络边界，未认证监听不应暴露给不可信网络。
