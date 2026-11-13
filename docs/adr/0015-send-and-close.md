# ADR 0015：有界发送后关闭与协议错误策略

## 状态

Accepted，2026-11-13。用户确认 Gate 管连接发送/关闭，协议层在自动回错前执行策略，业务拥有 closing、清理路由和退出流程。随 GSR v1.0.4 发布。

## 依据与取舍

Skynet 的 socket_server.c 在普通 close 尚有发送缓冲时标记 closing、停止读入，继续发送；shutdown 可强制关闭。websocket.lua 先提交 Close 帧再关闭 socket。其 C 事件循环管理缓冲，Lua coroutine 等待事件。GSR 沿用顺序关闭意图，由 Go 连接 owner 管理唯一 writer、总期限、强制断连与等待，不等待客户端确认，不以业务 Timer 模拟发送完成。

## 决定

- ClientSession 新增 SendAndClose([]byte) error；原 Close 仍可立即中止，Send 仍仅表示本地接纳。新方法复制并接纳唯一末帧后原子封闭发送与入站接纳，保留此前队列 FIFO，不绕过容量；重复调用返回 ErrClientSessionClosed，不延长期限。
- Config 与 gsr.GateConfig 增加 SendAndCloseTimeout，零为一秒，负数拒绝。总预算从接纳开始覆盖在写旧帧、队列、末帧和关闭；独立受管理的超时能中断阻塞写，不逐帧续期。节点停机与显式 Close 可提前中止。接纳失败仍封闭连接并异步关闭，不伪造成功。
- Protobuf Dispatcher 增加可选 WithErrorPolicy；默认保持现有行为。显式配置时构造器检查发送后关闭/关闭窄能力，不导入 Gate。策略仅在尚未响应的失败路径同步执行一次，包含无法解码 Envelope 的路径；无可用请求关联号或通知不能伪造终止响应。
- ReplyErrorAndClose 编码唯一安全 ErrorResponse 并直接交 SendAndClose；终止响应尝试失败不递归自动回错，原始错误保留。已有成功响应不再触发策略或追加错误。
- Gate 内部 Closing 与本地入站接纳使用同一同步边界；先完成接纳的消息可以留在 Agent Mailbox，由业务 closing 拦截。读循环不得抢先清空末帧；最终关闭沿现有断线通知路径。
- 业务在策略中决定是否关闭并标记 closing，等待断线通知执行幂等 Leave/条件 Unbind/Exit；不能在接纳后立即 Exit 导致 OnStop.Close 中止末帧。普通业务拒绝可保留连接。Sproto 不借用 Protobuf ErrorResponse，本次不增加其协议层策略。

## 代价与验证

扩展 ClientSession 会影响实现该接口的测试替身，发布时需明确源码兼容影响。每连接至多一个受管理的关闭定时器；writer 收尾等待其回调退出。覆盖 FIFO/快照、容量失败、重复与并发接纳、阻塞旧写、显式关闭/停机、原有断线通知、错误策略、无关联号与已有响应，切片收尾执行 race。
