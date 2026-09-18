# WebSocket Gate、客户端协议与 Agent

状态：讨论中，未实现。记录已确认方向及待裁决细节，不是完整密码协议或生产安全验收结论。

## 范围与参考

首版只支持 WebSocket，公网使用 WSS/TLS，并额外提供显式开启的内网 DebugGate，以最小业务往返验证接入设计。HTTP 入站/出站、Redis、MySQL 是另一组 Core 外能力，后续分别推进。

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

生产 Gate 使用 WSS 提供传输保密和完整性，应用层不再使用 DES 加密；仅上行增加连接绑定的认证与序号检查，下行不添加应用 MAC。握手阶段没有 macKey，使用握手 Envelope，不套生产业务上行格式。DebugGate 不做 X25519、seq 或 MAC，直接传输 Envelope。

Envelope 采用 Protobuf 的薄包装，只标识具体消息和请求关联，不把业务字段、Core Command 或 ServiceID 放入其中：

```protobuf
message Envelope {
  uint32 msg_id = 1;      // 具体 Protobuf 消息编号
  uint32 request_id = 2;  // 客户端请求关联编号；0 为通知或推送
  bytes payload = 3;      // 对应消息的 Protobuf 数据
}
```

业务 payload 是对应业务消息的 Protobuf。请求、响应、通知和推送各自分配独立的 msg_id；共享协议文件提供跨 Go/TypeScript 的 MsgId 常量，Gate/Agent 注册表声明消息方向和 payload 类型。不维护节点级 Core Command 准入清单。旧打地鼠协议仅作业务样例，不进入 GSR 通用协议定义。

示例协议表：

```text
1001  EnterRoomRequest    C2S request
1002  EnterRoomResponse   S2C response
2001  MoveNotify          C2S notify
3001  PlayerMovedPush     S2C push
9001  ErrorResponse       S2C error response
```

响应使用独立的 msg_id，但回传原 request_id；通知和推送的 request_id 为零。握手使用控制编号 1 和 2，具体 payload 为：

```protobuf
message KeyExchangeRequest {
  bytes client_public_key = 1; // 32 字节 X25519 公钥
}

message KeyExchangeResponse {
  bytes server_public_key = 1; // 32 字节 X25519 公钥
}
```

握手消息的 request_id 固定为零，不带业务 seq/MAC；不增加应用层版本字段。旧头的 Tag、Length、ver 不进入新协议。

MsgId 常量示例：

```protobuf
enum MsgId {
  MSG_ID_UNSPECIFIED = 0;
  C2S_KEY_EXCHANGE_REQUEST = 1;
  S2C_KEY_EXCHANGE_RESPONSE = 2;
  C2S_ENTER_ROOM_REQUEST = 1001;
  S2C_ENTER_ROOM_RESPONSE = 1002;
  C2S_MOVE_NOTIFY = 2001;
  S2C_PLAYER_MOVED_PUSH = 3001;
  S2C_ERROR_RESPONSE = 9001;
}
```

`Envelope.msg_id` 保持 `uint32`，枚举只提供共享常量。Gate/Agent 的注册表负责校验方向、payload 类型、request_id 规则和响应关系；数字范围只用于人工整理，不能作为运行时猜测语义的依据。该注册表属于客户端接入层，不进入 Core Runtime，也不限制不同 Service 复用内部 `CommandID`。

## 已确认：生产 Gate 与 DebugGate

- 生产 Gate 负责公网或不可信网络接入：WSS、X25519 握手、上行 seq/MAC、容量和连接生命周期。
- DebugGate 只面向受信任内网调试：一条 WebSocket 二进制消息就是一份 Protobuf Envelope，不做 X25519 握手、不附加 seq、不校验 MAC。它仍使用同一份 Envelope、MsgId 常量、Gate/Agent 注册表和业务路由。
- 两者使用不同监听器、端口和显式配置；生产 Gate 不自动降级到 DebugGate，DebugGate 也不根据首包猜测生产格式。DebugGate 默认关闭，启用时应绑定回环地址或明确的内网地址，并由部署防火墙限制访问。
- DebugGate 没有应用层认证和防重放保证，不能暴露到公网，也不能被当作生产安全模式。业务鉴权仍由 Agent/业务 Service 负责。
- 两个 Gate 只在 Envelope 解码和 Agent 路由之前共享协议注册表；帧接收和安全校验属于各自适配器，业务路由不复制两套。

## 已确认：业务回复、通用错误与推送

| 情况 | msg_id | request_id | payload |
| --- | --- | --- | --- |
| 业务结果，包括业务成功与业务拒绝 | 独立的 Response 编号 | 原请求编号 | 对应业务 Response |
| 调用未取得正常业务结果 | ErrorResponse 编号 | 原请求编号 | ErrorResponse |
| 服务端主动推送 | Push 编号 | 0 | 对应业务 Push |

```protobuf
message ErrorResponse {
  int32 code = 1;     // 稳定错误码，供程序判断
  string message = 2; // 安全说明，不包含内部堆栈
}
```

- 业务消息的 msg_id 直接决定 payload 的 Protobuf 类型，不通过尝试解析 payload 猜类型。
- 业务结果包括业务成功与业务拒绝。余额不足、道具不足、当前状态不允许等业务拒绝，与业务成功一样返回对应业务 Response，用业务自己的 code 或结果字段区分。
- 例如扣款余额不足，返回 DeductResponse 的独立 msg_id、原 request_id 和业务码 INSUFFICIENT_FUNDS；业务 Handler 返回响应数据，不把预期业务拒绝作为普通 Go error 返回。
- ErrorResponse 表示本次调用未能取得正常业务结果。目标服务不可用、调用超时、无法识别消息或内部异常等使用 ErrorResponse 的独立 msg_id；客户端按 msg_id 选择错误解码。
- 超时只说明未及时取得结果，不能据此判断业务没有执行、扣款没有发生或状态已回滚；是否重试及如何防止重复操作由业务决定，不自动重放请求。
- request_id=0 的通知处理失败由服务端记录，不自动生成请求回复；推送使用独立的 Push msg_id。
- 连接断开时无法保证发送错误回复，由客户端连接事件结束等待。
- 程序分支依赖稳定 code，不比较 message；不得直接透传内部 error 文本或堆栈。
- 错误语义已经确认：业务拒绝使用业务 Response，无法取得正常结果时使用 ErrorResponse 并复用原 request_id；具体错误码常量和业务编号由业务协议项目维护，GSR 不建立跨业务的全局错误码表。控制握手编号、payload 和阶段门禁已经确定，消息编号及 Gate/Agent 注册表的归属也保持在客户端接入层。

## 已确认：C/S request_id 与 seq

- Envelope 中 request_id 使用 uint32；0 表示通知或服务端主动推送，非零表示客户端 Request。成功或失败响应原样带回请求 request_id。
- 每连接从 1 单调分配，不回绕、不复用；超时移除等待但不回收编号，迟到响应丢弃。耗尽时通过正常重连建立新的编号空间，不自动重放旧请求。
- 新连接可重新从 1 开始，旧连接等待不能迁移到新连接。按每秒 10 次 Request，编号约 4971 天（13.6 年）耗尽；通知和推送不消耗 request_id。
- C/S request_id 与 Core SessionID、ClusterSessionID 分离；Agent 以连接和 request_id 关联客户端等待，不把它当身份凭据或业务幂等键。
- seq 是独立的 uint64 连接序号，所有上行业务包均消耗，包括 request_id=0 的通知。单连接从 1 单调递增，不回绕；Gateway 接受 seq > lastSeq。这个规则额外拒绝同连接重复旧包。
- seq 在线上使用 8 字节大端序。Laya/TypeScript 实现不得用普通 number 表示完整 uint64，应采用 BigInt 或等效无损表示；uint32 request_id 可以精确表示。

## 生产 Gate 的连接认证与接收边界

目标是让连接 A 录制的已认证上行包，不能拿到另一条连接 B 直接复用；seq 额外拒绝同连接的重复旧包。不承诺阻止控制客户端、提取密钥后重新计算 MAC 或重新构造业务请求，也不等于登录鉴权或业务幂等。

已确认方向及实施约束：

- 每连接由客户端和服务端分别使用 CSPRNG 生成一对新的 X25519 临时密钥。私钥为 32 字节，仅在本端内存中保存；公钥为原始 32 字节，分别放入握手请求和响应。私钥、shared secret 和 macKey 不发送、不写日志，连接结束后废弃；不按账号或跨连接复用。
- 双方分别计算 `X25519(client_private_key, server_public_key)` 和 `X25519(server_private_key, client_public_key)`，结果必须一致；拒绝全零 shared secret。不要手工修改私钥位或自行实现曲线运算，交给标准 X25519 实现处理。
- 握手可信性依赖正确验证服务端身份的 WSS；无额外身份认证的 X25519 不承诺抵御能替换公钥的本机主动代理。TLS 终止位置和之后的信任边界需在部署时明确。
- 同连接串行执行：检查最小长度 → 读取大端 seq → 使用原始 Envelope 字节校验 MAC → 检查 seq 大于 lastSeq 并立即更新 → 解码及校验 Envelope 的结构字段 → 投递 Agent，由 Agent 校验 msg_id、方向、request_id 和 payload。认证失败不能推进 lastSeq；认证成功但消息未知或不合法时也消费该 seq。投递失败不能回退，序号接纳不代表业务执行成功。
- 协商前只接受 C2S_KEY_EXCHANGE_REQUEST，服务端只回复 S2C_KEY_EXCHANGE_RESPONSE；完成后 Gate 只接受带 seq/MAC 的业务上行，拒绝重复握手、错误方向和无 MAC 降级。MAC 校验通过后，Envelope 的 msg_id、payload 类型和业务方向交给 Agent/注册表判断；未知 msg_id 不因“未知编号”本身关闭连接。
- 握手失败直接关闭连接，不进入 Agent，也不发送业务 ErrorResponse；握手成功后双方才启用派生的 macKey。
- 必须设置消息大小、握手等待和接纳容量上限；具体默认值待裁决，Protobuf 不意味着无限消息大小。

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

依据：[X25519](https://www.rfc-editor.org/rfc/rfc7748.html)、[HKDF](https://www.rfc-editor.org/rfc/rfc5869.html)、[HMAC](https://www.rfc-editor.org/rfc/rfc2104.html)。这是标准原语的组合设计，仍需完整握手定义与跨语言测试，不能把原语标准当作组合协议已审计的证明。

## 已确认：跨语言固定向量

Go 与 TypeScript 必须使用同一组独立生成的测试向量验证客户端上行认证，不以“两个实现互相算出相同结果”代替参考结果。固定向量只用于测试，里面的私钥是公开的测试常量，不得被生产连接复用。

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

## 已确认：WebSocket 关闭码与错误归属

Gate 只使用标准 WebSocket 关闭码，不新增当前客户端必须理解的私有 4000–4999 关闭码。关闭原因是简短、稳定的通用标记，不暴露密钥、MAC、内部错误文本、堆栈或未认证载荷；具体错误写入服务端结构化日志。关闭控制帧的原因文本还必须满足 WebSocket 控制帧长度限制。

| 场景 | 关闭码 | 客户端可见原因 |
| --- | ---: | --- |
| 正常关闭 | `1000` | `normal` |
| Runtime 停止、服务下线或维护 | `1001` | `going_away` |
| 非法消息阶段、重复握手、方向错误、Envelope/帧格式错误 | `1002` | `protocol_error` |
| 收到不支持的数据类型，例如文本消息 | `1003` | `unsupported_data` |
| MAC 错误、序号重放、生产/调试模式不匹配或其它安全策略拒绝 | `1008` | `policy_violation` |
| 消息或 payload 超过配置上限 | `1009` | `message_too_big` |
| Gate 自身发生未预期的内部故障 | `1011` | `server_error` |

`1006`（异常断开）和 `1015`（TLS 握手失败）只能作为本地观察结果记录，不能主动放入 Close frame。当前不定义自有关闭码；如果将来确实需要客户端区分新的协议状态，再单独裁决版本兼容和 4000–4999 私有码。

错误归属按处理边界划分：

- **Gate** 负责 WebSocket、握手、帧大小、seq、MAC、模式和关闭；这些失败不进入 Agent，也不发送业务 `ErrorResponse`。
- **Agent** 负责已认证 Envelope 的 `msg_id`、`request_id`、payload 解码、方向和路由。未知 `msg_id` 或合法载荷的协议不支持，有 request_id 时返回通用 `ErrorResponse`，无 request_id 时丢弃并记录；不把未知编号本身当作 WebSocket 坏帧。
- **业务 Service** 负责余额不足、房间不存在、状态不允许等业务拒绝，按对应业务 Response 返回，不改用 WebSocket Close。

Gate 的网络/安全错误使用 Runtime 共享 Logger，按单连接、错误类别限频：保留首条结构化明细，重复项只累计数量，连接结束时补一条汇总。日志可包含连接 ID、Gate 模式、阶段、远端地址、关闭码和内部错误分类；只有在完成认证和解码后才记录可信的 msg_id/request_id。不得记录私钥、shared secret、macKey、原始 payload 或未认证 Envelope 字段。业务日志不做这类去重，也不由 Gate 重复记录。

## 已确认：Gate、Agent 与业务 Service

客户端 ↔ WebSocket Gate ↔ Agent Service ↔ 业务 Service。

- Gate 管连接、收发、上行认证和容量，剥离安全封装后交给 Agent；I/O 任务不得直接修改业务状态。
- `AgentService` 是普通 Service，原则上一个连接对应一个 AgentService，负责解释客户端命令与数据并中转到对应业务 Service。客户端不能指定任意内部 ServiceID 或伪造 Source。
- Gate 的业务组装只提供一个 `NewAgent func(ClientSession) runtime.Service` 构造回调，不增加独立的 `AgentFactory` 类型、Agent 管理器或通用依赖注入容器。Gate 为每条连接调用回调构造独立 Agent 实例；回调只负责组装连接私有 Service，不执行 socket I/O，不持有 Runtime 或 Gate 连接表。
- Agent 使用现有 Send/Call；内部 Handler 通过 return 完成 Reply，客户端响应由 Agent 组织并交 Gate 发出。不增加 Core 手工 Reply。
- Agent 内 Call 仍是 run-to-completion，等待时该 Agent 不处理下一条消息，其它 Service 仍可运行。
- Gate 仍是连接 owner：持有 WebSocket、握手状态、读写队列和关闭权；Agent 不接触 `net.Conn` 或 Gate 内部连接表。Gate 完成安全接纳后，把已认证 Envelope 投递给对应 Agent；Agent 通过受限的客户端会话能力发送完整的 Envelope。
- `AgentService` 组合通用客户端会话、request_id 关联、出站 Envelope 组织和断开清理能力，并持有一个具体的 `ProtocolHandler` 组件；`ProtocolHandler` 不是 Service，没有自己的 ServiceID、Mailbox 或生命周期。
- 例如查询房间人数时，`GameProtocol` 解码 `GetRoomCountRequest`，在当前 Agent Handler 内通过 `self.Call` 调用 `RoomService`，得到结果后返回 `GetRoomCountResponse`；`AgentService` 使用原 request_id 编码并交给客户端会话发送。客户端响应能力不是 Core 的手工 Reply，也不完成 Core Session。
- Gate 到 Agent 的入站路径使用受信任的 Runtime-owned 单向投递：Gate 完成 WebSocket、握手、MAC 和 seq 校验后，把独占的客户端消息快照投递到 AgentService Mailbox，Source 固定为零。这个内部 Command 不属于客户端 `msg_id`，不进入业务协议，也不作为公开 Gate API。
- Gate 的 accept/reader goroutine 不保存或调用 `ServiceContext`，只把新连接、客户端消息和断开事件投递为 GateService 内部 Command；GateService 在自己的 runner 中调用 `NewAgent`、`self.CreateService` 和受控的 Agent 投递。Agent 构造或 `OnStart` 失败时只关闭当前 ClientSession 并记录原因，不影响 Gate、Runtime 或其它 Agent。
- Agent 到 Gate 的出站路径使用绑定当前连接的 `ClientSession` 能力，最小接口只有 `Send(Envelope)` 和 `Close`。Agent/ProtocolHandler 在调用前负责 `msg_id`、`request_id`、payload 类型以及 Response/Push 方向的协议校验；Response 通过复制原请求的 `request_id` 构造，Push 使用 `request_id == 0`，这些语义不由 Gate 判断，也不重复作为方法参数传递。ClientSession 的 `Send` 不改写或推断 Envelope，只负责检查连接状态、出站大小限制并把完整 Envelope 放入 Gate 的有界写队列；写入 socket 仍由 Gate 的写循环完成。写队列接纳成功不代表客户端已经收到数据。
- Gate 与 Agent 的连接通知使用单向内部事件；断开时 Gate 先使 ClientSession 失效，再最多通知 Agent 一次，不等待或强杀 Agent。Agent 与 RoomService、PlayerService 等业务 Service 才使用普通 ServiceContext.Send/Call；业务协议代码不需要知道上述 Gate/Agent 连接细节。
- 具体 `GameProtocol`、`KickProtocol` 等协议组件由业务项目实现，只实现自己的 msg_id、Protobuf 类型、请求校验、简单收发和业务 Service 路由，不了解 WebSocket、seq、MAC、关闭码或 Runtime 内部调度。Agent/协议组件可以持有连接私有的少量会话状态，例如 playerID、登录状态、roomID、seatIndex、客户端版本和临时流程状态；但不拥有跨连接共享、需要恢复或需要持久化的房间、玩家资产、战斗等权威业务状态，也不承担复杂业务流程。后者应由业务 Service 负责，Agent 使用 Send/Call 访问，不把两个 Service 嵌套成一个运行单元。
- Agent 内的会话状态只由该 Agent 串行读写；roomID、playerID 等引用可以作为当前连接的缓存，但不能被当作业务事实来源。断线或重连后，需要从拥有状态的业务 Service 重新确认。
- 业务项目按自己的业务边界组织协议，例如 `CommonProtocol`、`LobbyProtocol` 和具体游戏协议；每个模块自带消息编号、类型绑定和协议测试。GSR 不内置这些协议、Protobuf schema、msg_id 清单或业务路由，只提供 Gate/AgentService 的通用接入边界。业务侧的 `ProtocolDispatcher` 只按 msg_id 选择模块，不承载业务规则；小模块可以直接使用局部 switch，不为每条消息建立额外接口。
- 不建立继承式或不断增长的公开 `BaseAgent`。Go embedding 只有在不会泄漏内部生命周期和方法集合时才可作为局部实现细节；具体 `ProtocolHandler` 接口在出现真实实现和测试边界后再定，不提前扩张 Core API。
- 正常断开时 Gate 停止接纳新消息、使客户端会话能力失效并只通知 Agent 一次；Agent 完成自己的清理后退出。Gate 不等待或强杀阻塞的 Agent，Agent 退出也不关闭 Runtime 或其它 Agent。
- Core Message、ServiceID、Cluster wire 不变；客户端 Envelope、seq、macKey 不进入 Core 或 Cluster 协议。

## 下一步与验收

Envelope 的 msg_id、request_id(uint32)、payload 三个字段，请求/响应/通知/推送独立编号，共享 MsgId 常量与 Gate/Agent 注册表，控制握手消息，大端 seq 和原始字节 MAC 验证顺序，固定 HKDF 参数，独立 DebugGate，WebSocket 关闭码和 Gate/Agent/业务错误归属，以及“一连接一个 AgentService、业务协议通过 ProtocolHandler 组件组合接入”的边界已确认。跨语言密码固定向量和 Agent 的 `NewAgent` 组装回调已确认；后续逐项收口容量与关闭顺序，并在 Gate 完成后增加一个命令行 TypeScript 客户端和 GSR 服务端的联调示例。一次讨论一个问题，未确认前不新增公开 API 或通用安全插件框架。

联调示例计划放在 `examples/websocket-gate/`：Go 端使用 GSR 组装最小 Gate、Agent 和业务 Service，TypeScript 端使用命令行客户端连接并发送 Envelope。示例不定义新的生产协议或框架，只复用业务项目提供的 Protobuf/MsgId；至少演练正常请求、业务拒绝、ErrorResponse、服务端 Push、request_id 关联、连接关闭和 DebugGate/生产 Gate 的边界。生产 Gate 的 X25519/HKDF/seq/MAC 固定向量验证另列为跨语言测试，不由示例掩盖安全协议审查。

实施按共享 Envelope/注册表 → DebugGate 本地闭环 → 生产 Gate 握手及上行认证 → Gate/Agent 往返 → 失败、背压和关闭推进。验证应包含跨语言固定向量、两种 Gate 的边界隔离、跨连接旧包拒绝、同连接重复拒绝、MAC 篡改和失败不推进序号、未知 msg_id、阶段/方向错误、超限、request_id 超时与迟到、编号耗尽、慢客户端和断开回收。测试不替代密码协议审查。
