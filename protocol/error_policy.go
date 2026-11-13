package protocol

import (
	"errors"
	"fmt"

	"github.com/cnooljaw/gsr/protocol/catalog"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
)

// ErrorAction 决定未响应请求的自动错误处理，不决定安全错误文案。
type ErrorAction uint8

const (
	// ReplyError 保持普通安全回错，连接继续开放。
	ReplyError ErrorAction = iota
	// ReplyErrorAndClose 尽力写出唯一安全错误后有界关闭；无关联号时只关闭。
	ReplyErrorAndClose
)

// ErrCloseUnsupported 表示显式错误策略所需的关闭能力缺失。
var ErrCloseUnsupported = errors.New("protocol: sender does not support send and close")

type terminalSender interface {
	SendAndClose([]byte) error
	Close() error
}

// DispatcherOption 只在构造时配置 Dispatcher，不在处理期间修改。
type DispatcherOption[Scope any] func(*Dispatcher[Scope]) error

// WithErrorPolicy 在尚未响应的失败路径同步调用策略；Scope 不保存、不借给异步任务。
// 显式配置要求 sender 同时支持 SendAndClose/Close，nil 策略拒绝；默认无策略保持旧行为。
func WithErrorPolicy[Scope any](policy func(Scope, uint32, error) ErrorAction) DispatcherOption[Scope] {
	return func(dispatcher *Dispatcher[Scope]) error {
		if policy == nil {
			return errors.New("protocol: error policy is required")
		}
		closer, ok := dispatcher.writer.sender.(terminalSender)
		if !ok {
			return ErrCloseUnsupported
		}
		dispatcher.errorPolicy = policy
		dispatcher.closer = closer
		return nil
	}
}

func (dispatcher *Dispatcher[Scope]) fail(self Scope, msgID, requestID uint32, cause error) error {
	action := ReplyError
	if dispatcher.errorPolicy != nil {
		action = dispatcher.errorPolicy(self, msgID, cause)
	}
	if action == ReplyError {
		if requestID == 0 {
			return cause
		}
		code, message := errorResponseFor(cause)
		if err := dispatcher.writer.sendResponse(requestID, dispatcher.errorMessageID, &commonv1.ErrorResponse{Code: code, Message: message}); err != nil {
			return errors.Join(cause, fmt.Errorf("protocol: send ErrorResponse: %w", err))
		}
		return cause
	}
	if action != ReplyErrorAndClose {
		return errors.Join(cause, fmt.Errorf("protocol: invalid error action %d", action), dispatcher.closer.Close())
	}
	// 已知 Notify 即使携带非法 request_id 也不伪造终止响应；默认回错路径保持原契约。
	entry, known := dispatcher.registry.byID[msgID]
	if requestID == 0 || (known && entry.Kind == catalog.Notify) {
		return errors.Join(cause, dispatcher.closer.Close())
	}
	code, message := errorResponseFor(cause)
	frame, err := dispatcher.writer.encodeResponse(requestID, dispatcher.errorMessageID, &commonv1.ErrorResponse{Code: code, Message: message})
	if err != nil {
		return errors.Join(cause, err, dispatcher.closer.Close())
	}
	// 这是唯一一次终止响应尝试；不经普通 Send，失败也不递归回错。
	return errors.Join(cause, dispatcher.closer.SendAndClose(frame))
}
