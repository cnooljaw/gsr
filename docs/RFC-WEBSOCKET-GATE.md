# WebSocket Gate、客户端协议与 Agent

状态：设计方向已确认；DebugGate 示例已完成 Go/TypeScript Echo 往返，跨语言认证固定向量已完成；生产 Gate 已接入可选直连 WSS 或 Ingress 终止 TLS 后的 HTTP WebSocket、固定二进制握手、上行认证、Agent 投递、连接写回和 Runtime 共享 Logger。单/多慢连接、混合连接与部分失败阶段已有可重复测试；目标部署容量、完整失败矩阵和密码协议审查尚未完成。

## 范围与参考

首版只支持 WebSocket，公网客户端使用 WSS/TLS；生产 Gate 默认允许由 K8s Ingress 终止 TLS 后以 HTTP/1.1 WebSocket 转发到 Pod，也支持显式配置 TLS 证书的直连 WSS。另提供显式开启的内网 DebugGate，以最小业务往返验证接入设计。HTTP 入站/出站、Redis、MySQL 是另一组 Core 外能力，后续分别推进。

Skynet 的 `lualib/snax/gateserver.lua` 通过 socketdriver 消息及 Lua coroutine 管理监听、连接与数据事件；GSR 借鉴职责分离，不照搬 TCP netpack 和 C/Lua 调度。Go I/O 任务必须有明确 owner 管理容量、取消、关闭和等待。

旧 BSPacket 参考 `../baison_middle/protocol/packet.go` 和 `../nbgame_core/transport/newnet/protocols/cpacket.go`。最新选择替代此前固定 12 字节头、DES 及 0x40/0x80 模式设计。当前只推进新客户端、新协议；旧协议识别、分流、代理及兼容均无当前需求，不设计、不实现、不预留接口，等用户明确提出后再裁决。此前讨论的 tag 分流不构成新协议新增 tag 的已确认要求。

## 已确认：线上消息形状

一条完整 WebSocket 二进制消息对应一个应用包，不把 WebSocket 物理分片当作应用消息，不另加长度头。

```text
连接阶段：Client ↔ Gateway，交换 X25519 公钥
          X25519 shared → HKDF → 上行 macKey

生产 Gate 上行：seq（8B） | Protobuf Envelope | MAC（16B）
生产 Gate 下行：Protobuf Envelope
DebugGate 上行/下行：Protobuf Envelope
```

生产 Gate 握手固定为两条 33 字节二进制消息：客户端发送 `version(1B) | client_public_key(32B)`，服务端返回 `version(1B) | server_public_key(32B)`；当前 `version` 固定为 `1`。

直连模式使用 WSS 提供传输保密和完整性；K8s 模式由 Ingress 对客户端提供 WSS，Ingress 到 Pod 使用受信任集群网络上的 HTTP/1.1 WebSocket。两种模式都保留连接级 X25519、上行 seq/MAC 和同一应用协议；应用层不再使用 DES 加密，下行不添加应用 MAC。握手阶段没有 macKey，使用固定 33 字节二进制握手，不套生产业务上行格式。DebugGate 不做 X25519、seq 或 MAC，直接传输 Envelope。

Envelope 采用 Protobuf 的薄包装，只标识具体消息和请求关联，不把业务字段、Core Command 或 ServiceID 放入其中：

```protobuf
message Envelope {
  uint32 msg_id = 1;      // 具体 Protobuf 消息编号
  uint32 request_id = 2;  // 客户端请求关联编号；0 为通知或推送
  bytes payload = 3;      // 对应消息的 Protobuf 数据
}
```

业务 payload 是对应业务消息的 Protobuf。请求、响应、通知和推送各自分配独立的 msg_id；通用协议文件定义 Envelope 和通用错误，业务协议文件定义业务消息编号与 payload。每个非零 MsgId enum 值通过 `message_route` option 显式关联 Protobuf payload 全名、方向和消息种类，生成工具据此产出强类型协议目录；运行时不根据编号范围或命名猜测。Agent 使用目录解码，再按实现进度手动注册强类型 Handler，因此协议可以先定义，Handler 可以逐个完成，不需要为未实现消息放占位函数。Request 必须带非零 request_id，成功处理后必须发送一次 Response；Notify 的 request_id 必须为零，处理完成不回复；Response 和 Push 只能作为服务端下行消息，Push 的 request_id 固定为零。Gate 只负责传输与安全，不判断业务消息类型。不维护节点级 Core Command 准入清单。旧打地鼠协议仅作业务样例，不进入 GSR 通用协议定义。

示例 `.proto` 中的编号和 payload 映射如下：

```protobuf
C2S_ECHO_REQUEST = 1001 [
  (gsr.protocol.v1.message_route) = {
    payload_type: ".gsr.example.echo.v1.EchoRequest"
    direction: DIRECTION_CLIENT_TO_SERVER
    kind: KIND_REQUEST
  }
];
```

该映射由 `protoc-gen-gsr-registry` 生成 Go 协议目录；业务侧只需逐步注册已实现的类型，例如 `protocol.Register(dispatcher, agent.OnEcho)`。Handler 注册按 Protobuf payload 类型定位 msg_id，不需要再复制一份 ID 到业务注册代码。

示例协议表：

```text
1001  EnterRoomRequest    C2S request
1002  EnterRoomResponse   S2C response
2001  MoveNotify          C2S notify
3001  PlayerMovedPush     S2C push
9001  ErrorResponse       S2C error response
```

响应使用独立的 msg_id，但回传原 request_id；通知和推送的 request_id 为零。握手不使用 Protobuf，不占用客户端 `msg_id`，也不进入 `Envelope`：

```text
Client → Server: version(1B) | client_public_key(32B)
Server → Client: version(1B) | server_public_key(32B)
```

当前 `version` 固定为 `1`，两方向总长度固定为 33 字节；长度或版本不正确、X25519 公钥非法时直接拒绝。握手不带 request_id、seq 或 MAC；旧头的 Tag、Length、ver 不进入新协议。

通用协议和业务协议分别维护自己的 MsgId 常量；二者位于不同 Protobuf package：

```protobuf
// gsr/common/v1/common.proto
package gsr.common.v1;

enum MsgId {
  MSG_ID_UNSPECIFIED = 0;
  S2C_ERROR_RESPONSE = 9001 [
    (gsr.protocol.v1.message_route) = {
      payload_type: ".gsr.common.v1.ErrorResponse"
      direction: DIRECTION_SERVER_TO_CLIENT
      kind: KIND_RESPONSE
    }
  ];
}
```

```protobuf
// gsr/example/echo/v1/echo.proto
package gsr.example.echo.v1;

enum MsgId {
  MSG_ID_UNSPECIFIED = 0;
  C2S_ECHO_REQUEST = 1001 [
    (gsr.protocol.v1.message_route) = {
      payload_type: ".gsr.example.echo.v1.EchoRequest"
      direction: DIRECTION_CLIENT_TO_SERVER
      kind: KIND_REQUEST
    }
  ];
  S2C_ECHO_RESPONSE = 1002 [
    (gsr.protocol.v1.message_route) = {
      payload_type: ".gsr.example.echo.v1.EchoResponse"
      direction: DIRECTION_SERVER_TO_CLIENT
      kind: KIND_RESPONSE
    }
  ];
}
```

`Envelope.msg_id` 保持 `uint32`，枚举只提供常量。Agent 的协议分发器根据生成目录选择 Protobuf 类型，并校验方向、消息种类和 request_id；Gate 不解析业务 payload。协议目录由 `.proto` 生成，业务 Handler 仍手动增量注册，二者不要求同时实现。客户端 msg_id 与不同 Service 可复用的内部 `CommandID` 是两套编号。

## 已确认：生产 Gate 与 DebugGate

- 生产 Gate 负责客户端接入：可选择直连 WSS，或接受 Ingress 终止 TLS 后的 HTTP WebSocket；两者都执行 X25519 握手、上行 seq/MAC、容量和连接生命周期。
- DebugGate 只面向受信任内网调试：一条 WebSocket 二进制消息就是一份 Protobuf Envelope，不做 X25519 握手、不附加 seq、不校验 MAC。它与生产 Gate 共用通用 Envelope 和 Agent 业务处理逻辑。
- 两者使用不同监听器、端口和显式配置；生产 Gate 不自动降级到 DebugGate，DebugGate 也不根据首包猜测生产格式。DebugGate 默认关闭，启用时应绑定回环地址或明确的内网地址，并由部署防火墙限制访问。
- DebugGate 没有应用层认证和防重放保证，不能暴露到公网，也不能被当作生产安全模式。业务鉴权仍由 Agent/业务 Service 负责。
- 两个 Gate 共享 Agent 业务处理逻辑；帧接收和安全校验属于各自适配器。Gate 不解码或分发业务 payload。
- 两个接入入口保留各自的监听、握手、帧验证、容量计数和日志行为；相同的 Agent 构造保护、断线重试、停机补发，以及单连接有界出站队列、写循环和关闭机制放在 `gate/internal/gatecommon`，不以模式开关合并成一个 Gate。

`gate/debuggate` 已实现本地示例闭环：`NewGateService` 显式接收底层 `*runtime.Runtime`，应用通过 `Runtime.CreateService` 启动它；这是底层示例组装方式；应用层现已通过 `self.OpenDebugGate` 接入 `gsr.Run`，不扩展 Core 或向业务暴露 Runtime。Gate 只监听字面回环/私有 IP 的 `/ws`，每个连接由 Gate runner 调用 `NewAgent` 创建一个 Agent。创建返回的 AgentID 绑定到该连接，读循环直接调用 `Runtime.DeliverClientMessage` 投递不透明 Envelope 字节，不逐帧经过 Gate Mailbox；Gate runner 中的连接到 Agent 映射只用于断开通知。`ClientSession.Send(Envelope)` 将响应放入有界队列，再由唯一写循环发回。此处 `ClientSession` 表示客户端连接的写回能力，不是 Core `SessionID` 或 Envelope 的 `request_id`。DebugGate 显式限制连接数、帧大小和每连接出站队列，默认帧上限为 4 MiB。出站队列按序列化 Envelope 字节数加每项固定 64 字节队列开销计费，避免空帧或很多小帧绕过内存上限；正在写出的单帧另受 `MaxFrameBytes` 限制。集成测试覆盖请求/响应 request_id、连接上限、正常断开通知、Agent 创建期间既有连接继续收帧和 Runtime 关闭时连接回收。`examples/websocket-gate` 另提供独立 Echo Agent、Go DebugGate 服务端和 TypeScript CLI；通用 Envelope 与 Echo 业务消息分别定义在 `.proto` 文件中，Go 与 TypeScript 使用同一组协议源生成类型并完成真实请求/响应往返。Go Agent 从生成目录解码 Envelope，再通过手动注册的强类型 Echo Handler 响应；未实现的协议消息无需占位 Handler。Echo 示例不包含业务生产鉴权；DebugGate 没有 TLS、生产握手或 MAC，不能用于公网。它借用 Runtime 的共享 Logger，以 Gate ServiceID 记录监听、连接接纳、Agent 创建、帧读写和投递故障；不记录原始载荷或业务 Envelope 字段，也不负责关闭 Logger。调试入口不复制生产 Gate 的握手/MAC 诊断和按连接限频机制。

DebugGate 的 `MaxConnections` 统计已进入 `/ws` GET Handler 的连接，不限制原始 TCP 接纳数或其它 HTTP 路径；它仅用于受信任环境调试。普通 HTTP 请求头与未升级连接的 keep-alive 空闲等待各限 5 秒，避免无期限保留这类 socket。

TypeScript CLI 默认连接 DebugGate；增加 `--secure` 后，它会发送固定 33 字节的 X25519 握手，并按 `seq | Envelope | MAC` 发送第一条上行消息，可用于验证生产 Gate 的客户端认证路径。示例 `cmd/debugserver --prod` 可在本机启动不带 TLS 的生产 Gate，模拟 Ingress 终止 TLS；真实部署的监听地址、TLS 和部署配置仍由业务服务组装。

### 当前归属：Gate 与协议进入框架

通用 Envelope、协议目录与分发位于 `gate/protocol`，ProdGate 和 DebugGate 位于 `gate/prodgate`、`gate/debuggate`，共同的连接细节属于 `gate/internal/gatecommon`。通用 `.proto`、生成工具与跨语言认证夹具也由 `gate/` 持有。`examples/websocket-gate/` 保留 Echo 业务协议、Agent、Go 启动入口和 TypeScript 客户端，并从框架包导入通用能力。提升包归属不代表完成安全审计；DebugGate 已有 `gsr.Run` 应用组装入口，当前 Echo 示例仍保留底层 Runtime 组装；ProdGate 的应用组装仍待实现，业务 `ServiceContext` 不获得客户端消息投递权限。

## 已确认：业务回复、通用错误与推送

| 情况 | msg_id | request_id | payload |
| --- | --- | --- | --- |
| 业务结果，包括业务成功与业务拒绝 | 独立的 Response 编号 | 原请求编号 | 对应业务 Response |
| 调用未取得正常业务结果 | ErrorResponse 编号 | 原请求编号 | ErrorResponse |
| 服务端主动推送 | Push 编号 | 0 | 对应业务 Push |

```protobuf
enum ErrorCode {
  ERROR_CODE_UNSPECIFIED = 0;
  ERROR_CODE_INVALID_REQUEST = 1;
  ERROR_CODE_UNSUPPORTED_MESSAGE = 2;
  ERROR_CODE_INTERNAL = 3;
}

message ErrorResponse {
  ErrorCode code = 1; // 稳定通用错误码，供程序判断
  string message = 2; // 安全说明，不包含内部堆栈
}
```

消息文本不参与程序分支，也不包含 Handler 的内部错误内容。

- 业务消息的 msg_id 直接决定 payload 的 Protobuf 类型，不通过尝试解析 payload 猜类型。
- 业务结果包括业务成功与业务拒绝。余额不足、道具不足、当前状态不允许等业务拒绝，与业务成功一样返回对应业务 Response，用业务自己的 code 或结果字段区分。
- 例如扣款余额不足，返回 DeductResponse 的独立 msg_id、原 request_id 和业务码 INSUFFICIENT_FUNDS；业务 Handler 返回响应数据，不把预期业务拒绝作为普通 Go error 返回。
- ErrorResponse 表示本次调用未能取得正常业务结果。目标服务不可用、调用超时、无法识别消息或内部异常等使用 ErrorResponse 的独立 msg_id；客户端按 msg_id 选择错误解码。
- 超时只说明未及时取得结果，不能据此判断业务没有执行、扣款没有发生或状态已回滚；是否重试及如何防止重复操作由业务决定，不自动重放请求。
- Gate 只限制握手、网络写入和连接资源，不为每个客户端业务 Request 设置统一超时，不代发超时 `ErrorResponse`，也不声称能取消已经进入 Agent 的 Handler。
- 客户端自行决定等待某个 `request_id` 的期限；超时后移除本地等待，保留连接时按 `request_id` 丢弃迟到响应，不复用该编号。Agent/业务 Service 根据操作语义为内部 `Call` 选择有限等待；若内部等待失败，由 Agent 按既有协议返回关联的 `ErrorResponse`。
- request_id=0 的通知处理失败由服务端记录，不自动生成请求回复；推送使用独立的 Push msg_id。
- 连接断开时无法保证发送错误回复，由客户端连接事件结束等待。
- 程序分支依赖稳定 code，不比较 message；不得直接透传内部 error 文本或堆栈。
- 错误语义已经确认：业务拒绝使用业务 Response，无法取得正常结果时使用 ErrorResponse 并复用原 request_id；通用 `ErrorCode` 只表达无效请求、未支持消息和内部失败，业务响应自己的业务码由业务协议定义，GSR 不建立跨业务领域码表。固定二进制握手格式和阶段门禁由 Gate 定义，业务消息编号和 payload 由业务协议定义，Agent 负责按 msg_id 处理。

## 已确认：C/S request_id 与 seq

- Envelope 中 request_id 使用 uint32；0 表示通知或服务端主动推送，非零表示客户端 Request。成功或失败响应原样带回请求 request_id。
- 每连接从 1 单调分配，不回绕、不复用；超时移除等待但不回收编号，迟到响应丢弃。耗尽时通过正常重连建立新的编号空间，不自动重放旧请求。
- 新连接可重新从 1 开始，旧连接等待不能迁移到新连接。按每秒 10 次 Request，编号约 4971 天（13.6 年）耗尽；通知和推送不消耗 request_id。
- C/S request_id 与 Core SessionID、ClusterSessionID 分离；Agent 以连接和 request_id 关联客户端等待，不把它当身份凭据或业务幂等键。
- seq 是独立的 uint64 连接序号，所有上行业务包均消耗，包括 request_id=0 的通知。单连接从 1 单调递增，不回绕；到达最大值后客户端关闭并重新握手，Gateway 接受 seq > lastSeq。这个规则额外拒绝同连接重复旧包。
- seq 在线上使用 8 字节大端序。Laya/TypeScript 实现不得用普通 number 表示完整 uint64，应采用 BigInt 或等效无损表示；uint32 request_id 可以精确表示。

## 生产 Gate 的连接认证与接收边界

目标是让连接 A 录制的已认证上行包，不能拿到另一条连接 B 直接复用；seq 额外拒绝同连接的重复旧包。不承诺阻止控制客户端、提取密钥后重新计算 MAC 或重新构造业务请求，也不等于登录鉴权或业务幂等。

已确认方向及实施约束：

- 每连接由客户端和服务端分别使用 CSPRNG 生成一对新的 X25519 临时密钥。私钥为 32 字节，仅在本端内存中保存；公钥为原始 32 字节，分别放入握手请求和响应。私钥、shared secret 和 macKey 不发送、不写日志，连接结束后废弃；不按账号或跨连接复用。
- 双方分别计算 `X25519(client_private_key, server_public_key)` 和 `X25519(server_private_key, client_public_key)`，结果必须一致；拒绝全零 shared secret。不要手工修改私钥位或自行实现曲线运算，交给标准 X25519 实现处理。
- 直连模式的握手可信性依赖客户端正确验证服务端身份的 WSS；Ingress 模式则依赖客户端验证 Ingress 的 WSS，以及 Ingress 到 Pod 的受信任集群网络。无额外身份认证的 X25519 不承诺抵御能替换公钥的本机主动代理；若 Ingress 到 Pod 的网络不可信，应另行配置 mTLS，不能把应用层 MAC 当作服务端身份认证。
- 同连接串行执行：检查最小长度 → 读取大端 seq → 使用原始 Envelope 字节校验 MAC → 检查 seq 大于 lastSeq 并立即更新 → 将原始 Envelope 字节投递给 Agent。Agent 负责 Protobuf 解码以及校验 Envelope 结构、msg_id、方向、request_id 和 payload。认证失败不能推进 lastSeq；认证成功但消息未知或不合法时也消费该 seq。投递失败不能回退，序号接纳不代表业务执行成功。
- 协商前只接受固定 33 字节的 `version(1) | client_public_key(32)`；服务端只回复固定 33 字节的 `version(1) | server_public_key(32)`。完成后 Gate 只接受带 seq/MAC 的业务上行，拒绝重复握手、错误方向和无 MAC 降级。MAC 校验通过后，Agent 根据 Envelope 的 msg_id 选择业务处理逻辑并解码 payload；未知 msg_id 不因“未知编号”本身关闭连接。
- 握手失败直接关闭连接，不进入 Agent，也不发送业务 ErrorResponse；握手成功后双方才启用派生的 macKey。
- 必须设置消息大小、握手等待和接纳容量上限；具体初始值与零值语义见“已确认：Gate 容量与关闭顺序”，Protobuf 不意味着无限消息大小。

当前实现位于 `gate/prodgate/`：Gate 在 HTTP/WebSocket Handler 前限制已接纳 TCP 连接数；HTTP 请求头和普通 keep-alive 空闲连接有 5 秒超时，避免未升级连接长期占用名额。`TLSConfig` 非空时启动直连 WSS Listener，为 nil 时启动普通 HTTP WebSocket Listener。首个二进制 WebSocket 消息必须是固定 33 字节握手，完成 X25519 握手后，逐帧校验 MAC 和递增 seq，再将原始 Envelope 字节直接交给目标 Agent 的 `Runtime.DeliverClientMessage`。Agent 的响应通过每连接有界队列进入唯一 WebSocket 写循环。生产 Gate 从 Runtime 借用同一 Logger 并绑定自身 ServiceID；同一连接同一错误类别只记录首条，关闭时补充抑制数量，不记录密钥、MAC 或原始载荷。生产 Gate 与 DebugGate 仍使用不同监听器和显式配置，不会根据首包自动降级。端到端测试覆盖直连 TLS、Ingress 终止 TLS 后的普通 WebSocket、握手与业务帧往返、错误 MAC 拒绝、首帧超时、握手前连接上限与断开后名额回收、Mailbox/出站队列满载、基础慢客户端背压、短时多连接并发及 Runtime 关闭回收；这不替代密码协议审查或慢客户端长时压力验收。

已确认的 TLS 部署边界：TLS 是部署能力，不是 ProdGate 对所有部署的强制要求。K8s Ingress/LB 已终止 TLS 时，以及可信内网部署时，`TLSConfig` 可以为 nil，Gate 监听 HTTP/WS；公网直连时部署方必须提供 `TLSConfig`，由 Gate 监听 HTTPS/WSS。Gate 无法仅凭监听地址判断是否暴露公网，部署配置必须保证这一要求，不能把 `0.0.0.0` 等同于公网直连。Cluster/业务协议不因此改变。

只有在 ProdGate 自己终止 TLS 时，Gate 才克隆并约束 `tls.Config`：最低 TLS 1.2，只协商 HTTP/1.1，拒绝可替换整份连接级配置并绕过约束的 `GetConfigForClient`；允许 `GetCertificate` 动态选证书。一次证书选择返回错误或 panic 只终止当前 TLS 握手，不应阻断后续健康连接；Gate 日志不转录回调的原始错误或 panic 文本。不在 Gate 内新增统一证书管理、TLS 模式或公网暴露判断。Ingress/LB 的 TLS 版本与证书由部署方管理，Ingress 到 Pod 的链路必须位于可信网络边界，不可信时由部署方另行配置 mTLS。

生产 Gate 与 DebugGate 的 WebSocket 升级均允许任意 `Origin`，不使用域名白名单，也不把 `Origin` 当作客户端身份或权限依据；原生客户端、小程序和 Web 客户端使用同一接入语义。直连或 Ingress 的 WSS、X25519、HMAC 和 seq 分别保护传输、连接密钥、上行帧完整性及顺序，但不等于玩家登录鉴权。业务 Agent 必须显式校验 token 后才赋予玩家权限；当前 Echo 示例尚未实现这项业务鉴权。若将来让浏览器自动携带的 Cookie 直接赋予连接权限，必须重新审查跨站 WebSocket 劫持风险，不能用上行 MAC 替代身份校验。

已确认的密码参数：

```text
salt_input = ASCII("gsr/wss/hkdf-salt/v1")
              || client_public_key
              || server_public_key
salt = SHA-256(salt_input)

info = ASCII("gsr/wss/c2s-hmac-sha256/v1")

prk    = HKDF-Extract-SHA256(salt, shared_secret)
macKey = HKDF-Expand-SHA256(prk, info, 32)
```

- 公钥按客户端、服务端顺序拼接，使用原始 32 字节；不使用 Base64、十六进制或 Protobuf 编码后的握手包。
- `salt` 可以公开；`info` 用于协议、方向和用途隔离。`v1` 是固定域分离标签，不是客户端可选择的版本字段。
- 不协商算法，不把 `seq`、`request_id` 或业务数据放入 KDF。未来如果增加服务端到客户端的应用 MAC，必须使用不同的 `info` 派生独立密钥。
- MAC 使用 HMAC-SHA256 的前 16 字节，覆盖大端 seq 原始 8 字节和 Envelope 原始 Protobuf 字节，不包含标签自身。使用常量时间比较，必须在 Protobuf 解码前校验，不能重新序列化 Envelope 后验 MAC；因此不要求 deterministic serialization。

依据：[X25519](https://www.rfc-editor.org/rfc/rfc7748.html)、[HKDF](https://www.rfc-editor.org/rfc/rfc5869.html)、[HMAC](https://www.rfc-editor.org/rfc/rfc2104.html)。固定 33 字节握手格式和组合参数已固定，并由服务端握手测试覆盖；这不等于组合协议已通过独立安全审计。

## 已确认：跨语言固定向量

Go 与 TypeScript 必须使用同一组独立生成的测试向量验证客户端上行认证，不以“两个实现互相算出相同结果”代替参考结果。固定向量只用于测试，里面的私钥是公开的测试常量，不得被生产连接复用。

共享夹具位于 `gate/testdata/client-auth-vector.json`。其中 X25519 私钥、公钥和 shared secret 取自 [RFC 7748 §6.1](https://www.rfc-editor.org/rfc/rfc7748.html#section-6.1) 的 Alice/Bob 测试向量；单独的 `gate/testdata/generate_client_auth_vector.mjs` 校验这些公开 X25519 结果，并生成 salt、HKDF 和 MAC 的逐阶段参考字节。Go 与 TypeScript 测试各自从同一 JSON 读取输入及期望值，不在测试运行时调用生成器。

共享向量使用 JSON 或等价的文本字段承载十六进制字节，至少包含：

```text
client_private_key
server_private_key
client_public_key
server_public_key
shared_secret
salt_input
salt
hkdf_prk
mac_key
seq_bytes
envelope_bytes
mac
```

向量生成和断言必须满足：

- 私钥、公钥、shared secret、salt、HKDF 输出和 MAC 均按原始字节比较，不比较 Base64、PEM、DER 或 Protobuf 握手包的文本表示。
- `salt_input` 严格为固定 ASCII 标签与客户端公钥、服务端公钥的原始字节拼接；不能交换公钥顺序，不能把 Envelope 编码放进 salt。
- `seq_bytes` 必须是固定 `uint64` 的 8 字节大端表示；测试至少包含高位非零的序号，避免实现误用小端或 JavaScript `number`。
- `envelope_bytes` 是参与 MAC 的原始 Protobuf 字节；验证器先对这些原始字节验 MAC，再交给 Protobuf 解码，不允许先解码再重新序列化替代输入。
- MAC 只取 HMAC-SHA256 前 16 字节；测试覆盖正确 MAC、修改 seq、修改 Envelope、修改 `info`、交换公钥顺序和错误截断长度。
- Go 和 TypeScript 的测试都直接读取同一份向量，生产实现不依赖测试向量文件；向量生成脚本与运行时实现分开，避免同一错误同时生成“期望值”和实际值。
- 生产侧 Go 认证代码位于 `gate/prodgate/`：`security.go` 实现逐连接密钥派生和上行帧认证，`handshake.go` 校验固定 33 字节握手、生成服务端临时密钥、返回固定格式公钥并建立连接认证器；`gate.go` 已将握手接入 5 秒超时、TLS Listener、容量边界和 Gate Service 读写循环。使用 `go test ./gate/prodgate` 和示例目录下的 `npm run test:crypto` 分别验证 Go 与 TypeScript；重新生成夹具使用 `node gate/testdata/generate_client_auth_vector.mjs`。

## TypeScript Protobuf 生成方案

命令行客户端使用 Protobuf-ES：`.proto` 是 Go 服务端与 TypeScript 客户端共同的唯一协议源，由 Buf CLI 的单个 `buf generate` 配置分别调用本地 `protoc-gen-go` 和 `protoc-gen-es`。TypeScript 使用 `@bufbuild/protobuf` 运行时和 `protoc-gen-es` 生成的类型化消息，不在客户端手写二进制编码或运行时反射解析 `.proto`。

- `buf.yaml` 与 `buf.gen.yaml` 放在仓库根目录；输入包括框架 `gate/proto/` 的通用协议和示例 `examples/websocket-gate/proto/` 的 Echo 协议。
- 通用 Go 消息生成到 `gate/gen/go/`，Echo Go 消息生成到示例的 `gen/go/`，TypeScript 消息仍生成到示例的 `client/src/gen/`，两侧均从同一 schema 生成。
- Buf CLI、`protoc-gen-es` 和 `@bufbuild/protobuf` 版本由示例 `package-lock.json` 固定；`protoc-gen-go` 与 Go Protobuf 运行时版本由 Go module/安装说明固定。使用本地插件，生成不依赖远程插件服务。
- 生成源文件纳入版本控制，便于克隆后直接运行；改动 `.proto` 后通过单条生成命令更新两侧产物，并验证工作树没有遗漏的生成差异。
- 固定向量验证仍使用已记录的原始字节，不依赖 Protobuf-ES 和 Go 生成器重新编码成相同字节；这样 MAC 契约仍以原始输入字节为准。

## 已确认：WebSocket 关闭码与错误归属

Gate 只使用标准 WebSocket 关闭码，不新增当前客户端必须理解的私有 4000–4999 关闭码。关闭原因是简短、稳定的通用标记，不暴露密钥、MAC、内部错误文本、堆栈或未认证载荷；具体错误写入服务端结构化日志。关闭控制帧的原因文本还必须满足 WebSocket 控制帧长度限制。

| 场景 | 关闭码 | 客户端可见原因 |
| --- | ---: | --- |
| 正常关闭 | `1000` | `normal` |
| Runtime 停止、服务下线或维护 | `1001` | `going_away` |
| 非法消息阶段、重复握手、方向错误、Envelope/帧格式错误 | `1002` | `protocol_error` |
| 收到不支持的数据类型，例如文本消息 | `1003` | `unsupported_data` |
| MAC 错误、序号重放、生产/调试模式不匹配或其它安全策略拒绝 | `1008` | `policy_violation` |
| 首个密钥交换消息超时 | `1008` | `policy_violation` |
| 消息或 payload 超过配置上限 | `1009` | `message_too_big` |
| Gate 自身发生未预期的内部故障 | `1011` | `server_error` |

`1006`（异常断开）和 `1015`（TLS 握手失败）只能作为本地观察结果记录，不能主动放入 Close frame。当前不定义自有关闭码；如果将来确实需要客户端区分新的协议状态，再单独裁决版本兼容和 4000–4999 私有码。

错误归属按处理边界划分：

- **Gate** 负责 WebSocket、握手、帧大小、seq、MAC、模式和关闭；这些失败不进入 Agent，也不发送业务 `ErrorResponse`。
- **Agent** 负责已认证 Envelope 的 `msg_id`、`request_id`、payload 解码、方向和路由。未知 `msg_id` 或合法载荷的协议不支持，有 request_id 时返回通用 `ErrorResponse`，无 request_id 时丢弃并记录；不把未知编号本身当作 WebSocket 坏帧。
- **业务 Service** 负责余额不足、房间不存在、状态不允许等业务拒绝，按对应业务 Response 返回，不改用 WebSocket Close。

生产 Gate 的网络/安全错误（包括客户端出站写循环和底层写 deadline 设置失败）使用 Runtime 共享 Logger，按单连接、错误类别限频：保留首条结构化明细，重复项只累计数量，连接结束时补一条汇总。日志可包含连接 ID、Gate 模式、阶段、远端地址、关闭码和内部错误分类；只有在完成认证和解码后才记录可信的 msg_id/request_id。对端 WebSocket Close 帧只记录关闭码，不记录其可由客户端填写的原因文本。不得记录私钥、shared secret、macKey、原始 payload 或未认证 Envelope 字段。业务日志不做这类去重，也不由 Gate 重复记录。

在 HTTP/WebSocket 升级前，`net/http` 的 TLS 握手、Accept 和内部 Handler 错误尚无 `ClientSession`，无法使用连接 ID 与连接级去重。生产 Gate 和 DebugGate 共用 `http.Server.ErrorLog` 的安全分类入口：配置 Runtime Logger 时写入同一有界日志队列，未配置时仅向 Go 标准日志输出类别（默认目标为标准错误输出）；不转录可能包含未认证输入或 panic 文本的原始行。此阶段的日志不保证每条都成功落盘，也不伪造已认证的客户端身份。

## 已确认：Gate、Agent 与业务 Service

客户端 ↔ WebSocket Gate ↔ Agent Service ↔ 业务 Service。

- Gate 管连接、收发、上行认证和容量，剥离安全封装后交给 Agent；I/O 任务不得直接修改业务状态。
- `AgentService` 是普通 Service，原则上一个连接对应一个 AgentService，负责解释客户端命令与数据并中转到对应业务 Service。客户端不能指定任意内部 ServiceID 或伪造 Source。
- Runtime 内部消息类型区分客户端消息与普通 Core Command，Service 通过实现可选的 `runtime.ClientMessageHandler` 明确启用客户端处理能力。Agent 有两类逻辑入站分发，不能合并成靠 `Source` 猜测的单一路由：Gate 基础设施经 `Runtime.DeliverClientMessage` 投递的原始 Protobuf Envelope 字节按 `msg_id` 交给 `Requests`；普通本地 Service/Cluster Command 按 Core `CommandID` 分发到 `Commands`。两类 handler 属于同一个 AgentService、共用同一 Mailbox 和串行 runner，因此能安全访问同一连接私有状态；`Source` 仍只表达 Service 来源，基础设施投递的客户端消息仍为零。客户端 `msg_id`、客户端 `request_id` 和 Core `CommandID`/Session 不互相复用。Core Command 的 Send 与 Call 仍走同一种消息类型，由 Session 是否为零区分。
- 应用层 Agent 可额外实现 `gsr.ClientMessageHandler`；只有它被适配为 Core 的可选 `runtime.ClientMessageHandler`，普通 `gsr.Service` 不因此接纳客户端消息。两者共用原有 Mailbox 和 runner。`self.OpenDebugGate` 已能将业务 `gsr.Service` Agent 经现有适配器交给 DebugGate；底层 Gate 的 `NewAgent` 仍返回 `runtime.Service`，ProdGate 的应用入口未接线。
- Gate 的业务组装只提供一个 `NewAgent func(ClientSession) runtime.Service` 构造回调，不增加独立的 `AgentFactory` 类型、Agent 管理器或通用依赖注入容器。Gate 为每条连接调用回调构造独立 Agent 实例；回调只负责组装连接私有 Service，不执行 socket I/O，不持有 Runtime 或 Gate 连接表。
- Agent 使用现有 Send/Call；内部 Handler 通过 return 完成 Reply，客户端响应由 Agent 组织并交 Gate 发出。不增加 Core 手工 Reply。
- Agent 内 Call 仍是 run-to-completion，等待时该 Agent 不处理下一条消息，其它 Service 仍可运行。
- Gate 仍是连接 owner：持有 WebSocket、握手状态、读写队列和关闭权；Agent 不接触 `net.Conn` 或 Gate 内部连接表。Gate 完成安全接纳后，把已认证 Envelope 投递给对应 Agent；Agent 通过受限的客户端会话能力发送完整的 Envelope。
- `AgentService` 组合通用客户端会话、request_id 关联、出站 Envelope 组织和断开清理能力，并持有一个具体的 `ProtocolHandler` 组件；`ProtocolHandler` 不是 Service，没有自己的 ServiceID、Mailbox 或生命周期。
- 例如查询房间人数时，`GameProtocol` 解码 `GetRoomCountRequest`，在当前 Agent Handler 内通过 `self.Call` 调用 `RoomService`，得到结果后返回 `GetRoomCountResponse`；`AgentService` 使用原 request_id 编码并交给客户端会话发送。客户端响应能力不是 Core 的手工 Reply，也不完成 Core Session。
- Gate 到 Agent 的入站路径使用受信任的 Runtime-owned 单向投递：Gate 完成 WebSocket、握手、MAC 和 seq 校验后，通过 `Runtime.DeliverClientMessage(target, envelope)` 把完整 Protobuf Envelope 字节投递到 AgentService Mailbox；Runtime 在接纳时复制快照，Source、Session 和 Core CommandID 固定为零。目标 Service 必须实现 `runtime.ClientMessageHandler`，否则在 Mailbox admission 前立即返回 `runtime.ErrClientMessageUnsupported`，不入队、不静默丢弃。该类型不属于客户端 `msg_id`、不作为业务 Service 间的 Call/Send，也不作为公开 Gate API。Agent 收到后由协议目录解码，再分发给业务手动注册的强类型 Request/Notify Handler；普通 `Commands` 仍只处理 Core `CommandID`。
- Gate 的 accept/reader goroutine 不保存或调用 `ServiceContext`。新连接和断开事件投递为 GateService 内部 Command，由其 runner 创建 Agent、管理连接到 Agent 的断开映射；创建成功后，读循环使用返回的 AgentID 直接调用 `Runtime.DeliverClientMessage`，不为每帧额外经过 Gate Mailbox/Core Command。Runtime 在 Agent Mailbox 接纳时复制 Envelope 字节。Agent 构造或 `OnStart` 失败时只关闭当前客户端连接并记录原因，不影响 Gate、Runtime 或其它 Agent。
- Agent 到 Gate 的出站路径使用绑定当前连接的 `ClientSession` 能力，最小接口只有 `Send(Envelope)` 和 `Close`。这里的 `ClientSession` 是客户端连接能力，不是 Runtime `SessionID` 或 Envelope `request_id`。Agent/ProtocolHandler 在调用前负责 `msg_id`、`request_id`、payload 类型以及 Response/Push 方向的协议校验；Response 通过复制原请求的 `request_id` 构造，Push 使用 `request_id == 0`，这些语义不由 Gate 判断，也不重复作为方法参数传递。ClientSession 的 `Send` 不改写或推断 Envelope，只负责检查连接状态、出站大小限制并把完整 Envelope 放入 Gate 的有界写队列；写入 socket 仍由 Gate 的写循环完成。写队列接纳成功不代表客户端已经收到数据。
- `ClientSession.Close` 是幂等的；对端已经断开时底层的 EOF、broken pipe、connection reset，以及 WebSocket 已发出 Close 帧后的 `ErrCloseSent`，都视为关闭完成，不把正常断开升级为 Agent 或 Runtime 停止错误。其它真实 Close 帧写入或 socket 关闭失败均按错误返回；Gate 停止会聚合这些连接关闭错误，同时继续关闭其它连接并等待自身 I/O owner 退出。
- Gate 与 Agent 的连接通知使用单向内部事件；断开时 Gate 先使 ClientSession 失效，再最多通知 Agent 一次，不等待或强杀 Agent。Agent 与 RoomService、PlayerService 等业务 Service 才使用普通 ServiceContext.Send/Call；业务协议代码不需要知道上述 Gate/Agent 连接细节。
- 具体 `GameProtocol`、`KickProtocol` 等协议组件由业务项目实现，只实现自己的 msg_id、Protobuf 类型、请求校验、简单收发和业务 Service 路由，不了解 WebSocket、seq、MAC、关闭码或 Runtime 内部调度。Agent/协议组件可以持有连接私有的少量会话状态，例如 playerID、登录状态、roomID、seatIndex、客户端版本和临时流程状态；但不拥有跨连接共享、需要恢复或需要持久化的房间、玩家资产、战斗等权威业务状态，也不承担复杂业务流程。后者应由业务 Service 负责，Agent 使用 Send/Call 访问，不把两个 Service 嵌套成一个运行单元。
- Agent 内的会话状态只由该 Agent 串行读写；roomID、playerID 等引用可以作为当前连接的缓存，但不能被当作业务事实来源。断线或重连后，需要从拥有状态的业务 Service 重新确认。
- 业务项目按自己的业务边界组织协议，例如 `CommonProtocol`、`LobbyProtocol` 和具体游戏协议；每个模块自带消息编号、类型绑定和协议测试。GSR 不内置这些协议、Protobuf schema、msg_id 清单或业务路由，只提供 Gate/AgentService 的通用接入边界。业务侧的 `ProtocolDispatcher` 只按 msg_id 选择模块，不承载业务规则；小模块可以直接使用局部 switch，不为每条消息建立额外接口。
- 不建立继承式或不断增长的公开 `BaseAgent`。Go embedding 只有在不会泄漏内部生命周期和方法集合时才可作为局部实现细节；具体 `ProtocolHandler` 接口在出现真实实现和测试边界后再定，不提前扩张 Core API。
- 正常断开时 Gate 停止接纳新消息、使客户端会话能力失效并只通知 Agent 一次；Agent 完成自己的清理后退出。Gate 不等待或强杀阻塞的 Agent，Agent 退出也不关闭 Runtime 或其它 Agent。
- Core Message、ServiceID、Cluster wire 不变；客户端 Envelope、seq、macKey 不进入 Core 或 Cluster 协议。

## 已确认：Gate 容量与关闭顺序

Gate 只设置四类资源上限：最大连接数、握手最长等待、单条 Envelope 最大帧大小和每连接出站队列容量。入站不再增加另一套无界 Gate 队列，已绑定 Agent 的消息直接受其 Runtime Mailbox 容量约束；Mailbox 满载时 Gate 停止当前连接的入站接纳并关闭连接，不无限等待或缓存。

初始容量口径如下：

- 握手最长等待默认 `5s`。
- `MaxFrameBytes` 默认 `4 MiB`，按完整 WebSocket 二进制消息计算；生产消息包含 8 字节 seq、Envelope 和 16 字节 MAC，DebugGate 消息包含完整 Envelope。
- 生产 Gate 的 `MaxFrameBytes` 不得小于固定 33 字节密钥握手帧；更小的值会使所有生产客户端握手都无法成功，因此在创建 Gate 时直接拒绝。
- `MaxConnections` 必须由每个部署显式配置为正数；零值不表示无限连接，GSR 不替所有部署猜一个默认连接数。
- Gate 的普通 HTTP 请求头和非 WebSocket keep-alive 空闲连接都受 5 秒超时约束，避免访问其它路径的 HTTP 客户端长期占用连接名额；升级后的 WebSocket 连接不受 HTTP idle timeout 影响。
- 出站队列按编码后应用消息的字节数及每项固定队列开销计费，而不是按消息条数限制；慢客户端不能仅靠大量小消息或空消息绕过内存上限。DebugGate 示例当前固定收取 64 字节条目开销，部署需配置至少 64 字节队列容量。
- 入站继续复用 Agent 的 Runtime Mailbox 容量。达到上限时不建立第二个无限队列，当前连接停止接纳并关闭；出站队列满只返回稳定的 `ErrClientQueueFull`。
- 慢客户端的写循环受 `defaultWriteWait` 限制；底层写超时或 I/O 失败时只关闭当前连接，后续 Agent 写回得到 `ErrClientSessionClosed`。这属于连接资源隔离，不把已关闭连接误报成业务 Handler 错误，也不改变其它连接。

容量边界不生成业务 `ErrorResponse`。生产 Gate 累计 accepted、rejected、handshake timeout、frame too large、inbound mailbox full 和 outbound queue full 六类计数，并通过 Runtime 共享 Logger 记录安全/容量失败；同一连接同一类别只保留首条明细，连接结束时记录抑制汇总。DebugGate 没有应用层握手阶段，因此不产生握手超时计数。日志不得记录原始载荷、密钥或未认证字段。

Gate 停止按以下顺序执行：

```text
停止 Listener 和新连接接纳
        ↓
停止新客户端消息接纳
        ↓
使所有 ClientSession 失效
        ↓
关闭 WebSocket，解除 reader/writer 阻塞
        ↓
等待 Gate 自己的 I/O owner 退出
        ↓
通知仍有映射的 Agent（只等 Mailbox 接纳，不等 Handler 执行）
        ↓
清理连接映射并记录未发送的出站队列统计
```

Gate 不等待或强杀 Agent Handler。单独停止 Gate 时，Gate 在自己的 `OnStop` 上下文期限内重试断开通知，直到 Agent Mailbox 接纳或停止上下文结束；每轮都会尝试所有待通知 Agent，单个满载 Mailbox 不阻塞其他 Agent。整个 Runtime 正在停止时，Runtime 的停止错误视为终止条件。通知只保证入队，不等待 Agent 处理完成。ClientSession 的失效和断开通知必须幂等，避免 Gate 与 Agent 并发停止形成相互等待。容量拒绝和关闭不生成业务 `ErrorResponse`，具体原因进入 Gate 结构化日志。

断开通知通过 Gate 的受控 Infrastructure Call 投递，使 Gate 能确认 Agent Mailbox 是否接纳。正常运行时若 Mailbox 满载，Gate 保留连接映射并由当前连接处理协程按固定间隔重试；该协程在通知接纳前继续占用 `MaxConnections` 名额，使待处理数量有界。Gate 开始停机后，连接处理协程停止重试并退出，剩余映射由 `OnStop` 使用自己的有界上下文补发；这样 `OnStop` 等待 I/O 协程退出时不会被断开通知反向阻塞。Runtime 或目标 Service 停止也终止重试。DebugGate 同样使用 Call，而不是仅确认断开事件进入 Gate Mailbox 的 Send。

## 下一步与验收

Envelope 的 msg_id、request_id(uint32)、payload 三个字段，请求/响应/通知/推送独立编号，通用与业务专用 `.proto` 分开定义；每个 MsgId 通过 option 显式声明 payload、方向和消息种类，协议目录由代码生成，业务 Handler 按实现进度手动注册。固定 33 字节二进制握手、大端 seq 和原始字节 MAC 验证顺序、固定 HKDF 参数、独立 DebugGate、WebSocket 关闭码和 Gate/Agent/业务错误归属，以及“一连接一个业务定义的 AgentService、Runtime 内部消息类型区分客户端消息与普通 Core Command、Service 可选择实现 ClientMessageHandler、业务协议通过 ProtocolHandler 组件组合接入”的边界已确认。Runtime 的单向投递 API 与缺少客户端处理能力时的 admission 拒绝已实现；联调示例包含通用 Envelope 与 Echo 业务 `.proto`、Go/TypeScript 生成类型、强类型 Echo Handler 和 CLI 闭环。跨语言认证固定向量已由 Go 与 TypeScript 共用夹具验证。Echo 示例保留底层 Runtime 组装；应用也可经 `self.OpenDebugGate` 启动调试 Gate 和业务 Agent，不扩展 Core。生产 Gate 的直连 WSS 与 Ingress 终止 TLS 后的 HTTP WebSocket 纵向切片、共享 Logger 接线和可调时长的慢连接压力测试已落地；目标部署的容量与其它失败组合仍待验证。一次讨论一个问题，未确认前不新增公开 API 或通用安全插件框架。

联调示例位于 `examples/websocket-gate/`：Go DebugGate/ProdGate、连接 Echo Agent、TypeScript 命令行客户端和 request/response 往返已经落地；生成协议目录、逐步注册强类型 Handler、通用协议错误回复、业务拒绝 Response 以及 Notify→Push 往返也已接入。Echo `.proto` 只属于示例业务协议，不扩展 GSR Core。固定向量已作为独立跨语言测试夹具落地，不由示例运行路径加载或掩盖后续生产 Gate 的安全审查。

已增加显式开关的 `GSR_GATE_STRESS=1` 混合连接压力验收，可通过 `GSR_GATE_STRESS_SECONDS` 调整时长，记录持续发送、Agent 处理、Mailbox 满载和连接拒绝计数；测试等待队列排空并要求成功写入数与 Agent 处理数一致、Mailbox 满载和连接拒绝均为零。同一开关下的 `TestProdGateLongSlowClientBackpressure` 可用 `GSR_GATE_SLOW_CLIENT_SECONDS` 验证慢客户端持续不读时的有界出站队列和快速拒绝；`TestProdGateMultipleSlowClientsDoNotBlockHealthyRequests` 进一步组合四条慢连接与一条健康连接，验证背压期间健康请求持续完成，五条连接断开后各自 Agent 都收到通知。`TestProdGateFailurePhaseStress` 可用 `GSR_GATE_FAILURE_ATTEMPTS` 重复坏 TLS 首包、坏应用握手和坏 MAC，确认失败帧未进入 Agent、故障连接回收后健康连接仍可请求/响应；坏 TLS 首包本身不计入应用层 `RejectedConnections`，若同时触发 TCP 容量拒绝仍会计数。已完成一次 16 连接、2 分钟的混合样本和一次四慢一健康、5 秒的背压样本；这些测试只提供可重复证据，不把单次机器结果当作容量结论，仍需按目标部署继续观察更长时长和故障组合。既有认证测试覆盖跨连接旧包、同连接重复序号、MAC 篡改和认证失败不推进序号；新增 Go/TypeScript 测试覆盖 `uint64` 序号上限不回绕，协议分发测试覆盖最大 `uint32 request_id` 原样回传。未知 `msg_id` 由协议/Agent 测试覆盖；C/S `request_id` 的编号分配与耗尽由客户端管理，当前示例只使用固定编号，不包含并发分配器验收。测试不替代密码协议审查。

`TestProdGateKeepsConnectionAfterUnknownMessage` 已补充合法认证帧携带未知 `msg_id` 的真实 Gate 验收：Agent 回传原 `request_id` 的通用 `ErrorResponse`，连接不关闭，后续有效请求仍可完成。Agent 内部 Core Call 的超时与迟到回复已有独立验收；`TestProdGateKeepsConnectionAfterClientRequestTimeout` 另以业务 Agent 延迟首个请求，验证客户端结束该请求的等待后忽略迟到响应，仍可在同一连接完成下一请求。业务专属超时数值不由 Gate 定义；多连接背压已有可重复短时样本，更长时长和目标部署容量仍需实测。

`TestProdGateReturnsUnsupportedForUnimplementedRequest` 覆盖协议目录已定义但 Agent 尚未注册 Handler 的 Request：连续请求均返回保留原 `request_id` 的 `UNSUPPORTED_MESSAGE`，连接保持可用。

`TestProdGateKeepsConnectionAfterUnknownNotification` 进一步覆盖 `request_id == 0` 的未知通知：Agent 不生成 `ErrorResponse`，连接继续保留并可处理后续请求。

`TestDefinedNotificationWithoutHandlerDoesNotRespond` 验证协议目录已定义、但业务尚未注册 Handler 的 Notify 不生成无法关联的响应。

`TestResponseRejectsPayloadTypeMismatch` 验证 Handler 为 Response 提供错误的 Protobuf 类型时不发送错误载荷，改为返回关联的安全通用错误。

`TestNotificationCannotPushResponseMessage` 验证 Notify Handler 不能把 Response 编号作为主动 Push 输出。

`TestErrorResponseSendFailurePreservesHandlerError` 验证通用错误响应发送失败时，Handler 原始错误仍保留在返回错误中供 Runtime 诊断。

`TestProdGateDeliversNotificationPush` 覆盖已注册通知的生产链路：客户端发送零 `request_id` 的 `EchoNotify`，Agent 通过协议上下文发出零 `request_id` 的 `EchoPush`，Gate 原样写回客户端。

`TestProdGateRejectsNotificationWithRequestID` 覆盖已注册 Notify 携带非零 `request_id` 的生产链路：协议层返回 `INVALID_REQUEST`，并在通用错误响应中保留原编号。

`TestProdGateRejectsRequestWithoutRequestID` 覆盖已注册 Request 缺少 `request_id` 的生产链路：不发送无法关联的错误响应，连接仍可处理后续合法请求。

`TestProdGateKeepsConnectionAfterMalformedEnvelope` 覆盖认证成功但无法解码 Protobuf Envelope 的链路：Agent 丢弃无法关联的消息，连接仍可处理后续合法请求。

`TestProdGateReturnsErrorForInvalidPayload` 覆盖已知 `msg_id` 携带非法 Protobuf payload 的链路：Agent 返回 `INVALID_REQUEST` 的通用错误，连接保持可用并能继续处理合法请求。

`TestProdGateReturnsErrorForWrongDirection` 覆盖客户端反向发送服务端 Push 编号的链路：Agent 返回 `INVALID_REQUEST`，不把方向错误当作 WebSocket 坏帧，连接仍可继续处理合法请求。

`TestProdGateConcurrentBadMACDoesNotBlockHealthyConnection` 覆盖 8 条坏 MAC 连接并发失败期间健康连接继续完成 16 次请求/响应，并确认坏帧未进入 Agent。

`TestProdGateConcurrentBadHandshakeDoesNotBlockHealthyConnection` 覆盖 8 条低阶 X25519 公钥握手并发失败期间，健康连接继续完成 8 次请求/响应；无效握手不创建 Agent，也不触发 Mailbox 满载。

`TestProdGateHandshakeTimeoutClosesConnection` 覆盖未完成应用握手期间仍占用唯一连接名额、超限连接被拒绝，以及等待超时关闭后名额释放并允许新客户端完成请求/响应。

`TestProdGateRecoversFromAgentConstructorPanic` 覆盖 Agent 构造器 panic：当前连接收到内部错误关闭，后续连接仍能创建 Agent 并完成请求/响应。

`TestProdGateRecoversFromAgentStartFailure` 覆盖 Agent 的 `OnStart` 返回错误：Runtime 调用该实例的 `OnStop` 清理，Gate 拒绝当前连接后仍可接受新连接。

`TestProdGateListenFailureLeavesRuntimeAvailable` 覆盖 Gate 监听地址被占用时的启动失败：Runtime 保持可用，释放地址后可以在同一 Runtime 上创建并启动新的 Gate。

`TestProdGateRuntimeCloseReleasesActiveClient` 验证关闭活动连接、等待 I/O goroutine、使 `ClientSession` 后续写回返回 `ErrClientSessionClosed`，并在 `Runtime.Close()` 后立即重绑同一监听地址确认端口释放。

`TestProdGateRuntimeStopReleasesListener` 验证单独调用 `Runtime.Stop()` 即释放 Gate 监听地址，随后调用 `Runtime.Close()` 可以完成最终释放。

`TestProdGateRuntimeStopClosesPendingHandshake` 验证应用握手尚未完成时 `Runtime.Stop()` 会主动关闭连接并等待 I/O goroutine 退出，不等待配置的握手超时，也不创建 Agent。

`TestProdGateRuntimeStopClosesAllPendingHandshakes` 覆盖 8 条并发等待应用握手的连接：`Runtime.Stop()` 关闭全部连接并等待处理 goroutine 退出，不创建 Agent。

`TestProdGateRuntimeStopClosesAllActiveClients` 覆盖 8 条已认证活动连接：`Runtime.Stop()` 关闭全部连接，每个 `ClientSession` 后续写回均返回 `ErrClientSessionClosed`。

`TestProdGateAgentAndRuntimeStopCloseSessionIdempotently` 验证 Agent 的 `OnStop` 与 Gate 停止都会关闭同一 `ClientSession` 时，Runtime 停止和重复关闭均正常完成。

`TestProdGateAgentStopAfterClientDisconnectKeepsServing` 验证客户端先断开后，Agent 的 `OnStop` 再关闭会话不会产生停止错误；随后新客户端仍能完成认证、请求投递和响应写回。

`TestProdGateClientDisconnectOverlapsRuntimeStop` 并发触发客户端断开与 `Runtime.Stop()`，验证双方关闭同一会话后停止成功、Agent 的 `OnStop` 仅执行一次且会话拒绝后续写回。

`TestProdGateStopReportsClientCloseFailure` 与 `TestDebugGateStopReportsClientCloseFailure` 为两条连接分别注入 Close 帧写错误和 socket 关闭错误，验证生产 Gate 和 DebugGate 停机都会尝试关闭全部客户端连接，并通过 `errors.Is` 保留四类关闭错误。

`TestProdGateReturnsErrorAfterAgentCallTimeout` 覆盖 Agent 内部 Core Call 超时：客户端收到原 `request_id` 的 `INTERNAL`，目标 Service 之后完成的迟到内部回复不会再生成第二条客户端响应，超时归 Agent/Runtime 而不是 Gate。该测试锁定内部调用的错误归属，不定义客户端业务请求的超时数值或重试策略。

`TestProdGateRejectsLateAgentResponseAfterDisconnect` 已锁定断开后的迟到写回：Gate 先使 `ClientSession` 失效，再通知 Agent；Agent 后续写回得到 `ErrClientSessionClosed`，不会重新进入出站队列。
