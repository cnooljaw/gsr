# Service 内直接调用外部 HTTP

这个例子以“刷新玩家显示名”为业务场景。`profileService` 独占显示名状态；它在处理刷新命令时，直接使用应用注入的标准库 `http.Client` 查询外部资料接口。HTTP 只是一次有超时的依赖调用，不需要额外创建 `HttpService`、worker 或 GSR 公共 HTTP API。

从仓库根目录运行：

```sh
go test ./examples/http-outbound
```

测试中的 `httptest.Server` 充当外部资料接口：刷新成功后更新显示名；超时、HTTP 故障、畸形 JSON 或超限响应体都保留旧值，并返回稳定、安全的领域错误码。之后的有效响应仍可正常刷新。Runtime 不会把 Handler 返回的原始外部错误直接传给调用者，因此示例在业务边界进行错误映射。

HTTP 等待期间，当前 `profileService` 不处理下一条消息，但另一个 Service 仍能响应。调用者的 `Call` 截止时间只限制调用者等待，不会强杀正在执行的 Handler；所以 HTTP Client 自身必须设置有限超时。示例不自动重试，请求是否适合重试由具体业务决定。

测试使用 `Runtime.Call` 作为宿主注入入口；实际业务 Service 之间通信仍使用 `ServiceContext.Send/Call`。注入的 HTTP Client 由应用持有，Service 不关闭其 Transport。
