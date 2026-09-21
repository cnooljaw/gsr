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

`prodgate` 目前实现了首帧握手协议处理、连接密钥派生和上行帧认证，但尚未接入 WebSocket 读写循环、TLS 监听、握手超时或 Gate Service；它不能作为生产 Gate 使用。

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
