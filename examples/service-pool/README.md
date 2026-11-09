# 有界 Service 池示例

GSR v1.0.3 起提供此能力。在仓库根目录运行：

```sh
go run ./examples/service-pool
```

输出类似下面两行；完成顺序、任务与成员的分配可能变化：

```text
job=1 worker=2 completed=1
job=2 worker=3 completed=1
```

MainService 创建名为 `workers` 的池，配置两个独立成员、每成员一条等待消息。然后创建两个调用者，各发送一个请求。调用者使用普通 `self.CallName` 等待工作结果，成员直接 return，由 Runtime 自动回复。示例没有手工启动 goroutine。

每个成员用 50 毫秒等待模拟阻塞 I/O，并更新自己的计数。不同调用者允许两个成员同时工作；同一个调用者连续同步 Call 仍逐个等待。`completed` 是每个成员自己的计数，不能作为全局业务计数。示例不依赖运行耗时判断并发，底层并发与容量契约由框架测试验证。

调用者通过 Command 将结果交给 MainService 收集，这是本例的协调方式；实际业务调用者可直接消费 Call 返回值。结果收齐后 MainService Exit，并在 OnStop 通知宿主结束演示。两秒宿主预算只保护有限示例，业务应自行设置请求和资源的超时。

代码按本地消息设计，返回普通 Go struct；扩展为 Cluster 调用时需按 Cluster 支持的编码类型定义边界参数与结果。事务、幂等、业务顺序和共享数据库连接池的并发安全由业务决定。池采用轮询单次选址，满载不自动改投或重试；完整语义见 [使用指南](../../docs/guides/SERVICE-POOLS.md)。

```sh
go test -race ./examples/service-pool -timeout=2m
```
