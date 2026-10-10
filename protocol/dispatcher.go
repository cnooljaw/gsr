package protocol

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/cnooljaw/gsr/protocol/catalog"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

// Dispatcher 按生成的消息目录解码 Envelope，并调用构造时显式绑定的强类型 Handler。
// Scope 是调用方的逐消息上下文类型；Dispatcher 只透传，不保存或解释它。
type Dispatcher[Scope any] struct {
	registry       *Registry
	writer         *clientWriter
	errorMessageID uint32
	handlers       map[uint32]reflect.Value
}

// NewDispatcher 绑定协议目录和连接写回能力，并复制、校验 msg_id 到强类型 Handler 的表。
// Handler 必须是 func(Scope, Context, *具体Protobuf消息) error；编号与参数类型须匹配目录。
// 构造后 Handler 表只读；协议已定义但尚未实现的消息可以不绑定。
func NewDispatcher[Scope any](registry *Registry, sender ClientSender, handlers map[uint32]any) (*Dispatcher[Scope], error) {
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
	compiled := make(map[uint32]reflect.Value, len(handlers))
	for msgID, handler := range handlers {
		entry, exists := registry.byID[msgID]
		if !exists {
			return nil, fmt.Errorf("%w: %d", ErrUnknownMessageID, msgID)
		}
		if entry.Direction != catalog.ClientToServer {
			return nil, fmt.Errorf("%w: msg_id %d is not client-to-server", ErrWrongDirection, msgID)
		}
		binding, err := compileHandler[Scope](entry, handler)
		if err != nil {
			return nil, fmt.Errorf("msg_id %d: %w", msgID, err)
		}
		compiled[msgID] = binding
	}
	return &Dispatcher[Scope]{
		registry:       registry,
		writer:         &clientWriter{registry: registry, sender: sender},
		errorMessageID: errorEntry.ID,
		handlers:       compiled,
	}, nil
}

// MustDispatcher 用于程序员定义的静态协议接线；构造错误时 panic。
func MustDispatcher[Scope any](registry *Registry, sender ClientSender, handlers map[uint32]any) *Dispatcher[Scope] {
	dispatcher, err := NewDispatcher[Scope](registry, sender, handlers)
	if err != nil {
		panic(err)
	}
	return dispatcher
}

func compileHandler[Scope any](entry catalog.Entry, handler any) (reflect.Value, error) {
	if handler == nil {
		return reflect.Value{}, errors.New("protocol: handler is required")
	}
	function := reflect.ValueOf(handler)
	signature := function.Type()
	if signature.Kind() != reflect.Func || function.IsNil() {
		return reflect.Value{}, errors.New("protocol: handler must be a non-nil function")
	}
	if signature.IsVariadic() || signature.NumIn() != 3 || signature.NumOut() != 1 {
		return reflect.Value{}, errors.New("protocol: handler must be func(Scope, Context, Message) error")
	}
	if signature.In(0) != reflect.TypeOf((*Scope)(nil)).Elem() || signature.In(1) != reflect.TypeOf(Context{}) ||
		signature.Out(0) != reflect.TypeOf((*error)(nil)).Elem() {
		return reflect.Value{}, errors.New("protocol: handler Scope, Context or error type does not match")
	}
	payloadType := reflect.TypeOf(entry.NewMessage())
	if signature.In(2) != payloadType {
		return reflect.Value{}, fmt.Errorf("%w: handler expects %s, catalog requires %s",
			ErrPayloadTypeMismatch, signature.In(2), payloadType)
	}
	return function, nil
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
	// 从变量地址取值保留 Scope 的声明类型，接口 Scope 为 nil 时也能正确传入。
	results := handler.Call([]reflect.Value{
		reflect.ValueOf(&self).Elem(),
		reflect.ValueOf(handlerContext),
		reflect.ValueOf(message),
	})
	if !results[0].IsNil() {
		err := results[0].Interface().(error)
		if !responded {
			return dispatcher.fail(envelope.GetRequestId(), err)
		}
		return err
	}
	// Decode 已保证 Request 的编号非零、Notify 的编号为零，无需再次查询消息目录。
	if envelope.GetRequestId() != 0 && !responded {
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
