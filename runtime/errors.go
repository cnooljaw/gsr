package runtime

import "fmt"

// ErrorClass 标识错误所属的稳定类别。
type ErrorClass string

const (
	// ErrorClassDomain 表示业务或领域错误。
	ErrorClassDomain ErrorClass = "domain"
	// ErrorClassRuntime 表示 Core Runtime 错误。
	ErrorClassRuntime ErrorClass = "runtime"
)

// ErrorCode 是供程序判断的稳定错误标识。
type ErrorCode string

const (
	ErrorCodeIDExhausted             ErrorCode = "runtime.id_exhausted"
	ErrorCodeInvalidConfig           ErrorCode = "runtime.invalid_config"
	ErrorCodeRuntimeAlreadyStarted   ErrorCode = "runtime.already_started"
	ErrorCodeRuntimeNotRunning       ErrorCode = "runtime.not_running"
	ErrorCodeRuntimeStopping         ErrorCode = "runtime.stopping"
	ErrorCodeRuntimeStopped          ErrorCode = "runtime.stopped"
	ErrorCodeRuntimeClosed           ErrorCode = "runtime.closed"
	ErrorCodeInvalidService          ErrorCode = "runtime.invalid_service"
	ErrorCodeServiceNotFound         ErrorCode = "runtime.service_not_found"
	ErrorCodeServiceStarting         ErrorCode = "runtime.service_starting"
	ErrorCodeServiceStopping         ErrorCode = "runtime.service_stopping"
	ErrorCodeServiceClosed           ErrorCode = "runtime.service_closed"
	ErrorCodeServicePanicked         ErrorCode = "runtime.service_panicked"
	ErrorCodeMailboxFull             ErrorCode = "runtime.mailbox_full"
	ErrorCodeUnknownCommand          ErrorCode = "runtime.unknown_command"
	ErrorCodeInvalidCommandHandler   ErrorCode = "runtime.invalid_command_handler"
	ErrorCodeInvalidCommandArguments ErrorCode = "runtime.invalid_command_arguments"
	ErrorCodeInvalidCallContext      ErrorCode = "runtime.invalid_call_context"
	ErrorCodeCallCycle               ErrorCode = "runtime.call_cycle"
	ErrorCodeCallTimeout             ErrorCode = "runtime.call_timeout"
	ErrorCodeHandlerFailed           ErrorCode = "runtime.handler_failed"
)

// StatusError 是 Core 统一使用的结构化错误与状态类型，
// 包含稳定的类别、错误码和可安全展示的诊断信息。
//
// errors.Is 只比较 Class 和 Code，不将 Message 纳入错误身份判断，
// 因此可以改进错误描述而不改变程序行为。
type StatusError struct {
	Class   ErrorClass
	Code    ErrorCode
	Message string
}

func wrapStatusError(base error, format string, args ...any) error {
	return fmt.Errorf("%w: %s", base, fmt.Sprintf(format, args...))
}

// NewStatusError 构造结构化错误。
func NewStatusError(class ErrorClass, code ErrorCode, message string) *StatusError {
	return &StatusError{
		Class:   class,
		Code:    code,
		Message: message,
	}
}

// Error 实现 error 接口。
func (err *StatusError) Error() string {
	if err == nil {
		return "<nil>"
	}
	if err.Code == "" {
		return err.Message
	}
	if err.Message == "" {
		return string(err.Code)
	}
	return string(err.Code) + ": " + err.Message
}

// Is 使用 Class 和 Code 判断 StatusError 的稳定身份。
func (err *StatusError) Is(target error) bool {
	other, ok := target.(*StatusError)
	return ok && err != nil && other != nil &&
		err.Class == other.Class &&
		err.Code == other.Code
}

var (
	// ErrIDExhausted 表示所属组件的 uint64 标识分配器已耗尽。
	ErrIDExhausted error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeIDExhausted,
		"runtime ID space is exhausted",
	)
	// ErrInvalidConfig 表示 Runtime 配置无效。
	ErrInvalidConfig error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidConfig,
		"runtime configuration is invalid",
	)
	// ErrRuntimeAlreadyStarted 表示 Runtime 启动后再次调用了 Start。
	ErrRuntimeAlreadyStarted error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeAlreadyStarted,
		"runtime is already started",
	)
	// ErrRuntimeNotRunning 表示在 Start 之前尝试执行操作。
	ErrRuntimeNotRunning error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeNotRunning,
		"runtime is not running",
	)
	// ErrRuntimeStopping 表示操作与 Runtime 停止过程发生竞争。
	ErrRuntimeStopping error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeStopping,
		"runtime is stopping",
	)
	// ErrRuntimeStopped 表示操作的目标 Runtime 已停止。
	ErrRuntimeStopped error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeStopped,
		"runtime is stopped",
	)
	// ErrRuntimeClosed 表示操作的目标 Runtime 已永久关闭。
	ErrRuntimeClosed error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeClosed,
		"runtime is closed",
	)
	// ErrInvalidService 表示 CreateService 收到了 nil Service。
	ErrInvalidService error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidService,
		"service is invalid",
	)
	// ErrServiceNotFound 表示 ServiceID 当前未注册。
	ErrServiceNotFound error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceNotFound,
		"service is not found",
	)
	// ErrServiceStarting 表示 Service 尚未准备好接收普通消息。
	ErrServiceStarting error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceStarting,
		"service is starting",
	)
	// ErrServiceStopping 表示 Service 已停止接收新消息。
	ErrServiceStopping error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceStopping,
		"service is stopping",
	)
	// ErrServiceClosed 表示 ServiceID 指向已停止的 Service。
	ErrServiceClosed error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceClosed,
		"service is closed",
	)
	// ErrServicePanicked 表示 Service 的生命周期函数或 Handler 发生 panic。
	ErrServicePanicked error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServicePanicked,
		"service panicked",
	)
	// ErrMailboxFull 表示 Service 的有界 Mailbox 拒绝消息入队。
	ErrMailboxFull error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeMailboxFull,
		"service mailbox is full",
	)
	// ErrUnknownCommand 表示 Service 没有对应 CommandID 的 Handler。
	ErrUnknownCommand error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeUnknownCommand,
		"command is unknown",
	)
	// ErrInvalidCommandHandler 表示强类型 Command Handler 的签名不受支持。
	ErrInvalidCommandHandler error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidCommandHandler,
		"command Handler signature is invalid",
	)
	// ErrInvalidCommandArguments 表示 Command 参数与 Handler 不匹配。
	ErrInvalidCommandArguments error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidCommandArguments,
		"command arguments are invalid",
	)
	// ErrInvalidCallContext 表示 Call 收到了 nil context.Context。
	ErrInvalidCallContext error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidCallContext,
		"call context is invalid",
	)
	// ErrCallCycle 表示 Service 尝试 Call 自身。
	ErrCallCycle error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeCallCycle,
		"self call is not allowed",
	)
	// ErrCallTimeout 表示 Reply 完成前已超过 Runtime 的 MaxCallWait。
	ErrCallTimeout error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeCallTimeout,
		"call timed out",
	)
	// ErrHandlerFailed 表示 Handler 返回了未分类错误。
	ErrHandlerFailed error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeHandlerFailed,
		"service Handler failed",
	)
)
