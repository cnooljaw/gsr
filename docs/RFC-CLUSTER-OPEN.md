# Cluster 可选入站组装

状态：待用户确认；本轮只建议资源归属，不定稿公开签名或配置默认值。

## 当前依据

内部 tcpListener 已实现停止接纳与最终取消分离，入站请求通过 localRequestAdapter 落地到显式传入的本地 Runtime。公开 Cluster 当前仅管理出站资源；不能把内部能力已完成等同于公开 Open 已完成。

只读参考本地 Skynet：

- `lualib/skynet/cluster.lua` 的 cluster.open 单独调用 clusterd 的 listen，主动 Send/Call 不依赖它。
- `service/clusterd.lua` 的 command.listen 创建 gate Service，再调用 gate.open；数字端口路径返回实际监听地址与端口。
- `lualib/snax/gateserver.lua` 通过 C socketdriver 监听，Lua coroutine 等待监听结果；一个 gate 保存一个监听 socket，客户端连接另行管理。clusterd 每次 listen 可以新建 gate，因此不是全局只允许一个 listener。

GSR 沿用“显式开放入站、框架负责接线”，不照搬 Lua 模块单例和新建 gate Service 的管理方式。Go 的监听和连接任务必须由明确 owner 取消并等待。

## 本轮建议：归现有 Cluster 管理

建议以 Cluster.Open 作为可选组装动作，第一版一个 Cluster 最多持有一个 listener，不返回需要应用单独关闭的第二个资源 owner。

- NewCluster 和仅出站 Client 继续不要求本地 Runtime、NodeID 或监听。
- 只有调用 Open 时显式提供监听配置与目标 Runtime；名称解析仍属于外层能力。
- Cluster 管理 listener、已接纳连接及其任务，但不负责启动、停止或关闭 Runtime。
- 第一版不支持多个 listener、运行中换绑 Runtime 或监听热切换；多 listener 并非 Skynet 限制，而是 GSR 当前没有对应需求时的范围约束。

例如战斗节点只主动调用大厅，可以不 Open；大厅要接收调用才 Open，并把请求落到大厅自己的 Runtime。两者均不需要通过本地 NodeID 表明身份。

## 收益与代价

应用不必自行组装 listener、连接任务、适配器和资源等待；Cluster 继续是网络资源的统一 owner。代价是 Cluster 内部要覆盖 Open 与停止的竞争、监听失败回滚和入站诊断生产者收尾，不能简单把一个监听调用塞进现有构造函数。

独立 Listener 对象便于未来多监听管理，但会让当前应用同时持有两个关闭 owner；本轮不推荐为尚未需要的能力扩大日常组装接口。

## 确认后再逐项收口

公开 Open 的完整签名、实际监听地址如何返回、重复调用规则及配置默认值，下一步再定。停止入口必须同时满足已确认的 Host 顺序和入站保留回复规则；现有 Cluster.Quiesce 不做网络 I/O，而内部 tcpListener.quiesce 会关闭监听，接线时必须明确处理这一区别，不能静默改变公开契约。

入站诊断目前有独立内部队列，合入公开诊断出口时需要明确字段及关闭归属，不增加诊断转发 goroutine。认证仍是后续能力；Open 本身不构成可信网络边界，未认证监听不应暴露给不可信网络。
