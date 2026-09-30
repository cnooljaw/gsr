# ADR 0011：以 Service 为中心的应用入口与统一 Command

## 状态

Accepted，2026-09-16 用户确认；已落地最小应用运行链。Cluster 能力入口为应用层 `gsr.ServiceContext.Cluster()`。

本裁决替代旧设计中节点级 CommandCodecRegistry 作为远端命令准入清单的要求，以及要求应用理解 app.Host 的使用模型。不改变 Core 与 Cluster 的职责边界。

## 参考依据

只读参考 `skynet/examples/cluster1.lua` 在启动回调中配置节点、创建 Service、注册名称并 cluster.open；`examples/main.lua` 在组装完成后 skynet.exit。前者本身没有退出，不能混同两个例子的实际行为。

`lualib/skynet/cluster.lua` 使用 skynet.pack 编码调用参数，在模块初始化时取得 clusterd。Lua Service 执行环境及 C/Lua 调度机制隐藏了组件取得和发送者身份。GSR 沿用使用意图，不照搬协程模型，也不通过 Go 全局“当前 Service”或 goroutine-local 推断来源。

## 裁决

- 一个应用节点只有一个 Runtime、一个 Cluster。这个约束不等于要求进程级全局单例；同进程测试多个逻辑节点的隔离方式留给实现。
- Go main 读取配置并启动运行环境，等待退出和统一收尾；普通 MainService 完成业务 Service 创建、名称发布、Cluster.Open，然后 Exit。入口退出不关闭 Runtime、Cluster 或已创建 Service。
- 对使用者组件就叫 cluster，不叫 network。不要求业务理解 Host、codec registry 或手工组件关闭链；停止继续扩展 Host 的公开使用模型。
- CommandID 保持 uint32，由目标 Service 解释。不同 Service 可复用编号且含义不同；操作身份是目标 Service 与 CommandID 的组合。无“本地命令”和“远程命令”两类。
- Cluster 使用统一默认值编码，不再按 CommandID 维护额外准入/codec 清单。跨节点后仍投递到目标本地 Handler/Dispatcher；本地路径不因此引入编码。
- 未知 Command 由目标判断：Send 丢弃，不返回远端业务回执；Call 立即返回 ErrUnknownCommand，不静默等超时。Cluster 不以命令未注册为由拒绝合法载荷。
- 帧边界、长度、深度、载荷上限及解码校验继续保留；非法输入不能进入 Handler。不支持编码的参数明确失败，Go 指针和任意自定义对象不会自动获得跨进程语义。
- Codec 不是权限表；访问控制如有需求应单独设计。取消清单意味着合法但未知命令会进入目标 Mailbox，仍受有界 admission 约束。
- 业务来源仍由框架绑定。节点唯一 Cluster 不等于唯一发送 Service；不得以共享可变的“当前 Service”推断 Source。
- 应用 Service 通过 `cluster := self.Cluster()` 取得绑定当前 Service 的能力入口。框架从 `self.Self()` 绑定调用来源，而不是从收到消息的 `CommandContext.Source()` 继承上一跳；业务不传 source、不调用 Client 构造器，也不手工注入 Host/Cluster。
- 每节点仍只有一份 Cluster 资源。来源绑定入口只是使用视图，不创建独立连接、队列、PendingCall 表或后台任务，不需要业务单独关闭。
- Cluster 能力组合放在应用层 `gsr.ServiceContext` / `gsr.CommandContext`；Core 的 `runtime.ServiceContext` 不依赖 Cluster，不引入通用能力注册容器。新 Service 由框架接线，业务仅传实例。
- 绑定入口遵守所属 Service 上下文的使用范围，不得借给其它 Service 或异步任务；节点唯一不授权进程级“当前 Service”变量。MainService 退出不能关闭其它 Service 使用的共享 Cluster。

## 目标业务形状

以下为调用步骤示意，完整可运行代码见 examples/twonode。cluster 是通过 self.Cluster() 取得的当前 Service 绑定入口，不是全局包状态。

```go
// main：运行环境统一处理启动、进程等待及关闭。
err := gsr.Run(ctx, config, &MainService{})

// MainService.OnStart：每一步照常检查 error。
cluster := self.Cluster()
id, err := self.CreateService(ctx, &EchoService{})
err = self.Register("echo", id)
err = cluster.Open("battle")
self.Exit()
return nil

// 同一业务 Command，只改变投递目标。
value, err := self.Call(ctx, id, echoCommand, "hello")
value, err = cluster.Call(ctx, "battle", "echo", echoCommand, "hello")
```

## 实施顺序与验收

1. 按 self.Cluster() 落地应用层上下文与框架自动来源绑定，避免将 Host 换名后再暴露同样的组装负担。验证多个 Service 交错调用时来源不串、收到非零 Source 的 Service 转调时使用自身 ID，以及同进程独立节点不串资源。
2. 调整协议契约和测试，再移除节点级 Command codec 清单；验证不同 Service 复用编号、未知 Send 丢弃、未知 Call 及时报错，以及坏载荷仍被拦截。
3. 收口运行入口、名称注册和按节点表 Open，将双节点示例改成目标写法；不为旧 Host 或 registry 添加迁移别名。
4. 验证多 Service 并发来源隔离、仅出站节点、初始化回滚、有序关闭，以及双进程恢复。

实施进度：已移除 app.Host、app.Config.Commands、CommandCodecRegistry，统一默认编码并由目标解释 Command。gsr.Run、self.Cluster()、self.Logger()、self.Register()、本地 SendName/CallName 和按节点名 Open 已落地；双节点示例不再持有 Host 或手工绑定 Client。底层 Runtime/Cluster 仍保留基础设施 API，应用层通过最小 Service 适配器复用 Core 生命周期，不增加 ServiceManager。

## 后续裁决：名称生命周期（2026-09-30）

名称映射归目标 ServiceID，而非调用 Register 的 Service。Register 只接受当前应用内仍存活的目标 ID，避免目标退出后重新发布陈旧地址。目标 Service 启动失败、Exit 或停止时，应用层在调用其业务 OnStop 前删除全部对应名称；发布名称的 MainService 先退出，不会撤销仍存活的目标 Service。清理后允许新实例注册同名，但不在目标存活期间静默改绑。Core 不持有名称表；无法强杀不返回的 Handler，因此 Handler 尚未返回时不能保证名称已经清理。

业务组装推荐先 `CreateService`，成功返回后由创建者（通常是 MainService）`Register`；不要求目标 Service 在 OnStart 自行注册。后者会在 Runtime 正式发布目标 Service 前暴露名称，调用方可能在这个窗口解析到尚不可投递的 ID。业务 OnStop 不手写注销。
