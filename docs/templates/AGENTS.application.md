# AGENTS.md

## 项目定位与依赖

- 本仓库是基于 GSR 的 Go 业务项目，负责业务规则、协议、持久化和部署。
- GSR 模块为 `github.com/cnooljaw/gsr`；当前接入基线为 `v1.0.0`。实际版本以本仓库 `go.mod` 为准，升级时同步本文件与接入文档。
- GSR 是框架依赖；不在业务任务中修改 module cache、vendor 或相邻框架仓库。框架缺陷先记录版本和最小复现，按明确授权单独修复、验证和升级。
- 先完成最小可验证的业务纵向切片，不提前建设没有调用者的通用框架。

## 文档入口

- 本文件只记录工作规则；稳定的设计结论写入本仓库文档。
- `CONTEXT.md`：稳定业务术语和身份含义。
- `docs/ARCHITECTURE.md`：状态 owner、Service 划分、节点与资源职责。
- `docs/GSR-INTEGRATION.md`：依赖版本、已采用的 GSR 能力、配置和部署约束。
- `docs/PROTOCOL.md`：Command、客户端消息、参数、返回值与错误代码。
- `docs/GO-CODE-STYLE.md`：本项目采用的 Go 编码规范。
- 文档尚不存在时，按当前功能建立必要契约，不用假定的设计填充完整架构。

GSR 参考契约按依赖版本阅读：

- [API 契约](https://github.com/cnooljaw/gsr/blob/v1.0.0/docs/API-DESIGN.md)
- [稳定术语](https://github.com/cnooljaw/gsr/blob/v1.0.0/CONTEXT.md)
- [Go 规范](https://github.com/cnooljaw/gsr/blob/v1.0.0/docs/GO-CODE-STYLE.md)
- [发布范围](https://github.com/cnooljaw/gsr/blob/v1.0.0/docs/RELEASE-1.0.md)

本项目业务契约与所依赖的 GSR API 契约同时成立。代码、测试与文档冲突时先确认预期，不静默改接口或运行语义；协议、所有权、生命周期的重要变化先说明方案与代价，由用户裁决，再更新文档和测试。普通局部实现细节不逐项请求裁决。

## 应用与 Service

- 宿主负责配置、进程信号、外部资源和 `gsr.Run`；普通 MainService 负责创建、命名和发起业务。
- 普通业务实现 `gsr.Service`，使用 `gsr.ServiceContext` / `gsr.CommandContext`；不持有 Runtime，不用 `Runtime.Send/Call` 绕过业务来源，不手工组装 Host 或 ClusterClient。
- 在当前回调中使用 `self.Cluster()` / `self.Logger()`。不保存或跨 goroutine 使用 ServiceContext、CommandContext、protocol.Context，不用全局或 goroutine-local 推断当前 Service。
- 每份可变权威状态只有一个 Service owner；Service 之间只能消息通信，不持有彼此实现指针、不绕过 Mailbox 读写状态。
- Service 是运行单元，纯计算和普通同步依赖使用 Go 函数或具名字段，不为了代码分类额外包装成 Service。
- 创建成功的 Service 不随创建者退出而停止；`self.Exit()` 后立即返回，它不停止整个应用。启动失败的资源回滚和已创建子 Service 的补偿需有明确 owner。
- ServiceID 是本次 Runtime 的临时地址，不持久化或当作玩家/房间身份；重建后重新解析名称或更新路由。

## Command 与编码

- 每个 Service 显式实现 `Handle`。普通命令在构造时用具名 `*gsr.Dispatcher` 字段和静态命令表绑定强类型 Handler，不匿名嵌入 Dispatcher 获取 Handle。
- Handler 第一参数为精确的 `gsr.CommandContext`，参数数量和类型固定；返回 `error` 或 `(结果, error)`，不使用 variadic Handler。只绑定已实现的命令。
- 没有业务命令的 Service 可以直接返回 `runtime.ErrUnknownCommand`，无需空 Dispatcher。只有实测热点才讨论局部分发优化，不默认生成代码或增加分发层。
- 重要框架角色的编译期断言紧跟类型定义；多个接口放同一个 var 块。Agent 实现 `gsr.Service` 与 `gsr.ClientMessageHandler` 时两者均声明。按实际方法集用指针/值形式，不为断言修改 receiver，不批量断言无关工具接口。
- CommandID、参数顺序/类型、结果及错误代码属于业务协议，集中记录并测试；不因不同 Service 可以复用编号就复用不同语义的协议契约。
- 本地引用型 Args 必须是消息专用快照，按数据结构复制嵌套引用；成功接纳后不再修改。接收方视为只读，需要保存/修改时复制。结果不得泄漏 Service 内部的可变状态引用。
- 同时供本地与 Cluster 使用的边界采用 `int64`、`uint64`、`float64`、`[]any`、`map[string]any` 等规范类型；Dispatcher 不做隐式转换。struct 和具名领域类型在边界显式展开/编码/重建，返回值同样须可编码。
- typed 调用方法负责输入快照和结果检查，持有 self 的调用对象仅在当前回调中使用；不额外建立通用 RPC 框架。

## 消息与并发

- Send 成功只代表接纳，不代表送达或业务成功；需要结果使用 Call，显式处理队列满和拒绝。
- Call 必须设置有限业务等待；等待期间当前 Service 不处理下一条消息。避免循环等待，不假定框架能检测所有跨 Service/跨节点依赖环。
- 超时或断线不证明未执行；重试、幂等、RequestID、Revision、补偿属于业务。Cluster 不自动重放，业务不得手工或异步 Reply。
- Service 和业务层不直接启动 goroutine；状态推进使用 Command、一次性 Timer 或其它 Service。
- Timer 只投递 Command，周期行为在 Handler 中重新登记，失败、取消和过期规则明确处理。
- 外部 I/O 有有限 context/超时；需要并行时使用有明确 owner 和固定并发上限的基础设施 worker，负责取消、关闭、等待、panic 和错误归属。worker 不持有 self、不写业务状态，结果经受控入口回到状态 owner 校验。

## 生命周期、接入和错误

- 资源明确区分拥有与借用；拥有者负责关闭，借用者不 Close。宿主在 GSR 使用者退出后关闭 Redis/MySQL 等外部共享资源；不自行关闭 `self.Logger()`。
- OnStart 部分失败也应可由 OnStop 清理；Handler/生命周期不得无限阻塞，不把退出超时当作资源已释放。
- HTTP 通过 `self.OpenHTTP` / `gsr.HTTPIngress` 接入；HTTP Handler 可保存该窄入口，不能保存 ServiceContext。鉴权、输入、有限等待和响应错误映射由业务负责。
- Gate 管字节连接，Protocol 管消息编解码，Agent 组合二者。Agent 显式实现 Handle/HandleClientMessage，普通命令与协议使用各自具名 Dispatcher；业务 proto、目录和 Handler 留在业务仓库。
- 框架通用 ErrorResponse 编号 9001 不可占用。客户端消息 ID/RequestID 不与内部 CommandID/SessionID 混用。
- Gate MAC/seq 不替代玩家 token 鉴权；Plain 仅回环/私网，公网使用 WSS。Cluster 用于可信网络，NodeID/Source 不是认证凭据。
- 正常业务拒绝使用稳定 Domain StatusError，错误代码不占用 `runtime.` 前缀；用 `errors.Is` 或 Class/Code 分支，不比较 Message。预期失败不 panic，不将内部错误文本、stack 或敏感字段作为远端响应。
- 日志通常只在负责处置的边界记录，避免多层重复；业务日志使用当前 self.Logger，token、密钥、敏感原始载荷不写日志。关键审计有独立的可靠性要求。

## Go 编码

- Go 代码使用 gofmt，具名函数采用正常多行函数体，错误提前返回，避免无必要的 else 和深层嵌套。
- receiver 命名表达类型且保持一致，常用 service/agent，不命名为 this/self；context.Context 用 ctx，Service/Command 上下文用 self，未使用参数用 `_`。
- 错误包装用 `%w` 增加有定位价值的操作与身份；多阶段清理明确聚合策略。Must 构造只用于静态接线的编程错误。
- 构造器建立不变量，依赖使用具名字段；接口由使用者定义并保持窄，只有真实边界、多实现或测试隔离需求时引入。
- 配置只在加载边界读文件/环境变量/命令行，业务使用校验后的配置对象；敏感信息不提交。
- 自然语言注释尽量中文；导出符号写契约性 Go doc，每个 Test 函数前说明保护的可观察行为。机器指令、许可证和必要领域术语不改义。

## 验证与交付

- 新行为先补可失败的测试；先搜索已有用例并扩展，不为同一框架契约在多层重复建完整夹具。
- 优先验证业务增量：状态/拒绝条件、参数快照、协议转换、接线、超时/幂等和资源关闭；路由与生命周期测试走真实消息入口。
- 开发循环按改动跑最小相关测试，连贯切片完成后验证受影响 package；全仓 test/vet 留给跨包功能、集成或发布节点。
- 并发、生命周期、共享资源变更在切片收尾跑相关 race；不每轮机械执行全仓 race，不重复已被更完整验证覆盖的检查。
- 纯文档检查 diff/格式/链接，可运行代码示例需编译或演练；不伪造未具备运行条件的检查结论。
- 相关测试默认预算 2 分钟、package 验证 5 分钟；超时先定位，不能盲目重试或延长 timeout 掩盖问题。保留 Go cache，非排查需要不使用 `-count=1`。
- 上线前按实际环境验证网络、外部数据库、WSS、队列拒绝、日志与停机/恢复；交付说明已验证、未验证和环境限制。
- 本仓库具体测试命令和外部依赖门禁写入项目文档，不能直接把 GSR 仓库的测试命令当成业务验收。

## Git

- 不修改、丢弃或提交用户已有的无关改动，不用破坏性命令清理工作树。
- 默认在当前分支工作，不主动创建、切换、合并分支；分支操作需用户明确要求。
- 默认执行修改 → 验证 → 汇报，留工作树供审查，不自行 add/commit/push。
- 用户明确说“提交/commit”后才暂存本次所属文件、复核并提交；归属不明先确认。中文提交信息描述可验证变化。
- 提交前至少检查 status、diff --check、diff --cached --check；提交后报告 hash、内容和工作树状态。
- “提交”默认是本地 commit，推送需另行明确授权；审核、审查或看 diff 不等于提交授权。
