# Runtime Handler 诊断事件流

状态：公开签名、事件字段、上限与关闭规则均已获用户确认，Handler 诊断切片已实现。

## 当前问题与范围

原 recordHandlerError 只累加 HandlerErrors/Panics，未保存消息关联信息和错误原因；Handler recovery 未采集 stack。本切片已补 Handler 的 Send/Call 错误与真实 panic 诊断，不同时扩展 OnStart/OnStop、Timer 或 admission 事件类别。其它诊断缺口保留后续处理，不能宣称本轮覆盖全部 Runtime 故障。

Inspect 继续提供状态和计数快照；事件流提供逐次故障记录。Core 只提供基础观测机制，不拥有日志文件、日志库、订阅系统或日志消费任务。

## Skynet 依据

只读依据：本地 `../skynet/lualib/skynet.lua`。协程失败路径生成 traceback，并区分有 Session 请求的错误响应；`skynet.error` 绑定 C 的 `c.error`。这依赖 Lua coroutine 与 Skynet C 日志环境。

GSR 借鉴“故障发生处保留来源、原因和堆栈”，使用 Go recovery 采集当前 goroutine 的 stack；不复制 Lua 协程机制，不让宿主日志函数在 Service runner 内执行。

## 已确认的最小公开契约

```go
func (runtime *Runtime) Diagnostics() <-chan Diagnostic
func (runtime *Runtime) DroppedDiagnosticCount() uint64

type Diagnostic struct {
    ServiceID ServiceID
    Source    ServiceID
    Command   CommandID
    Session   SessionID
    Class     ErrorClass
    Code      ErrorCode
    Message   string
    Stack     string
}
```

这里的 Diagnostic 属于 runtime 包，不依赖 cluster 包，也不要求两包共用事件类型或泛型诊断框架。

- ServiceID 是处理消息的 Service，Source 是原消息来源；Timer/基础设施来源仍为零。
- Session 为零表示 Send，非零表示本地 Call；不增加业务 OperationID 或 ClusterSessionID。
- Class/Code 使用已有错误身份规则；未知错误使用 ErrHandlerFailed 身份，真实 Handler panic 使用 ErrServicePanicked 身份。
- Message 保存本地错误文本快照，不保留原始 error 对象、Args、返回值或业务对象引用。它不是可直接发给客户端的安全 Message，也不保证自动脱敏。
- Stack 仅在实际 recover 到 Handler panic 时采集；普通返回 ErrServicePanicked 不凭错误码伪造堆栈。
- Message 最多 4 KiB、Stack 最多 16 KiB，超限按字节截断；字符串快照保证宿主不能通过事件修改 Service 状态。错误格式化防御 panic，但 Go 无法强杀无限阻塞的自定义 Error 方法，不增加后台格式化 worker。

选择文本快照而不是 Err error，是为了避免诊断队列长期持有业务对象或让日志消费者并发访问可变自定义 error。代价是诊断消费者不能 errors.As 原始 error；程序分类使用 Class/Code，Call 返回值的既有错误规则不改变。

## 已确认的投递与生命周期

- 每个 Runtime 固定一个容量 256 的队列，不新增 Config 字段。满载丢弃新事件并累加 DroppedDiagnosticCount，不等待消费者。
- 每次 Handler 非 nil error 或真实 panic 最多投递一条事件；保留原计数规则，不因事件丢弃而少计数。Send 不增加返回路径，Call 不改变回复或 Service 停止语义。
- Diagnostics 返回同一条单消费流，不是广播。宿主基础设施负责消费和输出，普通 Service 不启动日志 goroutine。
- 不在 Stop 返回或 shutdown timeout 时关闭出口；残留 Handler 仍可能产生日志。仅在 Runtime 确认资源释放、进入 Closed 时幂等关闭。
- Close 未能进入 Closed 时出口保持打开；残留任务结束后，宿主可再次 Close 完成释放。日志消费 owner 也必须具备自己的取消方式，不能在异常停机时无限等待通道关闭。
- Closed 后缓冲事件仍可读取，丢弃计数仍可查询；Close 不等待消费者读空或日志落盘。nil Runtime 的诊断通道返回 nil，丢弃计数返回零。

## 验证门禁

覆盖 Send/Call 的关联字段、普通错误与 typed-nil 错误、真实 panic 的本地 stack 与安全 Reply 分离、后续消息行为、无消费者与队列满载、文本快照及截断，以及 Stop/Close 超时后的晚到事件、最终关闭、重复关闭与并发安全。

不新增 DiagnosticSink 接口、callback、全局 logger、事件路由器或转发 goroutine。后续 Cluster 诊断简化与此切片分开处理。
