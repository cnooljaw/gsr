package gate

import (
	commonv1 "github.com/cnooljaw/gsr/gate/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/runtime"
)

// Envelope 是客户端与服务端共用的完整 Protobuf 消息外层。
type Envelope = commonv1.Envelope

// ClientSession 是绑定单条客户端连接的出站能力，与 Runtime 的 SessionID 无关。
type ClientSession interface {
	Send(envelope *Envelope) error
	Close() error
}

// ClientDisconnectedCommand 表示 Gate 通知对应 Agent 客户端连接已断开的 Core Command。
const ClientDisconnectedCommand runtime.CommandID = 0xffff_ff10
