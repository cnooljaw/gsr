// Package catalog 定义由 Protobuf 生成的客户端消息目录条目。
package catalog

import "google.golang.org/protobuf/proto"

// Direction 表示消息在客户端与服务端之间的发送方向。
type Direction uint8

const (
	// ClientToServer 表示客户端发送给服务端。
	ClientToServer Direction = iota + 1
	// ServerToClient 表示服务端发送给客户端。
	ServerToClient
)

// Kind 表示消息在客户端协议中的交互类型。
type Kind uint8

const (
	// Request 表示客户端请求，request_id 必须非零并要求一次响应。
	Request Kind = iota + 1
	// Notify 表示客户端通知，request_id 必须为零且不产生响应。
	Notify
	// Response 表示服务端对客户端请求的响应。
	Response
	// Push 表示服务端主动推送，request_id 固定为零。
	Push
)

// Entry 将一个线上消息编号关联到方向和强类型 Protobuf 构造器。
type Entry struct {
	ID         uint32
	Direction  Direction
	Kind       Kind
	NewMessage func() proto.Message
}
