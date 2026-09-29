# Core 外的只读 HTTP 入口

这个示例模拟第三方系统查询房间人数，不是给游戏客户端增加第二套协议；游戏客户端仍走 WebSocket Gate。HTTP 请求由 Go `net/http` 接入，Handler 通过基础设施入口 `Runtime.Call` 查询本地 `roomService`，收到结果后由 HTTP Handler 写回 JSON。房间状态仍由 Service 独占，HTTP goroutine 不直接读取它。

从仓库根目录运行：

```sh
go test ./examples/http-inbound
```

测试用真实回环 HTTP Server 验证成功、Service 失败的安全错误响应和有界等待。示例只提供 `http.Handler`；监听地址、认证、TLS、限流以及 HTTP Server 的启动和关闭由宿主负责，不因此扩展 Runtime 或 WebSocket Gate。宿主停机时先停止 HTTP 接纳并等待在途 Handler，再关闭 Runtime。生产接入必须先确定第三方调用方身份和网络边界，不能直接把这个无认证的示例公开到公网。

`Runtime.Call` 的 Source 固定为零，表示受信任基础设施注入，不冒充某个业务 Service 或 HTTP 客户端。HTTP Handler 的等待期限不强杀已开始执行的 Service Handler；本例只有只读查询，若以后接入修改状态的 webhook，还需由业务定义幂等、超时后的结果处理和重试策略。
