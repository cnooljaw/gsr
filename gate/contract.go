package gate

import "github.com/cnooljaw/gsr/runtime"

// ClientSession 是绑定单条客户端连接的字节写回能力，与 Runtime 的 SessionID 无关。
// Send 成功仅表示本地队列接纳，接纳时复制快照；nil 与空切片都是零字节消息。
type ClientSession interface {
	Send(frame []byte) error
	Close() error
}

// ClientDisconnectedCommand 表示 Gate 通知对应 Agent 客户端连接已断开的 Core Command。
const ClientDisconnectedCommand runtime.CommandID = 0xffff_ff10
