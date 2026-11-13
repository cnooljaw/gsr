package gate

import "github.com/cnooljaw/gsr/runtime"

// ClientSession 是绑定单条客户端连接的字节写回能力，与 Runtime 的 SessionID 无关。
// Send 成功仅表示本地队列接纳，接纳时复制快照；nil 与空切片都是零字节消息。
type ClientSession interface {
	Send(frame []byte) error
	// SendAndClose 接纳唯一末帧并封闭连接，按 FIFO 有界写出后关闭；失败也封闭连接。
	// nil 只表示本地接纳，不等待网络或保证送达；Close 可提前中止。
	SendAndClose(frame []byte) error
	Close() error
}

// ClientDisconnectedCommand 表示 Gate 通知对应 Agent 客户端连接已断开的 Core Command。
const ClientDisconnectedCommand runtime.CommandID = 0xffff_ff10
