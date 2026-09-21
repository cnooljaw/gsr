package protocol

import (
	"errors"

	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/examples/websocket-gate/protocol/catalog"
	"google.golang.org/protobuf/proto"
)

var (
	// ErrMissingRequestID 表示响应没有可回传的客户端 request_id。
	ErrMissingRequestID = errors.New("protocol: response requires a non-zero request_id")
	// ErrDuplicateResponse 表示同一个客户端请求尝试发送多次响应。
	ErrDuplicateResponse = errors.New("protocol: request already has a response")
	// ErrUninitializedContext 表示当前 Context 不属于一次有效的消息处理。
	ErrUninitializedContext = errors.New("protocol: context is not initialized")
)

// ClientSender 是绑定单条连接的客户端消息写入能力。
type ClientSender interface {
	Send(*commonv1.Envelope) error
}

// Session 通过生成的消息目录编码并发送响应与服务端主动推送。
type Session struct {
	registry *Registry
	sender   ClientSender
}

// NewSession 创建绑定指定连接和协议目录的客户端写回能力。
func NewSession(registry *Registry, sender ClientSender) (*Session, error) {
	if registry == nil {
		return nil, errors.New("protocol: registry is required")
	}
	if isNil(sender) {
		return nil, errors.New("protocol: client sender is required")
	}
	return &Session{registry: registry, sender: sender}, nil
}

// Push 将强类型服务端消息编码为 request_id 为零的 Envelope 并排入连接发送队列。
func (session *Session) Push(msgID uint32, message proto.Message) error {
	if session == nil || session.registry == nil || isNil(session.sender) {
		return errors.New("protocol: session is not initialized")
	}
	payload, err := session.registry.encodeServerMessage(msgID, catalog.Push, message)
	if err != nil {
		return err
	}
	return session.sender.Send(&commonv1.Envelope{
		MsgId:   msgID,
		Payload: payload,
	})
}

func (session *Session) sendResponse(requestID, msgID uint32, message proto.Message) error {
	if session == nil || session.registry == nil || isNil(session.sender) {
		return errors.New("protocol: session is not initialized")
	}
	if requestID == 0 {
		return ErrMissingRequestID
	}
	payload, err := session.registry.encodeServerMessage(msgID, catalog.Response, message)
	if err != nil {
		return err
	}
	return session.sender.Send(&commonv1.Envelope{
		MsgId:     msgID,
		RequestId: requestID,
		Payload:   payload,
	})
}
