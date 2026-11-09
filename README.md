# GSR

GSR 是使用 Go 实现的游戏服务器 Service Runtime，借鉴 Skynet 的 Service、消息和生命周期思想，以显式依赖、强类型 Handler 和有界消息接纳组织业务。

已发布版本为 **v1.0.3**（[VERSION](VERSION)），包含有界 Service 池，主要变化和升级说明见 [发布说明](docs/releases/v1.0.3.md)。

## 快速开始

需要 Go 1.24 或更高版本。在仓库根目录运行：

```sh
go run ./examples/counter
```

输出：

```text
count = 5
count = 0
```

应用使用 `gsr.Run(ctx, config, mainService)` 启动。MainService 创建业务 Service，通过 `self.Node()` 组装名称与监听；业务使用 `self.Send/Call`、`self.SendName/CallName` 和来源绑定的 `self.Cluster()` 通信。

每个 Service 独占可变状态，Handler 串行执行并通过返回值自动回复。Send 成功只表示本地接纳；Call 有限等待，等待期间当前 Service 不处理其它消息。需要并行时，服务池可把同一名称路由到多个独立成员，各成员继续串行。

## 从哪里开始

- 开发业务：[独立项目接入](docs/guides/APPLICATION.md) → [Cluster 调用](docs/guides/CLUSTER.md) → [客户端协议](docs/guides/CLIENT-PROTOCOLS.md) / [Service 池](docs/guides/SERVICE-POOLS.md)。
- 查看行为：[术语](CONTEXT.md) → [API 契约](docs/API-DESIGN.md) → [设计与 ADR](docs/README.md#2-理解契约与设计)。
- 修改框架：[开发工作流](docs/development/WORKFLOW.md) → [Go 规范](docs/development/GO-CODE-STYLE.md) → [验证覆盖](docs/validation/COVERAGE.md)。

完整示例、性能测量、后续范围及各版本迁移统一见 [文档索引](docs/README.md)。
