package protocol

import (
	"errors"
	"fmt"
	"reflect"

	commonv1 "github.com/cnooljaw/gsr/gate/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/gate/protocol/catalog"
	"google.golang.org/protobuf/proto"
)

// Dispatcher 按生成的消息目录解码 Envelope，并调用业务手动注册的强类型 Handler。
// Scope 是调用方的逐消息上下文类型；Dispatcher 只透传，不保存或解释它。
type Dispatcher[Scope any] struct {
	registry       *Registry
	writer         *clientWriter
	errorMessageID uint32
	handlers       map[uint32]func(Scope, Context, proto.Message) error
}

// NewDispatcher 绑定协议目录和连接写回能力，内部建立解码与响应共用目录的不变量。
func NewDispatcher[Scope any](registry *Registry, sender ClientSender) (*Dispatcher[Scope], error) {
	if registry == nil {
		return nil, errors.New("protocol: registry is required")
	}
	if isNil(sender) {
		return nil, errors.New("protocol: client sender is required")
	}
	errorEntry, exists := registry.entryForType(reflect.TypeOf((*commonv1.ErrorResponse)(nil)))
	if !exists || errorEntry.Direction != catalog.ServerToClient || errorEntry.Kind != catalog.Response {
		return nil, errors.New("protocol: server-to-client ErrorResponse is required")
	}
	return &Dispatcher[Scope]{
		registry:       registry,
		writer:         &clientWriter{registry: registry, sender: sender},
		errorMessageID: errorEntry.ID,
		handlers:       make(map[uint32]func(Scope, Context, proto.Message) error),
	}, nil
}

// Register 将一个已定义的客户端 payload 类型绑定到业务 Handler；协议目录和 Handler 可分开演进。
func Register[Scope any, Message proto.Message](dispatcher *Dispatcher[Scope], handler func(Scope, Context, Message) error) error {
	if dispatcher == nil || dispatcher.registry == nil || dispatcher.writer == nil || dispatcher.handlers == nil {
		return errors.New("protocol: dispatcher is not initialized")
	}
	if handler == nil {
		return errors.New("protocol: handler is required")
	}
	var sample Message
	messageType := reflect.TypeOf(sample)
	if messageType == nil {
		return errors.New("protocol: handler payload must be a concrete Protobuf message type")
	}
	entry, exists := dispatcher.registry.entryForType(messageType)
	if !exists {
		return fmt.Errorf("protocol: payload type %s is not in the generated message catalog", messageType)
	}
	if entry.Direction != catalog.ClientToServer {
		return fmt.Errorf("%w: payload type %s is not client-to-server", ErrWrongDirection, messageType)
	}
	if _, exists := dispatcher.handlers[entry.ID]; exists {
		return fmt.Errorf("protocol: handler already registered for msg_id %d", entry.ID)
	}
	dispatcher.handlers[entry.ID] = func(self Scope, context Context, message proto.Message) error {
		typedMessage, ok := message.(Message)
		if !ok {
			return fmt.Errorf("%w: dispatcher decoded %T for handler %s", ErrPayloadTypeMismatch, message, messageType)
		}
		return handler(self, context, typedMessage)
	}
	return nil
}

// HandleFrame 解码完整 Envelope，并将本次 self 原样传给对应的客户端 Handler。
// Handler 必须在本次调用内使用 self，不得保存它或交给异步任务。
func (dispatcher *Dispatcher[Scope]) HandleFrame(self Scope, frame []byte) error {
	if dispatcher == nil || dispatcher.registry == nil || dispatcher.writer == nil || dispatcher.handlers == nil {
		return errors.New("protocol: dispatcher is not initialized")
	}
	var envelope commonv1.Envelope
	if err := proto.Unmarshal(frame, &envelope); err != nil {
		return fmt.Errorf("protocol: decode Envelope: %w", err)
	}
	return dispatcher.handleEnvelope(self, &envelope)
}

func (dispatcher *Dispatcher[Scope]) handleEnvelope(self Scope, envelope *commonv1.Envelope) error {
	message, err := dispatcher.registry.Decode(envelope.GetMsgId(), envelope.GetRequestId(), envelope.GetPayload())
	if err != nil {
		return dispatcher.fail(envelope.GetRequestId(), err)
	}
	entry, exists := dispatcher.registry.entryForID(envelope.GetMsgId())
	if !exists {
		return dispatcher.fail(envelope.GetRequestId(), fmt.Errorf("%w: msg_id %d", ErrUnknownMessageID, envelope.GetMsgId()))
	}
	handler, exists := dispatcher.handlers[envelope.GetMsgId()]
	if !exists {
		return dispatcher.fail(envelope.GetRequestId(), fmt.Errorf("%w: msg_id %d", ErrHandlerNotRegistered, envelope.GetMsgId()))
	}

	responded := false
	handlerContext := Context{
		writer:    dispatcher.writer,
		requestID: envelope.GetRequestId(),
		responded: &responded,
	}
	if err := handler(self, handlerContext, message); err != nil {
		if !responded {
			return dispatcher.fail(envelope.GetRequestId(), err)
		}
		return err
	}
	if entry.Kind == catalog.Request && !responded {
		return dispatcher.fail(envelope.GetRequestId(), ErrResponseRequired)
	}
	return nil
}

func (dispatcher *Dispatcher[Scope]) fail(requestID uint32, cause error) error {
	if requestID == 0 {
		return cause
	}
	errorCode, message := errorResponseFor(cause)
	response := &commonv1.ErrorResponse{Code: errorCode, Message: message}
	if err := dispatcher.writer.sendResponse(requestID, dispatcher.errorMessageID, response); err != nil {
		return errors.Join(cause, fmt.Errorf("protocol: send ErrorResponse: %w", err))
	}
	return cause
}

func errorResponseFor(err error) (commonv1.ErrorCode, string) {
	switch {
	case errors.Is(err, ErrUnknownMessageID), errors.Is(err, ErrHandlerNotRegistered):
		return commonv1.ErrorCode_ERROR_CODE_UNSUPPORTED_MESSAGE, "unsupported message"
	case errors.Is(err, ErrWrongDirection), errors.Is(err, ErrInvalidPayload):
		return commonv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, "invalid request"
	case errors.Is(err, ErrRequestIDMismatch):
		return commonv1.ErrorCode_ERROR_CODE_INVALID_REQUEST, "invalid request"
	default:
		return commonv1.ErrorCode_ERROR_CODE_INTERNAL, "request could not be processed"
	}
}

// Push 在普通 Command 等业务流程中发送主动消息，不需要构造逐请求 Context。
// 调用方须仍遵守自身 Service 的串行执行和连接生命周期约束。
func (dispatcher *Dispatcher[Scope]) Push(msgID uint32, message proto.Message) error {
	if dispatcher == nil || dispatcher.writer == nil {
		return ErrUninitializedContext
	}
	return dispatcher.writer.push(msgID, message)
}
