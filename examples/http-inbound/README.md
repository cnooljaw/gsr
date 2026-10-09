# Core 外的 HTTP 入站 Service 示例

这个示例模拟第三方系统通过 HTTP 查询或管理游戏状态，不是给游戏客户端增加第二套协议；游戏客户端仍走 WebSocket Gate。HTTP 请求由 Go `net/http` 接入，Handler 通过受信任的本地入口调用 Service，业务状态仍由 Service 独占，HTTP goroutine 不直接读写它。

从仓库根目录运行：

```sh
go test ./examples/http-inbound
```

## 推荐业务接线

先读 `main_service.go` 的 `adminMainService.OnStart`：`gsr.Run` 启动入口 Service，入口创建并注册 `users` 和 `dashboard`，打开 HTTP 后自身退出。HTTP 由 Run 持有，MainService 退出不会关闭它。

```text
gsr.Run → MainService → CreateService / Register → OpenHTTP → Exit

HTTP Handler → HTTPIngress.CallName → 本地 Service
                                        └→ self.Cluster().Call → 远端 Service
```

`admin.go` 用标准库 `http.ServeMux` 组织 `GET /users/{id}`、`POST /users/{id}/ban` 和 `GET /dashboard/{id}`。HTTP Handler 保存 `HTTPIngress`，通过名称调用，不持有 Runtime 或 ServiceID。增加接口只需向 mux 注册对应 Handler，不新建 GSR 路由框架。

OpenHTTP 的构造回调只组装路由，不启动资源或调用业务 Service。已打开监听或节点已关闭时，会在调用构造回调前拒绝；并发首次打开可能分别构造路由，但最终只接纳一个监听。

`admin_service.go` 的 `dashboardService` 在自己的 Handler 内用 `self.CallName` 查询用户，再用 `self.Cluster().Call` 调用远端 `rooms`。HTTP 请求的 Source 为零；业务 Service 发起后续调用时，Source 是它自己的 ServiceID。HTTP goroutine 不冒充业务 Service，也不直接读取 Service 状态。

HTTP 示例将请求 deadline 和稳定的 `runtime.ErrCallTimeout` 都映射为 504，其余调用失败返回安全的 503，不透传内部错误文本。Service 转交错误及 Cluster wire 只保留 Class/Code 和安全 Message，不保证保留 `context.DeadlineExceeded` 错误链；错误映射不能只检查 Go context 错误。

`admin_test.go` 验证上述真实业务接线；其中远端节点是底层夹具。路由在启动时构造，将来确有变更需求时整体重建并替换，不添加逐条动态增删接口或 HTTP Agent 池。

## 底层边界验证

`handler.go`、`service.go` 和 `handler_test.go` 保留显式 `Runtime.Call` 与 `httpserver.Server` 夹具，保护房间人数接口的安全错误响应、超时和取消。它们不是普通业务的推荐启动方式。

`httpserver` 只管理监听、有限请求头等待和有序关闭，不接管路由、认证、TLS 或限流。生产接入必须先确定第三方调用方身份和网络边界，不能直接把这个无认证的示例公开到公网。

`Runtime.Call` 的 Source 固定为零，表示受信任基础设施注入，不冒充某个业务 Service 或 HTTP 客户端。HTTP Handler 的等待期限不强杀已开始执行的 Service Handler；原房间人数接口是只读查询，管理路由中的封禁操作是幂等示范。若以后接入非幂等 webhook，还需由业务定义超时后的结果处理与重试策略。
