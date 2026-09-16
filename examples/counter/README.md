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

`service.go` 定义一个只拥有整数计数的 Service；复用已有 CommandDispatcher 检查参数并分派 Add(n)、Get()、Reset()。不增加 Timer、Cluster、数据库或网关，也不手写 goroutine。

`main.go` 只创建并启动 Runtime、创建 MainService、等待演示完成通知，最后 Close。服务器通常改为等待进程退出信号；MainService 退出本身不关闭 Runtime。

`main_service.go` 在 OnStart 内通过 ServiceContext.CreateService 创建 Counter，再 Send Add(2)、Send Add(3)、Call Get、Send Reset、Call Get，最后 Exit 并正常 return。每条业务消息的 Source 都是 MainService 的 ID；不注入 Runtime，不调用 Runtime.Send/Call，不新增 Launcher/ServiceManager。

同一 Service 顺序投递，Get 排在先前修改命令之后；Send 成功仅表示接纳，不表示修改已执行。最后一次 Get 完成后才结束演示，避免依赖 Stop 排空业务 Mailbox。Call 有一秒调用方预算，不无限等待；返回值自动回复，不手动 Reply。Exit 不终止当前 Go 函数，应紧接着 return；OnStop 发送完成通知，Counter 仍归 Runtime 管理，最后由宿主 Close 统一回收。

Counter 创建成功即可工作，不定义额外 Start Command。OnStart 是生命周期钩子，业务 Start 则仅在有分阶段准备需求时自行定义。测试中的 Runtime.Call 只作为受信任的行为验证入口，不是示例推荐的业务调用方式。

这是进程内的小整数演示，使用 Go int，不定义溢出保护、持久化或跨节点协议；不是计费、钱包或生产计数器。只需要普通计算时仍应直接调用函数，不必做成 Service。

```sh
go test ./examples/counter -v
```
