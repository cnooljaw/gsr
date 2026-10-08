# Cluster 入站请求并发与背压

状态：已获用户确认。采用每连接有界并发 Call、满载暂停读取以及本地接纳顺序；正式契约同步至 API-DESIGN 和 ADR 0005。

## 需要明确的问题

本地适配器已能通过 Runtime.Call 等待目标 Service。如果直接在 socket 读循环中同步调用它，一个慢 Call 就会阻塞该连接后续请求的读取，即使后续请求的目标是其它空闲 Service。

不能用无限 goroutine 解决此问题：每个等待者持有参数、Session 和连接相关资源，必须设置上限并明确关闭归属。

## Skynet 的实际机制

`lualib/snax/gateserver.lua` 的 `dispatch_queue` 使用 `skynet.fork` 继续分派，避免当前消息处理阻塞队列分派。`service/clusteragent.lua` 使用 `skynet.rawcall` 向目标 Service 发起本地调用；`lualib/skynet.lua` 中 `yield_call` 挂起当前 Lua coroutine，并通过 session 表恢复它。

GSR 可以沿用“读包与等待本地结果分离、响应沿原连接写回”的结构。Go 需要显式管理 goroutine 数量、取消与等待，不能将 Skynet 的协程调度直接翻译成无限启动 goroutine。业务 Service 仍由 Runtime runner 串行执行。

## 已确认方案

单个入站连接由外层连接组件管理：

1. 一个读循环按字节流顺序读取、校验帧。
2. Send 在读循环中完成本地接纳，不等待目标 Handler，保持同连接 Send 的接纳顺序。
3. Call 先取得该连接的有限并发名额，再交给连接组件管理的等待任务调用 Runtime.Call；只允许最多 `MaxInboundCalls` 个任务同时执行，不额外建立无界队列。
4. Call 名额耗尽时暂停读取后续请求，直到名额释放或连接关闭。此时连后续 Send 也可能等待，这是满载背压的明确代价。不会因名额已满而返回一个伪装成目标 Mailbox 满载的错误。
5. 每个任务将 Response/Error 交给同一 streamConnection 串行写入；不同 Call 可以按完成顺序回包，依靠 ClusterSessionID 匹配。
6. 连接结束时取消所有基础设施 Call 的 context、关闭 socket 解除 I/O 等待，并等待所属任务退出；不强杀目标 Handler、不重放请求。

`MaxInboundCalls` 必须为正数，由连接组装时显式提供，默认值留到 TCP Config 切片确定。节点级还必须限制入站连接数，使“连接数 × 每连接 Call 上限”形成总等待任务上限；没有节点级上限不能宣称生产资源已经有界。

满载暂停读取也可能延后检测对端断开，直到名额释放；本地取消或主动关闭必须能直接解除等待，不依赖下一次读包。

## 顺序保证及代价

- 同一 Service 的 Handler 永不并发，仍按本地 Mailbox 成功接纳顺序处理。
- 同一连接的 Send 由单读循环顺序接纳。
- 并发 Call 的 goroutine 调度可能改变本地接纳顺序；后读到的 Send 也可能先于此前启动但尚未接纳的 Call 入队。因此，不承诺混合 Send/Call 的 TCP 到达顺序等于目标 Service 执行顺序。
- 需要前后依赖的业务应等待前一个 Call 完成后再发起下一步。若要求所有混合请求严格按 wire 顺序接纳，就需要进一步拆分基础设施 Call 的“接纳”与“等待”能力，而不是偷偷改变现有同步 Runtime.Call。
- codec 和名称解析器必须能并发安全使用，且不执行无限阻塞的 I/O。连接组件需要捕获基础设施处理任务的 panic 并记录诊断；不能仅依赖 Runtime 对业务 Handler 的 recovery 来保护解码步骤。
- 诊断使用 listener 持有的共享有界非阻塞出口，连接直接上报地址、Session 和错误；不设每连接诊断队列、转发任务或外部 callback。慢消费者不阻塞网络读取，满载共享丢弃计数；噪声连接可能挤占其它连接的诊断容量。单连接不能关闭共享出口，由组装 owner 等全部生产者退出后关闭。
- listener 负责停止 Accept、关闭连接并等待连接任务；单连接负责关闭自身 socket、取消 Call 并等待自身任务。context 取消不会自动解除 socket Read，保留必要的取消监视机制，不为消除代码相似性而移除。

## 不选用的简单实现

全连接串行 Call 最容易实现，但无法让同连接其它 Service 请求绕过慢 Call；无限并发则无法给资源和关闭时间设定可靠边界。

推荐先采用上述有界并发与满载暂停读取，保留本地 Mailbox 顺序，而不新增“wire 顺序等于本地执行顺序”的承诺。

## 确认后的实施与门禁

- 内部入站连接组件管理读循环、Call 并发名额、取消和等待，不向业务公开 goroutine 能力。
- 通过 net.Pipe 验证慢 Call 期间其它目标仍可处理请求、满载时不继续创建任务、关闭取消等待、回复不串帧。
- 验证连续 Send 顺序、混合请求只保证本地接纳顺序、坏帧诊断以及 codec/名称解析 panic 的本地错误归属。
- 完成后运行全仓测试、vet 和 race，再接入真正的 TCP listener 与节点级连接数量限制。
