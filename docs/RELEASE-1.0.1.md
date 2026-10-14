# GSR v1.0.1 发布说明

版本：`1.0.1`（见 [VERSION](../VERSION)）。发布日期：2026-10-14。阶段：正式内部发布。代码由附注标签 `v1.0.1` 固定，上一版 `v1.0.0` 标签保留。

## 相对 v1.0.0 的主要变化

1. **节点组装入口更明确**：名称发布、Cluster/HTTP 监听和 Gate 创建统一放在 `self.Node()`；MainService 可以完成组装后退出，资源继续由 Run 或独立 GateService 管理。
2. **Cluster 通信入口更单一**：`self.Cluster()` 只保留来源绑定的 `Send/Call`；监听使用 `Node.OpenCluster`，仍按节点表端口绑定 `0.0.0.0`。
3. **Service 日志能力收窄**：`self.Logger()` 与 `Logger.WithService` 返回只提供 `Info/Error` 的 `ServiceLogger`，业务不能关闭共享 Logger 或重新绑定来源。
4. **应用配置由 Run 统一接线**：新增应用层 `gsr.RuntimeConfig` / `gsr.ClusterConfig`，不暴露 Logger 注入字段，由 Run 转换并统一注入共享日志器。
5. **统一 Node 运行对象**：内部 `application` 合并进 `Node`，同一 Run 的 Service 借用同一个节点对象，不同 Run 相互隔离；移除包装与名称注册转发。
6. **补充接入文档与参数覆盖**：增加 Cluster 调用教程、独立业务项目指南和 AGENTS.md 模板，补充字符串、数值、数组/切片、map、nil 与空集合的编码及真实 TCP 往返测试。

## 升级注意

本版包含源码不兼容的 API 调整，升级调用方时按下表迁移；不提供旧入口别名。

| v1.0.0 | v1.0.1 |
| --- | --- |
| `self.Register(name, id)` | `self.Node().RegisterService(name, id)` |
| `self.Cluster().Open(name)` | `self.Node().OpenCluster(name)` |
| `self.Cluster().ListenAddress()` | `self.Node().ClusterListenAddress()` |
| `self.OpenHTTP(address, handler)` | `self.Node().OpenHTTP(address, handler)` |
| `self.OpenGate(ctx, config)` | `self.Node().CreateGate(ctx, config)` |
| `gsr.Config.Runtime` 使用 `runtime.Config` | 使用 `gsr.RuntimeConfig` |
| `gsr.Config.Cluster` 使用 `cluster.Config` | 使用 `gsr.ClusterConfig` |
| 将 `Logger.WithService` 的结果声明为 `*logging.Logger` | 改为 `*logging.ServiceLogger`，关闭保留给原始 Logger owner |

升级依赖：`go get github.com/cnooljaw/gsr@v1.0.1`。完整接入方式见 [业务项目指南](APPLICATION-DEVELOPMENT.md)，参数规范化与 Handler 示例见 [Cluster 教程](CLUSTER-CALL-TUTORIAL.md)。

消息投递、Source 绑定、Cluster wire/ValueCodec、资源归属和关闭顺序保持不变。节点表 host 仍是访问地址，port 同时用于本机监听；访问限制由部署网络、ACL 或防火墙负责。完整契约见 [API 文档](API-DESIGN.md)和 [ADR 0012](adr/0012-node-capabilities-and-service-logging.md)。

## 发布验证

环境：Go 1.24.13，darwin/arm64；最低 Go 版本仍为 1.24.0。

- `GSR_PROCESS_TEST=1 go test -race ./... -timeout=2m`：通过，包含默认 Go 测试、双进程正常停机与强杀后恢复。
- `go vet ./...`：通过。
- Go 格式、diff 与发布相关 Markdown 本地链接检查：通过。

本轮未重跑 TypeScript 认证测试（客户端与 wire 未改），未启用真实 MySQL、负载或 Gate 压力门禁。内部接入范围继续沿用 [1.0 发布基线](RELEASE-1.0.md)，详细证据见 [验证清单](VALIDATION-STATUS.md)。
