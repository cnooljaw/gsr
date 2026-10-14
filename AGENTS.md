# AGENTS.md

本文件是 Agent 进入 `gsr` 项目后的工作入口，只记录必须遵守的规则和文档索引。长期设计结论写入 `docs/`，不要只保留在聊天记录中。

## 项目定位

- `gsr` 是一个重新设计和实现的 Go 游戏服务器 Service Runtime。
- 借鉴 Skynet 的 Service、消息、生命周期和简洁 API 思想，但使用 Go 的类型系统、显式错误和明确依赖边界。
- `../GameServiceRuntime` 与 `../skynet` 仅为只读参考目录，不是本项目的历史兼容目标，不在其中修改文件。
- 不因参考实现已有某个接口、命名或内部结构，就默认本项目必须兼容；是否采用由当前文档、测试和实际需求决定。

## 文档优先级

1. `AGENTS.md`：工作规则与文档入口。
2. `CONTEXT.md`：稳定术语及其准确含义。
3. `docs/API-DESIGN.md`：公开 API、Runtime 边界和 Skynet 风格原则。
4. `docs/adr/`：难以逆转的设计裁决及其取舍。
5. `docs/GO-CODE-STYLE.md`：Go 编码规则与评审标准。
6. 测试：已经落地的行为契约。
7. 实现：在文档未覆盖处提供当前行为证据，但不能替代设计裁决。

代码、测试和文档冲突时，不静默选择其中一份。先确认预期行为，更新作为契约的文档和测试，再修改实现。

## 工作原则

- 先完成最小、可验证的纵向切片，不提前建设尚无调用者的通用框架。
- Service 是运行单元，不是代码分类方式；需要独立运行时优先 Service，普通调用不额外包装，基础设施例外必须说明具体理由。见 [Service 化原则](docs/API-DESIGN.md#11-service-化原则)。
- 每个 Service 在自身类型上显式实现 `Handle`，让 Runtime → Service 的消息入口可见；这是项目编码规则，不能仅靠 Go 接口检查强制。不得靠匿名嵌入 Dispatcher 的方法提升隐式获得 `Handle`。
- 普通应用层 `gsr.Service` 默认在构造时用具名字段持有 `gsr.Dispatcher`，把已实现的 `CommandID` 绑定到强类型 Handler，并在 `Handle` 中转发；不使用 Dispatcher 的 Service 不必持有它。直接实现 Core `runtime.Service` 的基础设施和底层集成示例仍须显式声明 `Handle`，但不套用依赖 `gsr.CommandContext` 的应用层 Dispatcher。只有真实 benchmark/profile 确认个别命令的分发成本成为瓶颈时，才在 `Handle` 中校验参数并直接调用强类型 Handler，其余命令继续委托 Dispatcher。不引入 Fast Handler 注册机制、默认代码生成或额外分发层。见 [Dispatcher 契约](docs/API-DESIGN.md#44-业务-service-默认使用-dispatcher)。
- 应用由普通 MainService 发起业务：Service 内通过 ServiceContext.CreateService/Send/Call/SendName/CallName/Exit 和 Node.RegisterService 创建、命名、通信和自身退出；宿主管 Runtime。本地与远端共用节点内名称表，名称不进入 Core；Runtime.Send/Call 仅作基础设施或测试注入，不作为业务通信捷径；不新增 Launcher/ServiceManager，不强制业务 Start Command。
- 应用启动使用 gsr.Run；业务通过 self.Cluster() / self.Logger() 取得自身来源绑定能力，不手工注入 Host 或创建 ClusterClient。Core 的 runtime.ServiceContext 不反向依赖 Cluster；禁止用全局单例的“当前 Service”或 goroutine-local 推断来源。
- 节点组装使用 self.Node().RegisterService/OpenCluster/OpenHTTP/CreateGate，监听观测使用 ClusterListenAddress；self.Cluster() 仅通信，self.Logger() 只记录，不承担关闭或改绑。见 [ADR 0012](docs/adr/0012-node-capabilities-and-service-logging.md)。
- 修改范围必须能够追溯到当前任务；不顺手重构无关代码。
- 框架内部组件之间必须成立的不变量由构造器建立并保证，不要求业务调用方手工组装关联后再由运行时检查；业务仍负责输入、资源所有权和生命周期契约。
- Gate 负责有界、顺序、失败可观察地收发不透明字节消息，Protocol 负责消息编解码、校验和分发，业务 Agent 是二者的组合点。`gate/` 与 `protocol/` 同级且生产代码互不依赖；Gate 不导入 Protobuf、不解释 Envelope，连接能力只提供 `Send([]byte)` / `Close()`。Protocol 只依赖字节发送窄接口，不拥有连接。Send 成功只代表本地接纳，不保证送达、业务执行或执行一次；Gate 接纳时复制字节快照。详见 API 文档的“Gate 与 Protocol 边界”。
- 公开 API 或架构边界发生变化时，同步更新 `docs/API-DESIGN.md`。
- 新增或修改 Go 代码必须遵守 `docs/GO-CODE-STYLE.md`。
- 形成稳定的新设计结论时写入相关文档；较大的取舍可新增 ADR 或 RFC，不把 `AGENTS.md` 扩写成架构百科。
- 默认不为旧参考项目提供兼容层、迁移别名或废弃 API，除非用户明确提出兼容需求。

## 设计裁决流程

- 遇到会改变公开 API、运行语义、状态所有权、生命周期、Cluster 协议或长期兼容面的待裁决问题时，先阅读 `../skynet` 中对应实现，不只凭印象描述 Skynet。
- 汇报时依次说明：Skynet 的实际做法、它依赖的 Lua/C 运行机制、GSR/Go 中可直接沿用与不宜照搬的部分、推荐方案及代价。
- 推荐提出后由用户最终裁决；用户裁决前不得把推荐方案写成 Accepted ADR，也不得据此实施难以逆转的代码。
- 用户裁决后同步更新 API 文档、CONTEXT 和必要 ADR，再以测试锁定行为。
- 普通局部实现细节无需逐项上升为裁决；只有影响外部可观察契约或重要架构边界时才执行上述流程。

## Runtime 硬边界

- Core 只包含 `Runtime`、`Service`、`ServiceID`、`Command`、`Mailbox`、`Scheduler` 和 `Timer`；Cluster、Discovery、Snapshot、Diagnostic、AI Runner 建立在 Core 之上。
- Runtime 使用固定、显式的 Core 组件生命周期计划；不得把后续 Service、Scheduler、Timer 或其它组件的清理逻辑不断追加到单一 `closeFn`。
- 核心术语使用 `Runtime`、`Service`、`ServiceID`、`Command`、`Send`、`Call`、`Reply`、`Timer`；不要混用 `ServiceRef`、`Actor`、`Spawn`、`Ask`、`Tell` 或 Skynet `PTYPE`。
- `ServiceID` 固定为 Runtime 局部的 `uint64` 临时地址，不包含 Node、Epoch 或 Generation，不持久化或跨 Runtime 重启使用。
- 同一 Runtime 生命周期内已经发放的 ServiceID 不复用；Service 重建必须获得新 ID。
- ServiceID、SessionID、TimerID 和 ClusterSessionID 都使用 uint64 单调分配；零值保留，耗尽后返回 `ErrIDExhausted`，不得回绕、扫描或复用活跃/历史 ID。
- 每份可变业务状态必须有唯一 Service owner；外部只能通过 ServiceID 和 Command 请求状态变化。
- Service 之间不得持有彼此的实现指针，不得绕过 Mailbox 直接修改其他 Service 状态。
- 本地 `Send` 与 `Call` 使用同一个未编码 Message、Dispatcher、Mailbox 和 Scheduler 路径，只通过 `SessionID == 0` 或 `SessionID > 0` 区分是否等待 Reply。
- Reply 通过 Runtime 的 PendingCall completion 路径返回，不作为业务 Command 进入调用方 Mailbox；ClusterEnvelope 必须显式区分 request、response 和 error frame。
- Reply error 使用稳定的 ErrorClass、ErrorCode 和安全 Message；程序分支只依赖 Class/Code，不比较 Message。未知 Handler error 不透传内部文本或 stack。
- 普通 Send/Call 的 Source 必须由 Runtime 自动设置为当前发起 Service 的 ServiceID；Timer 投递使用 Source == 0；受控 Redirect 只能转发 Runtime-owned Message，业务代码不得传入或伪造任意 Source。
- 宿主或受信任基础设施通过 Runtime.Send/Runtime.Call 显式注入本地消息，Source 固定为零；Runtime.Call 复用有界 Mailbox、有限等待和 PendingCall，不创建 goroutine。业务 Service 只获得 ServiceContext，不能借用基础设施入口绕过自身来源与执行约束。
- Cluster 内部入口显式接收 source；普通业务 API 使用预先绑定当前 ServiceID 的 per-Service client，并调用 `Send(node, serviceID, ...)` / `Call(node, serviceID, ...)`；受信任 relay 只能转发 Runtime-owned Message。禁止用全局变量或 goroutine-local 推断当前 Service。
- 未编码的 Message 不得成为共享可变状态的通道：引用型 Args 必须是消息独占快照，成功 admission 后发送方不得修改，且不得发送 Service 状态的内部引用。
- Service 内 Call 采用 run-to-completion：等待期间同一 Service 不处理下一条消息，Reply 只恢复当前 Handler；Call 必须有有限超时并谨慎使用，self Call 直接拒绝。
- 第一版每个 Running Service 使用一个 Runtime-owned runner goroutine 串行消费 Mailbox；Handler 通过返回值由 Runtime 自动完成 Reply，业务不得手工或异步 Reply。
- Mailbox 与 Cluster 出站队列必须有界且 admission 非无限阻塞；满载返回稳定 Runtime Error，不自动无限扩容。
- Cluster Send 在 admission 前失败时立即返回错误；返回 nil 只表示本地 Cluster 边界接受或入队。admission 后的传输失败只进入 A 侧基础设施诊断，B 侧 Handler error 归 B，不通过 Send 返回 A；需要远端结果时使用 Call。
- ClusterEnvelope、NodeID、连接和编码不得进入 Core 本地 Message 或 ServiceID。
- Cluster 主动 Send/Call 不要求本地 NodeID 或 open；open 只负责接受新调用。出站只持有目标，入站只持有连接，不传递 LocalNode 或来源 NodeID；回复沿原连接返回，反向主动调用使用独立出站连接。
- Cluster 每个远端 Node 只有一个 outbound NodeSender；每条 inbound socket 由私有 inboundConnection 管理读循环与有界 Call 任务，经 localRequestAdapter 投递本地 Runtime，回复沿原连接返回。断线自动重连，但不自动重放业务请求。
- Cluster 统一编码，不维护节点级 Command 准入清单；CommandID 由目标 Service 解释，不同 Service 可复用编号。未知 Send 由目标丢弃，未知 Call 返回 ErrUnknownCommand。wire 输入必须设置帧长、集合深度和 payload 上限。
- 能安全确认帧边界的格式错误和解码失败帧丢弃并限频诊断；非法长度、越界、认证失败或无法恢复帧边界时关闭连接。合法载荷中的未知 Command 不是坏帧。
- Timer 只投递 Command，不通过回调直接修改业务状态。
- 第一版 Timer 只有一次性 After 和 CancelTimer；Timer 不拥有更高 admission 优先级。
- Service 和业务层不得直接启动 goroutine，也不得引入公共 `Runtime.Go` 或 `Fork` 作为逃生口。
- 只有调度器、Transport/连接适配器、固定上限的外部 worker 等明确并发 owner 可以启动 goroutine；owner 必须负责取消、关闭、等待、并发上限、panic 和错误归属。
- Core Runtime 统一提供调度、本地路由、生命周期、panic recovery 和基础观测机制；远端路由属于 Cluster。timeout、retry、幂等、补偿以及业务资源的所有权、一致性和过期判断由拥有操作语义的业务/外层模块决定，Runtime 只负责投递和连接恢复。

## Go 与验证

- 优先使用标准库；新增第三方依赖前说明它解决的具体问题和不采用标准库的理由。
- 新增行为先建立可失败的测试，再完成最小实现；纯文档修改不要求补测试。
- 补测试前先用 `rg` 查找已有的行为用例和验证层级；优先扩展最相关的现有测试。只有明确的未覆盖行为或跨层接线风险才新增用例，不为同一底层契约在 Core、Cluster、应用层重复搭建完整夹具；汇报新增用例相对旧用例的增量。
- 开发循环默认执行最小相关验证，不在每次小修改后重复运行全仓测试、`vet` 和 race 测试。
- 按变更范围选择验证：文档修改只做 diff/格式检查；局部代码或测试修改先运行对应测试，测试代码或实现形成一个连贯切片后再运行受影响 package 全测。只加一个独立测试时，不因惯例额外跑全包。
- `go test ./...` 和 `go vet ./...` 留给跨 package 功能切片完成、重要集成节点或发布前检查，不作为每次小改动的门禁。生产代码修改确有静态分析需要时，可先对受影响 package 执行 `go vet`。
- 涉及并发、生命周期或共享状态时，优先在切片收尾对受影响 package 运行 race 测试；只有改动影响 Runtime、Scheduler、Mailbox、Cluster 等跨 package 共享并发边界，或处于重要集成/发布验收时，才运行 `go test -race ./...`。普通文档、命名或纯业务规则修改不机械运行 race。
- 同一份代码已由更完整的验证覆盖后，不重复运行较窄的同类测试；只有排查 flaky、随机性或并发时序问题时才重复执行测试。
- 每个 `Test...` 测试函数前写一句注释，说明它保护的可观察行为；不要只重复函数名。
- Go 代码中的自然语言注释尽量使用中文，包括包说明、导出符号文档、字段说明和测试注释；保留必要的 API 名称、领域术语及代码示例。构建约束、`go:` 指令、工具标记、许可证原文等具有机器语义或必须保留原文的内容不翻译；翻译不得改变原有契约含义。
- 相关测试默认控制在 2 分钟内，受影响 package 验证默认控制在 5 分钟内。超过预算时先缩小范围并定位慢点，不通过反复延长 timeout 掩盖死锁、泄漏或错误分层。
- 同一个命令超时或无进展后不得盲目连续重试；先检查具体测试、goroutine、外部依赖和正常耗时基线，再决定继续、拆分或报告阻塞。
- 保留 Go 构建和测试缓存；只有排查缓存、随机性或 flaky test 时才使用 `-count=1`，不得习惯性清理 test cache。
- 独立且不争抢同一重资源的检查可以并行；全仓测试和全仓 race 测试默认不同时运行。
- 如果仓库尚未具备某项命令的运行条件，明确说明未验证项，不伪造成功结论。
- 导出符号提供有契约价值的 Go doc；测试优先验证可观察行为，不依赖无必要的私有实现细节。

## Git

- 不修改或丢弃用户已有的无关改动。
- 不使用破坏性命令清理工作区。
- 提交信息尽量使用中文，描述本次可验证的行为或设计变化。

### 单人开发与提交门禁

- 默认在当前分支工作，不主动创建、切换或合并分支；只有用户明确要求时才进行分支操作。
- Agent 的默认工作循环是：修改 → 最小相关验证 → 汇报变更和验证结果。默认不执行 `git add`、`git commit`、`git push` 或合并。
- Agent 完成修改后，必须把工作树留给用户审查。用户可以在 VS Code 或 Codex Review 中查看 diff，并决定是否需要修正。
- 用户要求修正时，继续修改并重新验证，仍然不提交；可以重复“审查 → 修正 → 验证”循环。
- 只有用户明确说“commit/提交”后，Agent 才能整理本次任务所属文件、暂存、复核暂存 diff 并创建提交。若存在归属不明或用户自己的改动，先停下确认，不替用户决定。
- 用户只说“审查/审核/看看 diff”不等于授权提交；“提交”默认指本地 `git commit`，推送远端必须另行明确说明。
- 创建提交前必须至少检查 `git status --short`、`git diff --check` 和 `git diff --cached --check`；提交后汇报提交 hash、提交内容和工作树状态。
- 具体命令和角色边界见 [`docs/DEVELOPMENT-WORKFLOW.md`](docs/DEVELOPMENT-WORKFLOW.md)。
