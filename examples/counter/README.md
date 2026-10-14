# 最小 CounterService 示例

从仓库根目录执行：

```sh
go run ./examples/counter
```

输出：

```text
count = 5
count = 0
```

`service.go` 定义一个只拥有整数计数的应用 Service；构造时把 Add(n)、Get()、Reset() 绑定到具名 `gsr.Dispatcher` 字段，并显式用 `Handle` 转发到它。业务方法直接使用强类型参数，不手写 `switch` 和参数断言。示例不调用 Timer、Cluster、数据库或网关，也不手写 goroutine。

`main.go` 使用统一的 gsr.Run(ctx, config, mainService)，不手工创建或关闭 Runtime。有限演示向入口提供完成通知，OnStop 通知 main 取消应用等待；服务器通常改为等待进程退出信号。MainService 的 Exit 本身仍不关闭 Runtime，通知只是此演示的应用完成约定。框架日志写到 io.Discard，使输出只保留两行计数。

`main_service.go` 在 OnStart 内通过 ServiceContext.CreateService 创建 Counter，用 `self.Node().RegisterService("counter", id)` 发布本节点名称，再通过 `SendName/CallName` 顺序执行 Add(2)、Add(3)、Get、Reset、Get，最后 Exit 并正常 return。名称每次解析为本节点 ServiceID，仍走原有 Mailbox；每条业务消息的 Source 都是 MainService 的 ID。不注入 Runtime，不调用 Runtime.Send/Call，不新增 Launcher/ServiceManager。

同一 Service 顺序投递，Get 排在先前修改命令之后；Send 成功仅表示接纳，不表示修改已执行。最后一次 Get 完成后才结束演示，避免依赖 Stop 排空业务 Mailbox。Call 有一秒调用方预算，不无限等待；返回值自动回复，不手动 Reply。Exit 不终止当前 Go 函数，应紧接着 return；OnStop 发送完成通知，Counter 仍归 Runtime 管理，最后由 Run 统一回收。

Counter 创建成功即可工作，不定义额外 Start Command。OnStart 是生命周期钩子，业务 Start 则仅在有分阶段准备需求时自行定义。测试覆盖真实运行输出、独占未启动实例的参数校验，以及输出失败时保留错误。

这是进程内的小整数演示，使用 Go int，不定义溢出保护、持久化或跨节点协议；不是计费、钱包或生产计数器。只需要普通计算时仍应直接调用函数，不必做成 Service。

```sh
go test ./examples/counter -v
```
