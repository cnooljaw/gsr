# 有界 Service 池

当前开发能力，尚未发布；不属于 v1.0.2 标签已有 API。设计裁决见 [ADR 0014](adr/0014-bounded-service-pools.md)。

## 创建与调用

在 MainService 的 OnStart 或其它当前 Service 回调中组装：

```go
err := self.Node().CreateServicePool(ctx, "settlement-write", gsr.ServicePoolConfig{
    Size:            4,
    MailboxCapacity: 32,
    NewService: func() gsr.Service {
        return newSettlementService(store)
    },
})
if err != nil {
    return err
}
```

`newSettlementService` 是业务自己的构造函数，返回实现 `gsr.Service` 的新指针。实例仍须显式声明 Handle，通常持有具名 Dispatcher 并转发。工厂只组装对象，资源初始化放 OnStart；共享 store 的并发安全由业务保证。不得把同一个有状态实例指针重复交给工厂返回；零大小无状态对象没有实例内可变状态，允许 Go 合并其地址。

外部调用保持现有形式：

```go
// 同一节点。
result, err := self.CallName(ctx, "settlement-write", settleCommand, request)

// 另一节点；池在目标节点创建，名称也在目标节点解析。
result, err := self.Cluster().Call(ctx, "settlement", "settlement-write", settleCommand, request)
```

两段代码是分别使用的调用示意，不应对同一结算重复执行。HTTPIngress 的 CallName 和 SendName 也使用同一张名称表；直接向具体 ServiceID 投递仍只访问该实例，不经过池路由。

## 并行与容量

上例最多同时执行四个成员 Handler，各成员独立串行，每成员最多等待 32 条消息，总等待容量最多 128 条，不包含正在执行的四条消息。MailboxCapacity 为零时沿用 Runtime 的默认容量，Size 必须大于零。

名称路由轮询一次，随后完全沿用现有 Mailbox 和自动 Reply。选定成员队列满返回 `runtime.ErrMailboxFull`；不扫描其它成员，也不自动换实例重试。池整体不保证到达、开始或完成顺序，某个慢成员后方仍可能排队。

多个独立调用方可以并行访问池。同一个调用方 Service 连续同步 Call，仍一次等待一个结果；Call 期间该调用方不处理其它消息。单次批量请求也不会由框架自动拆分。成员不要对同池发起依赖可用执行者的嵌套同步 Call，以免形成等待环；对自身 ServiceID 的 Call 仍拒绝。

## 生命周期与失败

- 创建先保留名称，构造并启动所有成员后发布；创建期间名称调用返回 `ErrServiceStarting`。普通名称和池名称互斥，重复注册不会调用工厂。
- 构造失败或启动失败撤销名称，并使用 Runtime 的局部停止能力回滚已创建成员，不停止其它 Service。与普通 Service 一样，工厂、OnStart 和失败启动的清理钩子必须遵守各自资源/取消契约；Go 无法强杀不返回的构造函数或生命周期代码。
- 创建者退出不影响池，Run 停机统一回收成员。一个成员 Exit 或 panic 后移除，幸存成员继续服务，不自动补员；最后成员退出释放名称。
- Call 超时只结束等待，已排队或正在执行的请求可能继续执行，不重放、不补执行者；实例实际结束前不会被框架视作新的空闲执行能力。结果不确定性、事务、幂等、业务顺序与重试归业务。
- 来源保持原调用路径语义：本地业务为调用方 ServiceID，HTTP/Cluster 入站为零；不借池伪装业务身份。

## 验证归属

池测试只保护新增路由与生命周期组合：多个调用者进入不同成员、独立容量、超时不重放、名称保留/冲突、无效工厂、创建失败/取消回滚和成员退出。既有双节点测试增加命名池轮询结果和零来源验证，复用原网络夹具；底层 PendingCall、Mailbox 串行性、codec 和完整关闭矩阵继续由原测试保护。
