# Cluster 出站调用、监听与节点身份

状态：已获用户确认。撤回“固定交换 NodeID”及“保留可选来源 NodeID”的中间方案，采用目标路由与入站连接分离。

## 参考实现与取舍

Skynet `lualib/skynet/cluster.lua` 的 `cluster.call/send` 按目标节点取得 sender，不要求调用方先 `cluster.open`。`service/clusterd.lua` 的 listen 根据节点名配置或直接提供的端口开启 gate；`service/clusteragent.lua` 按 socket 接收请求，沿原连接回复。内部 `cluster.nodename()` 生成 hostname + PID，用于 trace 标记，不是必须注册的可寻址节点身份。

Lua coroutine 负责挂起与恢复本地调用，但“主动调用无需监听、回复沿原连接返回”不依赖 Lua，Go 可直接沿用连接组织方式。旧 GSR 的 NodeID/version 握手不是新 GSR 的兼容要求。

## 已确认契约

1. 仅出站的进程可以没有本地 NodeID，也不需要 open 监听；只要能解析目标 NodeID，就能 Send/Call。
2. open 表示接受新的入站连接；节点地址配置表示调用方如何找到目标。open 不负责赋予或认证节点身份，配置节点名也不等于已开启监听。
3. 不强制交换 NodeID，不新增 Version 或其它业务 Envelope 字段。接收端不要求知道调用方的逻辑 NodeID。
4. Call 回复沿原连接返回；发起端按自己记录的目标 NodeID、Session、Target 和 Command 完成等待。回复不是一次反向的新业务调用，不需要解析调用方地址。
5. B 若要主动向 A 发起新的业务调用，A 才需要开放监听，并让 B 可以解析 A 的地址。B 使用自己的 outbound sender 建立另一条到 A 的连接；两个方向独立复用连接，不是每次调用创建 socket。
6. 删除 `NodeSenderConfig.LocalNode`、`DeliveryFailure.LocalNode`、入站 `from NodeID` 与入站 owner 的 node 字段。目标 NodeID、注册接收器的 NodeID，以及响应 completion 所匹配的远端目标仍必须非空；completion 的 remoteNode 是本地出站目标，不是来源声明。
7. 入站诊断归属于连接 owner；生产 Transport 可附带连接标识和远端地址，不能将 IP/临时端口当成 NodeID。每条入站连接分别管理，不以节点名合并连接。Core Source 仍为零，业务调用者的本地 Service Source 绑定规则不变。
8. 认证与可寻址 NodeID 分开；可信内网可不启用认证。后续认证方案不得把调用方注册节点或 open 作为前提；具体认证协议另行裁决，本轮不实现 token 或握手。无认证的连接不提供对端身份保证，非可信网络需要外层安全传输和访问控制。

代价是接收端默认不能按逻辑来源 NodeID 归类匿名请求，也不能靠 NodeID 交换发现地址配置错误；收益是无需为只主动调用的进程配置多余身份或监听端口。

## 实施与验证边界

- 删除来源参数及相关存储，保持目标身份校验。主动 Send/Call 是实例自身能力，open 只决定是否接受新的外部调用，不限制原连接回复。
- `Transport` 只要求 `Send(to, envelope)`；内存替身的 Register/Unregister 与具体 Transport 的 Close 不属于主动发送接口，生命周期由外层管理。
- 内存 Transport 按已注册目标投递请求；测试连接的 writer 直接绑定调用方 completion，不注册调用方、不通过反向 Send 模拟回复。
- 真实 TCP 门禁使用匿名出站客户端：只有 B 监听，A 不注册节点、不交换 NodeID，验证 Send/Call、领域错误、慢 Call 隔离和断线完成等待。
- 静态节点表、公开 Open、连接数限制、连接诊断和按需重连均已实现；动态发现和生产认证尚未实现。历史身份方案不作为当前组装步骤。
