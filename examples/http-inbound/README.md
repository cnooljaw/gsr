# Core 外的 HTTP 入站 Service 示例

这个示例模拟第三方系统通过 HTTP 查询或管理游戏状态，不是给游戏客户端增加第二套协议；游戏客户端仍走 WebSocket Gate。HTTP 请求由 Go `net/http` 接入，Handler 通过受信任的本地入口调用 Service，业务状态仍由 Service 独占，HTTP goroutine 不直接读写它。

从仓库根目录运行：

```sh
go test ./examples/http-inbound
```

示例分两层：`handler_test.go` 使用底层 `httpserver.Server` 和 `Runtime.Call`，验证成功、Service 失败的安全错误响应与有界等待；`admin_test.go` 使用推荐的 `gsr.Run` + MainService + `HTTPIngress`，验证多条路由和跨节点汇总。`httpserver` 只管理监听、默认有限请求头等待和有序关闭，不接管路由、认证、TLS 或限流。生产接入必须先确定第三方调用方身份和网络边界，不能直接把这个无认证的示例公开到公网。

路由由应用使用 Go 标准库组织，实际接线见 `handler_test.go` 的 `newRoomCountServer`：

```go
mux := http.NewServeMux()
mux.Handle("GET /rooms/count", roomCountHandler(local, roomID))
server, err := httpserver.New(httpserver.Config{Address: "127.0.0.1:0", Handler: mux})
```

增加接口时继续向同一个 `mux` 注册对应的业务 Handler；不需要为 `/模块/动作` 再建一套 GSR 路由表。Handler 负责解析请求、设置有限等待、调用相应 Service 并写回 HTTP 响应。

`admin_test.go` 的业务接线包括 `GET /users/{id}` 查询用户、`POST /users/{id}/ban` 修改用户状态、`GET /dashboard/{id}` 查询汇总。MainService 创建并注册 `users` 和 `dashboard` 后，用 `self.OpenHTTP` 开启同一监听；HTTP Handler 通过 `HTTPIngress.CallName` 按名称调用，不持有本地 Runtime 或 ServiceID。`dashboardService` 在自己的 Handler 内使用 `self.CallName` 查询用户，再通过来源绑定的 `self.Cluster().Call` 调用远端 `rooms`。远端节点仍是底层测试夹具；原有房间接口测试继续保护超时与安全错误响应。

普通应用的接线见 `admin_test.go` 中的 `adminMainService.OnStart`：`CreateService` → `Register` → `OpenHTTP` → `Exit`。MainService 退出不关闭 HTTP；`gsr.Run` 持有监听并在 Runtime 停止前关闭。路由在启动时构造；将来确有运行期路由变更需求时，整体重建并替换路由表，不为单条路由增加动态增删 API。`net/http` 已负责 HTTP 连接和 Handler 并发，不另建固定 HTTP Agent 池。

`Runtime.Call` 的 Source 固定为零，表示受信任基础设施注入，不冒充某个业务 Service 或 HTTP 客户端。HTTP Handler 的等待期限不强杀已开始执行的 Service Handler；原房间人数接口是只读查询，管理路由中的封禁操作是幂等示范。若以后接入非幂等 webhook，还需由业务定义超时后的结果处理与重试策略。
