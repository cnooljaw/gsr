# WebSocket Gate 联调示例

本示例使用仅供本机调试的 DebugGate，演示 TypeScript 客户端发送 Protobuf Envelope、GSR Agent 解码业务请求并回传同一 `request_id`。DebugGate 不提供 TLS、握手、seq 或 MAC，不得暴露到公网。

通用 Envelope 和连接控制消息定义在 `proto/gsr/common/v1/common.proto`；Echo 示例的消息编号和 payload 定义在 `proto/gsr/example/echo/v1/echo.proto`。每个消息编号通过 `.proto` option 显式关联 payload、方向和 Request/Notify/Response/Push 种类，`npm run generate` 同时生成 Go 协议目录。业务 Handler 按实现进度手动注册；新协议消息可以先定义，之后逐个补 Handler。

## 运行

需要 Go 1.23.3 或更高版本、Node.js 22.15 或更高版本，以及 `protoc-gen-go` v1.36.1 在 `PATH` 中。可用下面的命令安装 Go 插件；Buf CLI 和 TypeScript 生成插件由本目录锁定的 npm 依赖提供。

```bash
cd examples/websocket-gate
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.1
npm ci
npm run generate
```

跨语言认证固定向量使用同一份测试夹具验证 Go 与 TypeScript：

```bash
go test ./prodgate
npm run test:crypto
```

固定向量采用 RFC 7748 的公开 X25519 测试密钥；`testdata/generate_client_auth_vector.mjs` 可独立重建共享 JSON 夹具。测试私钥仅用于公开测试向量，禁止用于实际连接。

`prodgate` 已接入首帧密钥握手、上行 seq/MAC 校验、认证后向 Agent 投递、每连接有界写队列、连接关闭管理和 Runtime 共享结构化日志。`TLSConfig` 非空时直连 WSS；留空时由 K8s Ingress 终止客户端 TLS，Pod 接收普通 HTTP WebSocket。两种模式共用应用层握手和业务协议，并有本机集成测试。它仍是示例代码，不是经过完整安全审计的生产产品；慢客户端长时压力和完整失败矩阵尚未完成。

K8s 默认部署可以让 Pod 监听 `0.0.0.0:8080`，Gate 不配置 `TLSConfig`；Ingress 持有 TLS Secret，并把 `/ws` 以 HTTP/1.1 WebSocket Upgrade 转发到 Service。Ingress 到 Pod 的网络应属于可信集群边界；如果该链路不可信，需要另外配置 mTLS，不能把应用层 MAC 当作 TLS 身份认证。

生产 Gate 的短时混合连接压力样本默认不运行，避免影响普通测试；需要显式开启并可按秒调整时长：

```bash
GSR_GATE_STRESS=1 GSR_GATE_STRESS_SECONDS=10 go test ./prodgate -run '^TestProdGateMixedConnectionStress$' -v
```

测试会输出发送量、Agent 处理量、Mailbox 满载和连接拒绝计数，用于建立机器相关基线，不直接代表最终容量上限。

终端一启动 GSR 服务端：

```bash
go run ./cmd/debugserver
```

终端二运行 TypeScript 客户端：

```bash
npm run client -- --text "hello GSR"
```

服务端默认只监听 `127.0.0.1:7888`。可以通过 `--addr 127.0.0.1:PORT` 指定其它本地端口，并通过客户端 `--url ws://127.0.0.1:PORT/ws` 连接。

示例只演练 Echo Request/Response 的编解码和 `request_id` 回传，不代表生产 Gate 的握手与认证已实现。
