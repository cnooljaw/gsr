// Package gate 提供统一的 WebSocket Gate、客户端写回与 Agent 断线通知能力。
// 单个监听固定使用 MAC（默认）或 Plain 协议；TLS 独立配置，不按首帧猜测或自动降级。
// Gate 负责连接和传输，业务 Agent 负责 token 鉴权与业务协议。
package gate
