package protocol

import (
	"errors"

	"github.com/cnooljaw/gsr/protocol/catalog"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
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

// ClientSender 只接纳编码后的字节消息，不拥有连接，也不依赖 Gate。
type ClientSender interface {
	Send([]byte) error
}

// clientWriter 归 Dispatcher 所有，将同一协议目录绑定到连接写回能力。
type clientWriter struct {
	registry *Registry
	sender   ClientSender
}

// push 将强类型服务端消息编码为 request_id 为零的 Envelope 并排入连接发送队列。
func (writer *clientWriter) push(msgID uint32, message proto.Message) error {
	payload, err := writer.registry.encodeServerMessage(msgID, catalog.Push, message)
	if err != nil {
		return err
	}
	return writer.send(&commonv1.Envelope{
		MsgId:   msgID,
		Payload: payload,
	})
}

func (writer *clientWriter) sendResponse(requestID, msgID uint32, message proto.Message) error {
	if requestID == 0 {
		return ErrMissingRequestID
	}
	payload, err := writer.registry.encodeServerMessage(msgID, catalog.Response, message)
	if err != nil {
		return err
	}
	return writer.send(&commonv1.Envelope{
		MsgId:     msgID,
		RequestId: requestID,
		Payload:   payload,
	})
}

// send 在协议边界编码完整 Envelope，字节发送方不需要理解 Protobuf。
func (writer *clientWriter) send(envelope *commonv1.Envelope) error {
	frame, err := proto.Marshal(envelope)
	if err != nil {
		return err
	}
	return writer.sender.Send(frame)
}
