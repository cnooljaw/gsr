# Core 外的只读 HTTP 入口

这个示例模拟第三方系统查询房间人数，不是给游戏客户端增加第二套协议；游戏客户端仍走 WebSocket Gate。HTTP 请求由 Go `net/http` 接入，Handler 通过基础设施入口 `Runtime.Call` 查询本地 `roomService`，收到结果后由 HTTP Handler 写回 JSON。房间状态仍由 Service 独占，HTTP goroutine 不直接读取它。

从仓库根目录运行：

```sh
go test ./examples/http-inbound
```

测试通过框架的 `httpserver.Server` 启动真实回环 HTTP 监听，验证成功、Service 失败的安全错误响应和有界等待。示例提供业务 `http.Handler`；`httpserver` 只管理监听、默认有限请求头等待和有序关闭，不接管路由、认证、TLS 或限流，也不扩展 Runtime 或 WebSocket Gate。宿主停机时先关闭 HTTP Server 并等待在途 Handler，再关闭 Runtime。生产接入必须先确定第三方调用方身份和网络边界，不能直接把这个无认证的示例公开到公网。

路由由应用使用 Go 标准库组织，实际接线见 `handler_test.go` 的 `newRoomCountServer`：

```go
mux := http.NewServeMux()
mux.Handle("GET /rooms/count", roomCountHandler(local, roomID))
server, err := httpserver.New(httpserver.Config{Address: "127.0.0.1:0", Handler: mux})
```

增加接口时继续向同一个 `mux` 注册对应的业务 Handler；不需要为 `/模块/动作` 再建一套 GSR 路由表。Handler 负责解析请求、设置有限等待、调用相应 Service 并写回 HTTP 响应。

`admin_test.go` 增加了一个稍完整的业务接线：`GET /users/{id}` 查询用户、`POST /users/{id}/ban` 修改用户状态、`GET /dashboard/{id}` 查询汇总。三条路由共用同一 HTTP Listener；用户状态只由本地 `userService` 修改，`dashboardService` 在自己的 Handler 内调用本地用户 Service，并以自身 ServiceID 通过 Cluster 按 `rooms` 名称调用远端房间 Service。测试只增加这一条跨 HTTP、Core 和 Cluster 的业务链路；原有房间接口测试继续保护超时与安全错误响应。

当前示例仍显式组装底层 Runtime，并把本地用户和汇总 ServiceID 交给 HTTP 路由。这能验证业务链路，但不代表 `gsr.Run` 已具备给 HTTP 入口按名称调用的接线能力。路由在启动时构造；将来确有运行期路由变更需求时，整体重建并替换路由表，不为单条路由增加动态增删 API。`net/http` 已负责 HTTP 连接和 Handler 并发，不另建固定 HTTP Agent 池。

`Runtime.Call` 的 Source 固定为零，表示受信任基础设施注入，不冒充某个业务 Service 或 HTTP 客户端。HTTP Handler 的等待期限不强杀已开始执行的 Service Handler；原房间人数接口是只读查询，管理路由中的封禁操作是幂等示范。若以后接入非幂等 webhook，还需由业务定义超时后的结果处理与重试策略。
