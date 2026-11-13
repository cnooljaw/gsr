# GSR 术语

本文件统一术语；行为细节见 [API 契约](docs/API-DESIGN.md)，能力版本见 [文档索引](docs/README.md)。

## 运行与组装

- **Runtime**：本地 Service 的运行环境，负责接纳、调度、消息、Timer、自动回复、生命周期和基础观测；不拥有 Cluster 或业务资源一致性。
- **Service**：独占自身可变状态的运行单元。不同 Service 不持有彼此实现指针，只通过 Command 请求状态变化。
- **ServiceID**：Runtime 局部的临时 uint64 地址，零值保留，同一 Runtime 内不复用，不持久化或跨重启使用；不含 NodeID、Epoch 或 Generation。
- **MainService**：应用定义的普通入口 Service，负责组装和发起业务；没有 Runtime 特权，自身退出不关闭节点。
- **Node**：Run 创建并管理的节点对象。同 Run 共享、不同 Run 隔离；通过 `self.Node()` 借用名称、服务池、Cluster/HTTP/Gate 组装能力，不提供业务通信或关闭入口。
- **应用配置**：`gsr.RuntimeConfig` / `gsr.ClusterConfig` 不暴露 Logger，Run 转换到底层配置并统一注入依赖。
- **Service 组装**：`ServiceContext.CreateService` 接受已构造实例并交给 Runtime 管理，不随创建者退出而停止；服务池另用显式工厂创建独立成员。
- **Service 池**：一个节点内名称对应多个独立 ServiceID 的可选有界并行能力，自 v1.0.3 提供。成员各自串行，轮询只选一次，不重放、不保证整体顺序、不自动补员；见 [指南](docs/guides/SERVICE-POOLS.md)。

## 本地消息与状态

- **Command / CommandID**：目标 Service 解释的命令与 uint32 编号，不同 Service 可复用编号；编号不区分本地与远程。
- **Message**：Runtime 内未编码的本地消息，承载 Source、Target、Session、Command 与 Args；不复用为网络封包。
- **Args 所有权**：引用型参数必须是消息独占快照；成功接纳后发送方不再修改，不发送 Service 内部状态引用。
- **Dispatcher**：Service 用具名字段持有的不可变强类型命令分发器；构造时校验 Handler，Service 显式实现 Handle 并转发。运行时分派不另建 Service 或 Mailbox。
- **Mailbox**：每 Service 的有界等待队列，容量不含当前 Handler；满载立即拒绝，不无限扩容。
- **Scheduler / runner**：Runtime 为每个 Running Service 管理一个 runner goroutine，串行消费 Mailbox；取消、等待和 recovery 属于框架。
- **Send / Admission**：发送与本地接纳。Send 返回 nil 只说明当前边界接受，不保证送达、执行或执行一次。
- **Call / SessionID / PendingCall**：有限等待目标结果的调用、本地 uint64 关联号及其完成记录。零 Session 表示 Send，非零表示 Call；当前 Service 等待时不处理下一条消息。
- **Reply**：Handler return 后 Runtime 自动完成原 PendingCall，不作为业务 Command 进入调用方 Mailbox；业务不手工或异步 Reply。
- **Source**：当前本地发起 ServiceID，由 Runtime 自动绑定。Timer、HTTP/Cluster 等基础设施注入为零，不是玩家或跨节点身份凭据。
- **ServiceContext / CommandContext**：当前回调内的 Service 能力；CommandContext 另外提供收到消息的 Source。不保存、转借或用于异步任务，新调用仍以 Self 为来源。
- **Redirect / Relay**：受信任基础设施转发 Runtime-owned Message 的能力，只考虑单向转发，不让业务指定任意 Source 或转移 Call Session。
- **Timer / TimerID / owner**：一次性 After 登记及 uint64 ID；owner 决定取消权限，触发仅投递 Command，Source 固定为零，不用回调直接改状态。
- **Revision**：领域状态版本，用来拒绝迟到结果；不同于 ServiceID，不属于 Core 身份。
- **业务语义**：事务、顺序、幂等、补偿、重试、fencing 和资源过期判断由业务/外层模块拥有，不由 Runtime/Cluster 推断。

## 生命周期与错误

- **RuntimeState**：Created → Starting → Running → Stopping → Stopped → Closed；Stopped 不可重启，Closed 表示 Runtime-owned 资源已安全释放。
- **Lifecycle Coordinator**：按固定、显式计划编排 Core 组件，不是动态业务插件容器；组件自己拥有资源。
- **OnStart / Start Command**：前者是生命周期初始化，失败只回滚当前实例并至多调用一次 OnStop，ID 作废不复用；后者是可选业务协议，非所有 Service 必需。
- **Exit**：当前 Service 请求回调返回后退出；幂等，不中断 Go 控制流，当前 Call 仍先按返回值完成。
- **Stop / Close**：`Runtime.Stop(ctx)` 关闭接纳并停止 Core 组件；`Runtime.Close()` 完成最终释放，使用内部 ShutdownTimeout。两者幂等，不允许重新 Start。
- **ShutdownTimeout**：Core 共享的关闭预算，不能强杀不返回的 Handler/钩子；超时残留任务仍须可观测，OnStop 不与未结束 Handler 并发执行。
- **退出后的回收**：OnStop 返回后释放业务实例和上下文，runner 退出后解除 Scheduler 跟踪；关闭 ID 的状态/结果元数据保留到 Runtime 停止收尾，元数据不是有界历史。
- **ErrorClass / ErrorCode / StatusError**：Domain/Runtime 类别、稳定字符串代码与安全 Message 的错误值。程序依赖 Class/Code 或 errors.Is，不比较文本；原始错误与 panic stack 留在本地。
- **RuntimeInspection / ServiceInspection**：Runtime.Inspect 的只读值快照，包含状态、队列、runner、Timer/PendingCall 与计数；不暴露内部对象，快照不承诺跨组件原子一致。
- **Runtime Handler 诊断**：本地错误/真实 panic 的关联字段与文本快照，不持有业务对象。固定 256 条非阻塞流，Message 最多 4 KiB、Stack 最多 16 KiB，仅在 Closed 时关闭；详见 [诊断设计](docs/design/RUNTIME-DIAGNOSTICS.md)。

## Cluster

- **NodeID / ServiceName**：外层目标节点名与节点内服务名。NodeID 用于静态地址解析；普通名称归目标 ServiceID，池名称归成员组，均不进入 Core 地址。
- **Service 绑定的 Cluster 入口 / ClusterClient**：`self.Cluster()` 绑定当前 Self，内部 client 借用节点唯一 Cluster 的连接、队列和等待表；无单独关闭责任，业务不创建 client。
- **ClusterConfig / ListenConfig**：出站静态路由、编码与有限预算；可选入站地址、连接/Call 上限和名称解析。Open 只监听，不注册 Discovery 或认证身份。
- **ClusterEnvelope / FrameKind / ServiceTarget**：网络请求、响应或错误载体，方向显式；目标必须且只能选远端临时 ServiceID 或 ServiceName，不携带 Core Source 或 wire version。
- **ClusterSessionID / ClusterCallRegistry**：Cluster 的 uint64 请求关联号与共享等待 owner；零为 Send。与本地 Session 分离，网络等待在写入前绑定连接，断线只完成该连接的等待。
- **PayloadCodec / ValueCodec**：全部命令共享的参数/结果编解码器，不是命令准入清单。默认支持有限值集合并规范化数值类型；容量分配前校验集合数量、深度与载荷。
- **FrameCodec**：四字节大端长度前缀的完整帧编解码，MaxFrame 含前缀；不负责 stream I/O 或生命周期。
- **NodeSender / outboundPeer**：每目标唯一有界发送队列及连接 owner。按需有限 Dial、复用与恢复，不自动重放已尝试请求；动态路由尚未实现。
- **入站连接 owner**：私有 inboundConnection 管单 socket 的读取、有界 Call、取消与等待，localRequestAdapter 投递 Runtime.Send/Call，回复沿原连接返回；不是业务 Service。
- **DeliveryFailure / Diagnostic**：接纳后投递失败及 Cluster 基础设施事件，不携带业务载荷，不是业务结果。共享出口容量 256，满载丢弃，全部生产者退出后关闭；见 [Cluster 设计](docs/design/CLUSTER.md)。
- **远程 Send 所有权**：A 负责本地接纳与传输诊断，B 负责 Handler 执行与错误，B 的业务失败不回到 A 的 Send 返回值。
- **Quiesce**：停止入站/出站新接纳、拨号并关闭监听，保留已接纳 Call 的原回复机会；不等同于 Close 的断连和等待释放。
- **InMemoryTransport**：同步、确定性的进程内测试替身，在接收边界复制 Envelope，不代表 TCP 或远端执行保证。

## Gate 与客户端协议

- **Gate / SecurityMode**：Gate 拥有连接、握手、安全封装、有界字节收发与关闭；MAC 为默认模式，Plain 显式用于可信回环/私网。安全模式不选择业务协议，TLS 独立配置。
- **Agent Service**：每连接一个普通业务 Service，组合 ClientSession 与具体协议 Dispatcher；客户端消息和普通 Command 共用该 Service 的串行 runner。Agent 只拥有连接私有轻量状态，权威房间/玩家状态归独立 owner。
- **ClientSession**：绑定单连接的 Send([]byte)/SendAndClose([]byte)/Close 能力，不暴露 net.Conn 或 Gate 内部表。接纳时复制字节快照，不保证送达。
- **Gate→Agent 投递**：Runtime.DeliverClientMessage 复制不透明字节后进入 Mailbox，Source/Session/CommandID 为零；目标须显式实现 ClientMessageHandler。消息类别不靠 Source 猜测。
- **Protocol**：Protobuf 位于 protocol，Sproto 位于 protocol/sproto，各自维护模型并只依赖字节发送窄接口；Gate 与 Protocol 互不依赖，Agent 为组合点。gosproto 是独立编解码权威库。
- **协议 Handler 上下文**：当前 Agent 的 Service 上下文与协议响应/推送上下文职责分开，只在当前同步 Handler 使用；不保存或异步调用。
- **C/S Envelope**：Protobuf 的 msg_id / request_id / payload 包装。目录由 schema 显式映射消息类型、方向与种类；Handler 表按实现绑定，不提升为其它协议的公共模型。
- **C/S request_id**：Protobuf uint32 关联号，零表示 Notify/Push，Request 使用非零并得到一次关联响应；单连接不回绕复用，耗尽重连，不用于幂等或防重放。
- **Sproto session**：客户端请求使用正 int32，响应沿用原 session，Notify/Push 无 session；每连接独立，首版不支持服务端等待客户端响应。坏包/未知协议/框架失败由 Agent 关闭连接，业务拒绝使用业务 Response。
- **C/S seq / 临时密钥**：MAC 模式每连接交换 X25519 公钥，以固定 HKDF 参数派生上行 macKey；uint64 seq 大端递增不回绕，认证成功即消费，不代表业务执行成功。
- **Gate 容量与关闭**：限制连接、握手时间、帧长和按字节计的出站队列，入站受 Agent Mailbox 约束；关闭使会话失效、关闭 socket、通知 Agent 并等待自身 I/O，不等待或强杀 Agent。
- **C/S 关闭码与日志**：标准 WebSocket 关闭码表达传输/安全结果，协议错误归 Agent、领域拒绝归业务。Gate 不记录密钥或原始载荷；具体映射见 [Gate 设计](docs/design/GATE-PROTOCOL.md)。

- **SendAndClose（v1.0.4）**：Gate 原子接纳最后字节帧、封闭新消息并按 FIFO 有界写出后关闭的能力；成功不是送达保证。协议错误策略选择是否使用它，业务拥有 closing 与断线清理。见 [ADR 0015](docs/adr/0015-send-and-close.md)。

## 日志

- **Logger / ServiceLogger**：Run 拥有共享 FIFO Logger，ServiceLogger 只绑定当前 Service 并提供 Info/Error，不暴露 Close 或重绑能力。
- **日志接纳与诊断**：日志记录处快照并非阻塞入队，由唯一 writer 输出；Runtime/Cluster 直接写共享 Logger，Diagnostics 是独立观测流，不能再次转发同一主日志。队列丢弃、输出失败与重复诊断抑制分别计数。
