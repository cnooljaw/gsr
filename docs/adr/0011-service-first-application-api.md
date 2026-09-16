# ADR 0011：以 Service 为中心的应用入口与统一 Command

## 状态

Accepted，2026-09-16 用户确认；待实现。具体 Cluster 能力取得及 Source 绑定接口仍待收口。

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

## 目标业务形状

以下是方向示意，不是当前可编译 API。cluster 表示当前节点能力；它如何取得和绑定来源尚未裁决。

```go
// main：运行环境统一处理启动、进程等待及关闭。
err := gsr.Run(config, &MainService{})

// MainService.OnStart：每一步照常检查 error。
id, err := self.CreateService(ctx, &EchoService{})
err = cluster.Register("echo", id)
err = cluster.Open("battle")
self.Exit()
return nil

// 同一业务 Command，只改变投递目标。
value, err := self.Call(ctx, id, echoCommand, "hello")
value, err = cluster.Call(ctx, "battle", "echo", echoCommand, "hello")
```

## 实施顺序与验收

1. 先明确 Service 如何取得 cluster 并自动绑定来源，避免将 Host 换名后再暴露同样的组装负担。
2. 调整协议契约和测试，再移除节点级 Command codec 清单；验证不同 Service 复用编号、未知 Send 丢弃、未知 Call 及时报错，以及坏载荷仍被拦截。
3. 收口运行入口、名称注册和按节点表 Open，将双节点示例改成目标写法；不为旧 Host 或 registry 添加迁移别名。
4. 验证多 Service 并发来源隔离、仅出站节点、初始化回滚、有序关闭，以及双进程恢复。

当前 app.Host、app.Config.Commands、CommandCodecRegistry 及相关测试仍是旧实现，不因本裁决被描述为已完成迁移。只做文档裁决时不修改运行行为。
