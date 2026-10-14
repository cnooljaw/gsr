# GSR 1.0.1

GSR 是使用 Go 实现的游戏服务器 Service Runtime，借鉴 Skynet 的 Service、消息和生命周期思想，以显式依赖、类型化 Handler 和有界消息接纳组织业务。

当前版本为 **1.0.1**（见 [VERSION](VERSION)），处于正式内部发布阶段。相对 v1.0.0 的变化、迁移说明和验证结果见 [1.0.1 发布说明](docs/RELEASE-1.0.1.md)；最初的接入范围见 [1.0 内部发布说明](docs/RELEASE-1.0.md)。

## 快速开始

需要 Go 1.24 或更高版本。在仓库根目录运行最小示例：

```sh
go run ./examples/counter
```

输出：

```text
count = 5
count = 0
```

应用以 `gsr.Run(ctx, config, mainService)` 启动。MainService 通过 `self.CreateService` 创建业务 Service，通过 `self.Node().RegisterService` 发布名称；业务使用 `self.Send/Call` 或 `self.SendName/CallName` 通信，通过 `self.Cluster()` 和 `self.Logger()` 取得自身来源绑定能力。

Service 显式实现 `Handle`，普通命令由具名字段持有的 `gsr.Dispatcher` 分发。每个 Service 独占自身可变状态，消息按 Mailbox 顺序在同一 runner 中执行；`Send` 成功只表示本地接纳，`Call` 必须有限等待。

## 接入示例

| 场景 | 入口 |
| --- | --- |
| 本地 Service 与命名通信 | [Counter](examples/counter/README.md) |
| 跨节点 Send/Call 与停机恢复 | [双节点](examples/twonode/README.md) |
| WebSocket、客户端协议与 Agent | [WebSocket Echo](examples/websocket-gate/README.md) |
| HTTP 管理入口 | [HTTP 入站](examples/http-inbound/README.md) |
| Service 内有限等待的 HTTP 调用 | [HTTP 出站](examples/http-outbound/README.md) |
| Redis 用户缓存 | [Redis](examples/redis-user-cache/README.md) |
| MySQL 产品 CRUD | [MySQL](examples/mysql-product/README.md) |

v1.0.1 按 [ADR 0012](docs/adr/0012-node-capabilities-and-service-logging.md) 分离 Node 组装、来源绑定的 Cluster 和窄日志入口，与 v1.0.0 源码不兼容。升级前按发布说明迁移调用；旧版代码和文档保留在 v1.0.0 标签。

## 文档与验证

- [文档索引](docs/README.md)、[项目术语](CONTEXT.md)和 [API 契约](docs/API-DESIGN.md)。
- [Cluster 节点间调用教程](docs/CLUSTER-CALL-TUTORIAL.md)：字符串、数字、字节串、数组、map 和多参数的跨节点调用。
- [独立业务项目开发指南](docs/APPLICATION-DEVELOPMENT.md)和 [业务项目 AGENTS.md 模板](docs/templates/AGENTS.application.md)：新项目的依赖、编码、状态与资源边界。
- [当前验证清单](docs/VALIDATION-STATUS.md)、[性能基线](docs/PERFORMANCE-BASELINE.md)和 [混合负载样本](docs/MIXED-LOAD-VALIDATION.md)。
- [开发工作流](docs/DEVELOPMENT-WORKFLOW.md)和 [Go 编码规范](docs/GO-CODE-STYLE.md)。

内部接入以当前 API 文档和测试为契约，后续改动需说明对调用方、运行语义和资源生命周期的影响。部署容量、跨机器故障、外部采集与真实数据库环境的验证按实际接入场景推进，详见发布说明。
