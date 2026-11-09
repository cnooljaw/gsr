# ADR 0013：客户端协议选择与 Gate 安全模式

## 状态

Accepted，2026-10-14 用户确认双协议方案并要求调整 Gate 模式命名。边界裁决已接受；安全模式命名、独立 Sproto Dispatcher 和 Go/TypeScript Echo 随 GSR v1.0.2 发布。

## 依据

只读参考 Skynet `examples/agent.lua`、`examples/proto.lua` 和 `lualib/sproto.lua`：Agent 使用 C2S/S2C schema，host:dispatch 解释 Sproto package 的 type/session，关联原请求生成响应；C 层负责编解码和 pack/unpack，Lua host 维护会话表。消息方向由所加载的协议集合限定。此模型可以沿用；Lua 动态调用、skynet.fork 和 TCP 的两字节长度头不照搬到 GSR 的 Go Service 与 WebSocket。

当前 GSR Gate/Runtime 已只接纳不透明字节，Protobuf Dispatcher、目录及 Envelope 位于 `protocol`，Agent 为组合点。Gate 的 ProtocolMAC 同时控制握手及后续 seq/MAC，名称容易与业务协议混淆。

## 裁决

- Gate 类型改名为 SecurityMode，常量为 SecurityModeMAC/SecurityModePlain，`gate.Config` 与 `gsr.GateConfig` 字段为 SecurityMode；默认值、TLS、握手、MAC/seq、Plain 绑定限制与资源生命周期不变，不保留旧名别名。
- `NewAgent` 决定具体业务协议，Gate 不增加业务协议枚举、探测或协商。ClientSession 保留 Send/Close；协议 Dispatcher 只依赖 Send，关闭连接仍由 Agent/Gate 负责。
- 保留现有 `protocol` 作为 Protobuf 实现，新增独立 `protocol/sproto`；两种实现各自保留协议定义、上下文和分发，不互相依赖。不为了目录对称迁移现有 Protobuf 导入路径。
- Envelope、Registry、MessageRoute、Direction、Kind、request_id 与通用 Protobuf ErrorResponse 均属于 Protobuf 实现，不提升为公共协议模型。
- Sproto 使用自身的 Request/Response、package(type/session) 和 pack/unpack，C2S/S2C 集合限定方向；不重复套 MessageRoute 或 Envelope。仍须构造时检查协议与强类型 Handler 的绑定。
- 每连接 Agent 独占协议会话状态，定义可以在只读条件下共享；Sproto session 与 Core/Cluster Session 分离。首个切片覆盖客户端 Request/Response、Notify 和服务端 Push，不增加服务端等待客户端回应的机制。
- 业务失败在对应 Sproto Response 中表达；未知协议、坏包和框架失败需要明确诊断与客户端结束等待的行为，不直接借用 Protobuf ErrorResponse。具体错误 schema 在实现切片中记录并验证。
- JSON 等协议可通过同一字节边界接入；本轮不实现，也不增加 ProtocolCodec、公共 Message/Route/SchemaProvider 或统一 Envelope。

## 代价与验证

安全模式改名影响调用方源码，但不改变 wire；旧版文档保留在 v1.0.1 标签。双协议会各自维护上下文与分发，这是两种协议独立模型的成本。标准库没有 Sproto 编解码器；第三方实现与生成工具在纳入生产依赖前需验证类型、坏包、资源上限和跨语言互通，不能仅因字符串 Echo 成功就宣称全面兼容。

接入方式与当前支持边界见 [客户端协议指南](../guides/CLIENT-PROTOCOLS.md)。

## 编解码库职责

维护独立 `github.com/cnooljaw/gosproto`，保留上游历史与许可证。库负责官方兼容的纯 Go 编解码、Packing、原生 RPC 和有限资源预算，不提供连接调度、Service 网络 helper 或业务 Handler 绑定；GSR 正常 require 发布版本，不使用本地 replace。`protocol/sproto` 负责 Handler 接线、Response/Push；Agent 决定错误关闭策略。

官方 C/Lua 作为兼容对照。固定 C 版本的部分 64 位解码存在 todword 符号扩展缺陷，边界验证采用独立编码输出和 Go 编解码核对，不复制已知错误。具体向量与性能研究归独立库维护。
