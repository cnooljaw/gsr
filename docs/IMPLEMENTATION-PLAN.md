# GSR Core 与 Cluster 实施路线图

本文把已接受的 API 和 ADR 拆成可验证的纵向切片。每个切片只引入完成当前运行链所需的能力；Core 完成后再开始 Cluster。

Core v0.1 是第一条公开兼容基线。此前各切片只在能力真正实现时把对应方法加入代码接口：Core 1 提供 Self/Send，Core 2 加入 After/CancelTimer，Core 3 加入 Call；不得为了让接口提前看似完整而加入固定返回“未实现”的占位方法。每次接口增长仍须同步文档和测试。

## 总体顺序

```text
Phase 0  契约闭合
  ↓
Core 0   基础类型与 Runtime 骨架
  ↓
Core 1   本地 Send 与 Service 生命周期
  ↓
Core 2   Timer
  ↓
Core 3   Call、PendingCall 与自动 Reply
  ↓
Core 4   故障、观测、并发与规模收尾
  ↓
Core v0.1
  ↓
Cluster 0–4
```

## Phase 0：契约闭合

状态：已完成当前实现所需裁决，后续发现公开契约缺口时先更新文档和 ADR。

已经冻结：

- Core 只使用 Runtime 局部 `ServiceID`，不使用 `ServiceRef`。
- Core admission 复制 `Args` 外层切片；引用型参数由 typed client 按消息类型构造快照。
- Handler 使用只读 `CommandContext.Source()` 获取 Runtime 填入的当前本地来源。
- Handler 通过返回值自动 Reply，不公开 `Reply()` 或异步 response API。
- Call 使用 run-to-completion，并受 Runtime `MaxCallWait` 和调用方 context 中更早的 deadline 限制。
- 第一版每个 Running Service 使用一个 Runtime-owned runner goroutine。
- OnStart 的 self Send 返回 `ErrServiceStarting`，self Call 返回 `ErrCallCycle`；启动期 Timer 最早在 Running 后 admission。
- Go 不能强杀 Handler 或 OnStop；超时任务从业务寻址中隔离，但必须追踪到真实返回。
- Cluster、NodeID、ClusterEnvelope、连接和编码不进入 Core。

## Core 0：基础类型与 Runtime 骨架

实现：

- `ServiceID`、`SessionID`、`TimerID`、`CommandID`。
- 可注入临界状态的 uint64 单调 ID 分配器。
- `ErrIDExhausted`。
- `ErrorClass`、`ErrorCode`、`ErrorStatus`、`StatusError`。
- Runtime Config 默认值与完整校验，包括有限且大于零的 `MaxCallWait`、Mailbox 容量和 shutdown deadline。
- Runtime 的 Created、Running、Stopping、Stopped 状态骨架。
- 空 Runtime 的 `Start`、幂等 `Close` 和并发等待结果。

验收：

- 所有 ID 从 1 开始，零值保留。
- 从 `math.MaxUint64-1` 直接验证最后一个 ID 和永久耗尽，不执行长循环。
- `StatusError` 按 Class 和 Code 支持 `errors.Is`，Message 不参与判断。
- 重复 Start 返回稳定生命周期错误。
- 并发 Close 只执行一次，等待者可以因自己的 context 提前返回，但不取消已经开始的关闭流程。

非目标：Service、goroutine、Mailbox、Timer 和 Cluster。

## Core 1：本地 Send 与 Service 生命周期

实现第一条完整运行链：

```text
Start
  → CreateService
  → Allocate ServiceID
  → OnStart
  → Publish Running Service
  → Start runner
  → Send admission
  → Mailbox
  → Handler
  → Stop
  → OnStop
```

实现：

- `Service`、`ServiceContext`、`CommandContext`。
- Service registry 和不可复用 ServiceID。
- 有界 Mailbox 和非无限阻塞 admission。
- per-Service runner；Scheduler 负责 runner 创建、panic recovery、停止、等待和观测，不引入全局 ReadyQueue 或固定 worker pool。
- 唯一的本地 Message 和 LocalDispatcher。
- `Self`、`Send`、`CreateService`、`Stop`、Runtime `Close`。
- Starting、Running、Stopping、Stopped 的接受边界。
- 排队 Send 丢弃与结构化诊断。
- Handler 和 OnStop 超时后的残留 runner 追踪。

验收：

- Service A Send 到 B，B 的 `CommandContext.Source()` 等于 A，业务不能覆盖 Source。
- 同一 Service Handler 永不并发；不同 Service 可以并行。
- Mailbox 按成功 admission 顺序 FIFO；多个并发发送者不承诺调用时刻的全局顺序。
- Mailbox 满时快速返回 `ErrMailboxFull`，不扩容、不无限等待。
- Runtime 只复制 `Args` 外层切片；typed client 的引用型 request 快照在各 client 自己的测试中验证。
- OnStart 失败的 ID 不发布、不复用，并清理启动期 Timer 等已取得的 Runtime 资源。
- OnStart 的 self Send、self Call 和 After 遵守 ADR 0007。
- Stop 与 Send 竞争只有一个 admission 结果：进入 Stopping 前成功入队，或进入 Stopping 后稳定拒绝。
- Stop/Close 幂等，OnStop 最多执行一次；超时 Handler 与 OnStop 不并发。
- Handler panic 只停止当前 Service，panic 和 stack 只进入本地诊断。
- Service 重建获得新 ID，旧 ID 永远不能命中新 Service。

非目标：Call、PendingCall、Timer 触发、Cluster、Supervisor 和自动恢复。

## Core 2：Timer

实现：

- Runtime 内部 Clock seam 和 FakeClock。
- 一次性 `After` 与 `CancelTimer`。
- TimerID owner 校验和单调耗尽。
- Timer 到期后构造普通 Message，使用与 Send 相同的 Mailbox admission。
- Service Stop、OnStart 失败和 Runtime Close 的 Timer 清理。
- Timer admission 失败的结构化诊断和基础指标。

验收：

- After 最多触发一次，只通过 Command 影响 Service 状态。
- 只有创建 Timer 的 Service 可以取消；未知、已取消或已触发 Timer 返回 false。
- CancelTimer、Stop 和触发竞争最多产生一次成功 admission。
- 同时到期 Timer 不承诺彼此顺序，也没有高于普通 Message 的 admission 优先级。
- Mailbox 满、目标 Stopping 和 Runtime Closing 时不重试、不静默丢失诊断。
- 测试使用 FakeClock，不依赖真实 sleep。

非目标：Ticker、周期任务、时间轮和业务 Timeline。

## Core 3：Call、PendingCall 与自动 Reply

实现：

- SessionID 分配和 PendingCall registry。
- `ServiceContext.Call(context.Context, ...)`。
- Send 和 Call 共用 Message、Dispatcher、Mailbox 和 Handler 路径，只由 SessionID 是否为零区分。
- Handler 返回值或 error 的一次 completion。
- 调用方 context 与 Runtime `MaxCallWait` 的 deadline 合并。
- self Call 拒绝。
- caller Stop、target Stop、Runtime Close、timeout 和 admission 失败的 PendingCall 清理。
- Domain/Runtime ErrorStatus；未知 Handler error 和 panic 的安全转换。

验收：

- Reply 不进入调用方 Mailbox，只通过 PendingCall 恢复原 Handler。
- Call 等待期间当前 Service 不处理下一条 Message，其它 Service 继续运行。
- 调用方没有 deadline 或给出更晚 deadline 时仍受 `MaxCallWait` 限制；更早 deadline 和 cancellation 生效。
- self Call 返回 `ErrCallCycle`；跨 Service 环形 Call 依靠有限 timeout 退出。
- admission 失败、timeout、双方 Stop 和 Runtime Close 都恰好清理一次 PendingCall。
- 未知、重复和迟到 completion 不会完成其它 Session，并产生受控诊断。
- Send 的 Handler error 只进入诊断；Call 的 Handler error 返回稳定 ErrorStatus。
- 未分类 error 转为 `runtime.handler_failed`，panic 转为 `runtime.service_panicked`；原始文本和 stack 不进入 Reply。

非目标：手工 Reply、异步 Reply、无限 Call、全局调用环检测和自动 retry。

## Core 4：故障、观测与规模收尾

实现唯一只读 Inspection，至少包含：

- Runtime 状态。
- Service 状态和 Mailbox 深度。
- runner 是否运行、是否超过 shutdown deadline、真实完成状态。
- PendingCall 和 Timer 数量。
- admission 拒绝、Handler error、panic、Call timeout、Timer 投递失败和 Stop timeout。

建立基准：

- 1、1K、10K 和目标规模空闲 Service 的内存、goroutine 与创建/停止耗时。
- Send 吞吐、Mailbox 等待和热点 Service。
- Call 延迟、timeout 和 PendingCall 压力。
- Timer 创建、取消、触发和清理。
- 大规模 Runtime Close 收敛。

Core v0.1 门禁：

- Core 公开 API、CONTEXT 和 ADR 一致。
- `go test ./...`、`go vet ./...` 和 `go test -race ./...` 通过。
- Stop 后不存在未登记 runner、Timer 或 PendingCall；故意阻塞的业务任务必须作为已知残留被 Inspection 准确报告。
- per-Service runner 的规模数据已记录。只有数据证明成本不可接受时，才新增 ADR 讨论固定 worker pool 或 continuation。

## Cluster 后续路线

Cluster 是 Core 的调用者和适配层，不进入 Core v0.1。

### Cluster 0：协议与内存传输

- `NodeID`、`ClusterSessionID`、`ClusterEnvelope`。
- 显式 Request、Response、Error frame。
- ErrorStatus wire 表达。
- 默认值 codec、Command codec registry 和启动期重复注册 panic。
- 帧长、容器深度和 payload 上限。
- InMemoryTransport 连接两个 Runtime，本地路径不执行 Cluster 编码。

### Cluster 1：远程 Send

- 每个远端 Node 唯一 outbound `NodeSender`。
- 有界出站队列和并发建连合并。
- 初始化时绑定 Source ServiceID 的 per-Service ClusterClient。
- 无路由、已知断连、编码失败、队列满和关闭错误。
- Send 返回 nil 只表示本地 Cluster admission 成功。

### Cluster 2：远程 Call

- Cluster PendingCall 和 ClusterSessionID。
- Response/Error frame completion。
- 断线、timeout、未知、重复和迟到 response。
- Domain Error 经 Cluster 往返后保持 Class/Code 和 `errors.Is` 语义。

### Cluster 3：TCP 与连接 owner

- 长度前缀 framing。
- 每条 inbound socket 一个 ConnectionAgent，响应沿原连接返回。
- 可选 NodeID/shared-token 握手。
- NodeSender 自动重连，但不自动重放已经失败或结果不确定的请求。

### Cluster 4：故障与资源收尾

- FaultTransport 注入半写入、断线、重复响应、损坏帧和出站队列满。
- 边界可信的未知版本、未知 Command 和解码失败帧丢弃并限频诊断。
- 非法长度、越界、认证失败或无法恢复帧边界时关闭连接。
- 双节点集成、race、关闭和连接泄漏门禁。

## 旧参考项目的使用方式

`../GameServiceRuntime` 与 `../skynet` 始终只读。

从旧 GameServiceRuntime 吸收问题和黑盒测试场景：

- Send/Stop admission 竞争。
- 并发 Stop/Close 和 OnStart/Runtime Close 竞争。
- PendingCall 的 admission 失败、timeout、重复、迟到和停止清理。
- Timer 与目标停止竞争。
- panic、内部 error 脱敏和任务真实返回。
- Registry、Mailbox、Timer、PendingCall、指标标签和 goroutine 泄漏。

不带入旧实现形状：

- `ServiceRef`、Node 嵌入 Core 地址和兼容别名。
- 固定 worker、ReadyQueue 和 permit 调度结构。
- 手工 `Reply()`、异步 Reply 和旧 Service 四钩子接口。
- `Payload` 兼容字段、可组合 Cluster codec 和公开远端解析。
- Cluster、Discovery、Snapshot、Supervisor、Runner 或业务类型进入 Core。

从 Skynet 优先参考其实际 API 和运行模型：

- `self/send/call/timeout/newservice/dispatch` 的简单动词和数字地址。
- source、session、每 Service mailbox、消息调度、Timer 投递消息和 Cluster sender/agent 方向性组织。
- 对依赖 Lua coroutine、PTYPE、C 引用计数、模块加载和历史 Harbor 的机制，只提炼目的，不逐字移植。

## 测试夹具

- FakeClock：确定性推进 Timer 和 Call timeout，不使用真实 sleep。
- ProbeService：记录 Source、执行顺序和同时运行的 Handler 数。
- BlockingService：在 OnStart、Handler、Call 和 OnStop 阶段受测试控制地阻塞和恢复。
- InMemoryTransport：单进程连接两个 Runtime，不依赖 TCP。
- FaultTransport：注入网络边界故障。
- Leak/Race harness：核对 runner、Timer、PendingCall、连接和 goroutine owner。

## 第一轮任务

1. 初始化 Go module 和最小目录结构。
2. 先为 ID 分配器、StatusError、Config 和 Runtime 空生命周期写失败测试。
3. 完成 Core 0 的最小实现。
4. 运行受影响 package 的格式、编译、测试和 vet。
5. Core 0 收尾后运行全仓 test/vet；其代码不包含并发 owner 时不机械运行 race。
6. 以中文提交一份可验证切片，例如“实现 Core 基础类型与 Runtime 生命周期骨架”。
7. 下一提交再进入 Core 1，不在 Core 0 预建 Mailbox、runner、Timer 或 Cluster 抽象。

## 通用阶段门禁

- 新行为先写可失败测试，再完成最小实现。
- 开发循环运行相关测试；package 收尾运行 package 全测和 vet。
- 功能切片收尾运行全仓 test/vet；涉及 goroutine、生命周期、锁、channel、Timer、Scheduler、Transport 或共享状态时运行 race。
- 测试默认使用 fake、probe 和受控阻塞点，不用反复 sleep 掩盖竞态。
- 测试保护公开行为、owner 和失败语义，不依赖无必要的私有字段或 goroutine 数量细节。

## 暂不进入第一版

Discovery、服务迁移、自动业务 retry、Snapshot、监督树、动态负载均衡、多 codec 协商和 durable delivery。
