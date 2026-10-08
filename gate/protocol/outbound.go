package protocol

import (
	"errors"

	commonv1 "github.com/cnooljaw/gsr/gate/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/gate/protocol/catalog"
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
	return writer.sender.Send(&commonv1.Envelope{
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
	return writer.sender.Send(&commonv1.Envelope{
		MsgId:     msgID,
		RequestId: requestID,
		Payload:   payload,
	})
}
