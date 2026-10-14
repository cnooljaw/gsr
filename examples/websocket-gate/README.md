# WebSocket Gate 联调示例

本示例默认使用统一 Gate 的 Plain 模式，演示 TypeScript 客户端发送 Protobuf Envelope、GSR Agent 解码业务请求并回传同一 `request_id`。Plain 不提供握手、seq 或 MAC，仅绑定回环/私有 IP，不得暴露到公网。

Go 启动入口只解析参数、处理退出信号并调用 `gsr.Run`。MainService 创建并注册共享的 `echo` Service，再调用 `self.Node().CreateGate` 并明确选择 Protocol，提供返回 `gsr.Service` 的 Echo Agent 构造回调，然后自身退出；EchoService、Gate 与连接 Agent 继续运行，由 Runtime 在进程退出时统一停止。Echo Agent 显式实现普通 `Handle` 和可选的 `HandleClientMessage`，两种消息入口共用该 Service 的 Mailbox 和 runner。

框架通用 Envelope 和 ErrorResponse 定义在仓库根目录的 `protocol/proto/gsr/common/v1/common.proto`；Echo 示例的消息编号和 payload 仍定义在本目录的 `proto/gsr/example/echo/v1/echo.proto`。握手属于 Gate 的固定二进制格式，断线通知属于内部 Core Command，二者都不是这里的 Protobuf 消息。每个消息编号通过 `.proto` option 显式关联 payload、方向和 Request/Notify/Response/Push 种类，`npm run generate` 从仓库根目录的 Buf 配置同时生成 Go 与 TypeScript 类型和 Go 协议目录。业务 Handler 按实现进度手动注册；新协议消息可以先定义，之后逐个补 Handler。

## 运行

组装协议目录时只需 `protocol.NewRegistry(echopb.MessageCatalogGsrExampleEchoV1Echo)`；框架自动纳入通用错误目录，业务不重复传入。通用错误编号 9001 不得被业务占用，冲突在构造时拒绝。

Agent 的强类型 Handler 接收 `self gsr.CommandContext` 和 `ctx protocol.Context`：前者用于调用其它 Service，后者用于当前客户端请求的响应。公共接入写法见 [Agent 中的协议分发](../../protocol/README.md#agent-中的协议分发)。

## Agent 的两个消息入口

请求链路（默认 CLI）：

```text
Client EchoRequest
  → Gate → Agent.HandleClientMessage → OnEcho
  → CallName("echo", echoCommand, text)
  → EchoService.Handle → handleEcho → 返回业务结果
  → Agent ctx.Response → Client EchoResponse（保留 request_id）
```

主动推送链路（CLI `--notify`）：

```text
Client EchoNotify（request_id = 0，不要求响应）
  → Gate → Agent.HandleClientMessage → OnEchoNotify
  → SendName("echo", notifyEchoCommand, text)
  → EchoService.Handle → handleNotifyEcho
  → Send(Source(), pushEchoCommand, text)
  → Agent.Handle → handlePushEcho
  → agent.protocolDispatcher.Push → Client EchoPush（request_id = 0）
```

第二条链路的 Push 是新的主动消息，不是 Notify 的 Response。EchoService 只知道来源 AgentID，不持有 Agent 实例或连接；Agent 的 `Handle` 收到普通 Command 时，没有当前客户端请求的 `protocol.Context`，因此使用协议 Dispatcher 的独立 `Push` 能力，不保存旧请求上下文。

阅读入口：[`main_service.go`](cmd/debugserver/main_service.go) 负责组装；[`agent.go`](cmd/debugserver/agent.go) 展示两个入口，普通 Command 使用 `commandDispatcher`，客户端 Envelope 使用 `protocolDispatcher`，均在构造时提供显式编号到强类型 Handler 的表；[`echo_service.go`](cmd/debugserver/echo_service.go) 展示显式 `Handle`、强类型业务 Handler 和基于来源的 Send。Call 最多等待一秒，期间仅当前 Agent runner 等待，不阻塞其它 Service。空文本的业务拒绝由 EchoService 决定，再由 Agent 转为协议 Response；Send 的成功仅表示接纳，后续失败由框架诊断记录，不保证客户端收到 Push。

Core CommandID 和客户端 msg_id 是不同编号空间，分别由目标 Service 和协议目录解释；不需要一一对应，也不按本地／远端另建命令清单。

## 环境准备与联调

需要 Go 1.24 或更高版本、Node.js 22.15 或更高版本，以及 `protoc-gen-go` v1.36.1 在 `PATH` 中。可用下面的命令安装 Go 插件；Buf CLI 和 TypeScript 生成插件由本目录锁定的 npm 依赖提供。

```bash
cd examples/websocket-gate
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.1
npm ci
npm run generate
```

跨语言认证固定向量使用同一份测试夹具验证 Go 与 TypeScript：

```bash
cd ../..
go test ./gate -run '^TestClientAuthVectorMatchesReference$'
cd examples/websocket-gate
npm run test:crypto
```

固定向量采用 RFC 7748 的公开 X25519 测试密钥；仓库根目录的 `gate/testdata/generate_client_auth_vector.mjs` 可独立重建共享 JSON 夹具。测试私钥仅用于公开测试向量，禁止用于实际连接。

框架包 `gate` 已接入固定 33 字节首帧密钥握手、上行 seq/MAC 校验、认证后向 Agent 投递、每连接有界写队列、连接关闭管理和 Runtime 共享结构化日志。`TLSConfig` 非空时直连 WSS；留空时监听普通 HTTP WebSocket，适用于可信内网或由 K8s Ingress/LB 终止客户端 TLS 的部署。公网直连必须由部署方提供 `TLSConfig`；Gate 无法从监听地址判断是否暴露公网。MAC 的 WS/WSS 部署共用固定二进制握手和业务协议；Plain 不握手，TLS 与模式独立，并有本机集成测试。提升为框架包不等于已经完成安全审计；慢客户端长时压力和完整失败矩阵尚未完成。

K8s 默认部署可以让 Pod 监听 `0.0.0.0:8080`，Gate 不配置 `TLSConfig`；Ingress 持有 TLS Secret，并把 `/ws` 以 HTTP/1.1 WebSocket Upgrade 转发到 Service。Ingress 到 Pod 的网络应属于可信集群边界；如果该链路不可信，需要另外配置 mTLS，不能把应用层 MAC 当作 TLS 身份认证。

Gate 自己终止 TLS 时要求 TLS 1.2 或更新版本，只协商 HTTP/1.1。`GetCertificate` 可以动态选择证书；`GetConfigForClient` 会被拒绝，因为它能替换连接级 TLS 配置并绕过 Gate 的约束。TLS 在 Ingress/LB 终止时，证书和 TLS 版本由该上游组件负责。

生产 Gate 的短时混合连接压力样本默认不运行，避免影响普通测试；需要显式开启并可按秒调整时长：

```bash
GSR_GATE_STRESS=1 GSR_GATE_STRESS_SECONDS=10 go test ../../gate -run '^TestMACGateMixedConnectionStress$' -v
```

测试会输出发送量、Agent 处理量、Mailbox 满载和连接拒绝计数，用于建立机器相关基线，不直接代表最终容量上限。

慢客户端长时出站背压可以单独运行；客户端连接建立后不读取服务端数据，`ClientSession.Send` 预计会稳定返回 `ErrClientQueueFull`，这属于验收目标：

```bash
GSR_GATE_STRESS=1 GSR_GATE_SLOW_CLIENT_SECONDS=10 go test ../../gate -run '^TestMACGateLongSlowClientBackpressure$' -v
```

坏握手和坏 MAC 的失败阶段样本可以重复运行，默认 32 轮：

```bash
GSR_GATE_STRESS=1 GSR_GATE_FAILURE_ATTEMPTS=32 go test ../../gate -run '^TestMACGateFailurePhaseStress$' -v
```

测试会确认失败连接不进入 Agent 业务处理，并记录失败连接和已接纳连接数量。

终端一启动 GSR 服务端，示例默认选择 Gate 的 Plain 模式（框架配置的零值默认仍是 MAC）：

```bash
go run ./cmd/debugserver
```

要在本机验证生产 Gate 的固定握手和上行 MAC，启动生产模式。这里不配置 TLS，用来模拟由 Ingress 终止 TLS 后转发普通 WebSocket：

```bash
go run ./cmd/debugserver --prod
```

终端二运行 TypeScript 客户端：

```bash
npm run client -- --text "hello GSR"
```

服务端默认只监听 `127.0.0.1:7888`。可以通过 `--addr 127.0.0.1:PORT` 指定其它本地端口，并通过客户端 `--url ws://127.0.0.1:PORT/ws` 连接。

TypeScript CLI 默认连接 Plain 模式，只演练 Echo Request/Response 的编解码和 `request_id` 回传。连接 MAC 模式时增加 `--secure`，客户端会先发送固定 33 字节 X25519 握手，再发送 `seq | Envelope | MAC`；该参数启用应用层认证，不会把 WS 自动改为 WSS：

直连配置 TLS 的生产 Gate 使用 `wss://` 地址，例如：

```bash
npm run client -- --secure --url wss://127.0.0.1:PORT/ws --text "hello secure GSR"
```

本地 `--prod` 默认监听 `127.0.0.1:7889`，因此可用下面的地址连接：

```bash
npm run client -- --secure --url ws://127.0.0.1:7889/ws --text "hello secure GSR"
```

空文本会返回业务自己的 `TEXT_REQUIRED` 结果，不会改成通用 `ErrorResponse`；`--notify` 则发送零 `request_id` 的通知并等待服务端 Push：

```bash
npm run client -- --secure --url ws://127.0.0.1:7889/ws --text ""
npm run client -- --secure --notify --url ws://127.0.0.1:7889/ws --text "push me"
```

示例的 `--prod` 默认使用普通 WS，适合本机或 Ingress 终止 TLS 后的内段联调。要用示例直接监听 WSS，显式提供成对的 PEM 证书和私钥文件：

```bash
go run ./cmd/debugserver --prod --addr 127.0.0.1:7889 --tls-cert /path/to/server.crt --tls-key /path/to/server.key
npm run client -- --secure --url wss://127.0.0.1:7889/ws --text "hello WSS"
```

客户端必须信任证书且证书的 SAN 包含连接地址；本机自签名测试证书可通过 `NODE_EXTRA_CA_CERTS=/path/to/server.crt` 交给 Node.js 信任，不要关闭 TLS 验证。此示例只在 `--prod` 下接受证书文件参数；这是示例 CLI 的限制，框架 Gate 的 Plain 模式也支持 TLS。正式部署仍由业务应用决定证书来源和监听配置；示例不提供证书管理或自动续期。
