# WebSocket Gate、客户端协议与 Agent

状态：讨论中，未实现。记录已确认方向及待裁决细节，不是完整密码协议或生产安全验收结论。

## 范围与参考

首版只支持 WebSocket，公网使用 WSS/TLS，以最小业务往返验证接入设计。HTTP 入站/出站、Redis、MySQL 是另一组 Core 外能力，后续分别推进。

Skynet 的 `lualib/snax/gateserver.lua` 通过 socketdriver 消息及 Lua coroutine 管理监听、连接与数据事件；GSR 借鉴职责分离，不照搬 TCP netpack 和 C/Lua 调度。Go I/O 任务必须有明确 owner 管理容量、取消、关闭和等待。

旧 BSPacket 参考 `../baison_middle/protocol/packet.go` 和 `../nbgame_core/transport/newnet/protocols/cpacket.go`。最新选择替代此前固定 12 字节头、DES 及 0x40/0x80 模式设计。当前只推进新客户端、新协议；旧协议识别、分流、代理及兼容均无当前需求，不设计、不实现、不预留接口，等用户明确提出后再裁决。此前讨论的 tag 分流不构成新协议新增 tag 的已确认要求。

## 已确认：线上消息形状

一条完整 WebSocket 二进制消息对应一个应用包，不把 WebSocket 物理分片当作应用消息，不另加长度头。

```text
连接阶段：Client ↔ Gateway，交换 X25519 公钥
          X25519 shared → HKDF → 上行 macKey

业务上行：seq（8B） | Protobuf Envelope | MAC（16B）
业务下行：Protobuf Envelope
```

WSS 提供传输保密和完整性，应用层不再使用 DES 加密；仅上行增加连接绑定的认证与序号检查，下行不添加应用 MAC。握手阶段没有 macKey，使用握手 Envelope，不套业务上行格式。

Envelope 确定使用以下字段与编号；MessageKind 的枚举取值和握手 payload 仍待裁决，尚不生成代码：

```protobuf
message Envelope {
  MessageKind kind = 1; // 消息类别
  uint32 command = 2;   // 业务命令号；握手不用
  uint32 session = 3;   // 请求与回复关联；0 为 Send/推送
  bytes payload = 4;    // 对应类型的 Protobuf 数据
}
```

业务 payload 是对应业务消息的 Protobuf，不维护节点级命令准入清单。旧打地鼠协议仅作业务样例：其 seq_id 对应这里的 session，msg_id 对应 command，不保留旧字段命名或兼容约束。打地鼠消息不进入 GSR 通用协议定义。

原 Code 的分类意图保留在 Envelope 类别中：业务、客户端协商请求、服务端协商响应；握手不占业务命令号。旧头的 Tag、Length、ver 不进入新协议。分类数值随 schema 确定，不把旧偏移和位宽带入新格式。

## 已确认：业务回复、通用错误与推送

复用 kind 区分正常业务消息与通用错误，不新增 Envelope 字段，不为通用错误占用业务命令号。

| 情况 | kind | session | payload |
| --- | --- | --- | --- |
| 业务结果，包括业务成功与业务拒绝 | BUSINESS | 原请求编号 | 对应业务 Response |
| 调用未取得正常业务结果 | ERROR | 原请求编号 | ErrorResponse |
| 服务端主动推送 | BUSINESS | 0 | 对应业务 Push |

```protobuf
message ErrorResponse {
  int32 code = 1;     // 稳定错误码，供程序判断
  string message = 2; // 安全说明，不包含内部堆栈
}
```

- BUSINESS 表示业务给出了结果，不代表业务操作成功。余额不足、道具不足、当前状态不允许等业务拒绝，与业务成功一样放在业务 Response 中，用业务自己的 ret 或结果字段区分。
- 例如扣款余额不足，返回 BUSINESS、原 session 和 DeductResponse（业务码 INSUFFICIENT_FUNDS）；业务 Handler 返回响应数据，不把预期业务拒绝作为普通 Go error 返回。
- ERROR 表示本次调用未能取得正常业务结果。目标服务不可用、调用超时、无法识别命令或内部异常等使用统一 ErrorResponse；客户端按 kind 选择错误解码，不靠尝试解析 payload 猜类型。
- 超时只说明未及时取得结果，不能据此判断业务没有执行、扣款没有发生或状态已回滚；是否重试及如何防止重复操作由业务决定，不自动重放请求。
- session=0 的 Send 处理失败由服务端记录，不自动生成请求回复；推送使用正常业务消息类别。
- 连接断开时无法保证发送错误回复，由客户端连接事件结束等待。
- 程序分支依赖稳定 code，不比较 message；不得直接透传内部 error 文本或堆栈。
- MessageKind 的数字取值、错误码清单和回复 command 的取值规则尚待裁决。这里的 BUSINESS/ERROR 表示已确认类别，不提前固定枚举数字。

## 已确认：C/S Session 与 seq

- Envelope 中 session 使用 uint32；0 表示 Send 或服务端主动推送，非零表示客户端 Request。成功或失败响应原样带回请求 session。
- 每连接从 1 单调分配，不回绕、不复用；超时移除等待但不回收编号，迟到响应丢弃。耗尽时通过正常重连建立新的编号空间，不自动重放旧请求。
- 新连接可重新从 1 开始，旧连接等待不能迁移到新连接。按每秒 10 次 Request，编号约 4971 天（13.6 年）耗尽；Send 不消耗 session。
- C/S Session 与 Core SessionID、ClusterSessionID 分离，不直接用客户端编号作为内部 PendingCall 的键，也不把它当身份凭据或业务幂等键。
- seq 是独立的 uint64 连接序号，所有上行业务包均消耗，包括 session=0 的 Send。单连接从 1 单调递增，不回绕；Gateway 接受 seq > lastSeq。这个规则替代此前“允许同连接旧包原样重发”的范围，额外拒绝同连接重复旧包。
- seq 的线上字节序尚待确定。Laya/TypeScript 实现不得用普通 number 表示完整 uint64，应采用 BigInt 或等效无损表示；uint32 session 可以精确表示。

## 连接认证与接收边界

目标是让连接 A 录制的已认证上行包，不能拿到另一条连接 B 直接复用；seq 额外拒绝同连接的重复旧包。不承诺阻止控制客户端、提取密钥后重新计算 MAC 或重新构造业务请求，也不等于登录鉴权或业务幂等。

已确认方向及实施约束：

- 每连接使用新的 X25519 临时密钥；私钥不发送，密钥仅存内存，连接结束后废弃。不按账号或跨连接复用，密钥和敏感明文不得写入日志。
- 握手可信性依赖正确验证服务端身份的 WSS；无额外身份认证的 X25519 不承诺抵御能替换公钥的本机主动代理。TLS 终止位置和之后的信任边界需在部署时明确。
- 同连接串行执行：检查阶段、消息大小和 seq → 校验 MAC → 解码及校验 Envelope → 更新 lastSeq → 投递 Agent。认证失败不能推进 lastSeq，投递失败不能回退。序号接纳不代表业务执行成功。
- 协商前只接受握手消息；完成后只接受带 seq/MAC 的业务上行，拒绝重复握手、错误方向、未知类别及无 MAC 降级。握手完成判定点、字段和失败处置仍待明确。
- 必须设置消息大小、握手等待和接纳容量上限；具体默认值待裁决，Protobuf 不意味着无限消息大小。

密码参数草案，实施前需与完整握手一起收口：

- X25519 → HKDF-SHA256 → 32 字节上行 macKey，绑定双方公钥、协议标识及 client→server 方向；拒绝全零共享秘密。精确 salt/info 编码待确定，不能让两端自行猜测。
- MAC 使用 HMAC-SHA256 的前 16 字节，覆盖 seq 原始 8 字节和 Envelope 原始字节，不包含标签自身。常量时间比较，不能重新序列化 Envelope 后验 MAC。

依据：[X25519](https://www.rfc-editor.org/rfc/rfc7748.html)、[HKDF](https://www.rfc-editor.org/rfc/rfc5869.html)、[HMAC](https://www.rfc-editor.org/rfc/rfc2104.html)。这是标准原语的组合设计，仍需完整握手定义与跨语言测试，不能把原语标准当作组合协议已审计的证明。

## 已确认：Gate、Agent 与业务 Service

客户端 ↔ WebSocket Gate ↔ Agent Service ↔ 业务 Service。

- Gate 管连接、收发、上行认证和容量，剥离安全封装后交给 Agent；I/O 任务不得直接修改业务状态。
- Agent 是普通 Service，解释客户端命令与数据并中转到对应业务 Service。客户端不能指定任意内部 ServiceID 或伪造 Source。
- Agent 使用现有 Send/Call；内部 Handler 通过 return 完成 Reply，客户端响应由 Agent 组织并交 Gate 发出。不增加 Core 手工 Reply。
- Agent 内 Call 仍是 run-to-completion，等待时该 Agent 不处理下一条消息，其它 Service 仍可运行。
- Agent 与连接/玩家映射、业务 payload 解码归属、断开和退出顺序尚待裁决，不默认所有客户端共用一个串行 Agent。
- Core Message、ServiceID、Cluster wire 不变；客户端 Envelope、seq、macKey 不进入 Core 或 Cluster 协议。

## 下一步与验收

Envelope 四个基本字段、业务回复/通用错误/推送的区分已确认。后续逐项收口：MessageKind 枚举、回复 command 规则与错误码、握手字段、HKDF 精确参数和 seq 编码、Agent 归属、容量与关闭顺序。一次讨论一个问题，未确认前不新增公开 API 或通用安全插件框架。

实施按 Envelope 编解码 → 握手及上行认证 → Gate/Agent 往返 → 失败、背压和关闭推进。验证应包含跨语言固定向量、跨连接旧包拒绝、同连接重复拒绝、MAC 篡改和失败不推进序号、未知类别、阶段/方向错误、超限、Session 超时与迟到、编号耗尽、慢客户端和断开回收。测试不替代密码协议审查。
