package runtime

import "fmt"

// ErrorClass identifies the stable owner of an error.
type ErrorClass string

const (
	// ErrorClassDomain identifies a business/domain error.
	ErrorClassDomain ErrorClass = "domain"
	// ErrorClassRuntime identifies a Core Runtime error.
	ErrorClassRuntime ErrorClass = "runtime"
)

// ErrorCode is a stable machine-readable error identifier.
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

// StatusError is the single structured error/status shape used by Core. It
// carries a stable class and code with a safe diagnostic message.
//
// errors.Is compares only Class and Code. Message is deliberately excluded
// from program identity so it can be improved without changing behavior.
type StatusError struct {
	Class   ErrorClass
	Code    ErrorCode
	Message string
}

func wrapStatusError(base error, format string, args ...any) error {
	return fmt.Errorf("%w: %s", base, fmt.Sprintf(format, args...))
}

// NewStatusError constructs a structured error.
func NewStatusError(class ErrorClass, code ErrorCode, message string) *StatusError {
	return &StatusError{
		Class:   class,
		Code:    code,
		Message: message,
	}
}

// Error implements error.
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

// Is makes Class and Code the stable identity of a StatusError.
func (err *StatusError) Is(target error) bool {
	other, ok := target.(*StatusError)
	return ok && err != nil && other != nil &&
		err.Class == other.Class &&
		err.Code == other.Code
}

var (
	// ErrIDExhausted means an owner-specific uint64 allocator is exhausted.
	ErrIDExhausted error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeIDExhausted,
		"runtime ID space is exhausted",
	)
	// ErrInvalidConfig means a Runtime configuration is not valid.
	ErrInvalidConfig error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidConfig,
		"runtime configuration is invalid",
	)
	// ErrRuntimeAlreadyStarted means Start was called after the Runtime started.
	ErrRuntimeAlreadyStarted error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeAlreadyStarted,
		"runtime is already started",
	)
	// ErrRuntimeNotRunning means an operation was attempted before Start.
	ErrRuntimeNotRunning error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeNotRunning,
		"runtime is not running",
	)
	// ErrRuntimeStopping means an operation raced with Runtime shutdown.
	ErrRuntimeStopping error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeStopping,
		"runtime is stopping",
	)
	// ErrRuntimeStopped means an operation targeted a stopped Runtime.
	ErrRuntimeStopped error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeStopped,
		"runtime is stopped",
	)
	// ErrRuntimeClosed means an operation targeted a permanently closed Runtime.
	ErrRuntimeClosed error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeRuntimeClosed,
		"runtime is closed",
	)
	// ErrInvalidService means CreateService received a nil Service.
	ErrInvalidService error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidService,
		"service is invalid",
	)
	// ErrServiceNotFound means a ServiceID is not currently registered.
	ErrServiceNotFound error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceNotFound,
		"service is not found",
	)
	// ErrServiceStarting means a Service is not ready to receive ordinary messages.
	ErrServiceStarting error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceStarting,
		"service is starting",
	)
	// ErrServiceStopping means a Service has stopped accepting new messages.
	ErrServiceStopping error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceStopping,
		"service is stopping",
	)
	// ErrServiceClosed means a ServiceID refers to a stopped Service.
	ErrServiceClosed error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServiceClosed,
		"service is closed",
	)
	// ErrServicePanicked means a Service lifecycle or Handler function panicked.
	ErrServicePanicked error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeServicePanicked,
		"service panicked",
	)
	// ErrMailboxFull means a bounded Service mailbox rejected admission.
	ErrMailboxFull error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeMailboxFull,
		"service mailbox is full",
	)
	// ErrUnknownCommand means a Service has no Handler for a CommandID.
	ErrUnknownCommand error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeUnknownCommand,
		"command is unknown",
	)
	// ErrInvalidCommandHandler means a typed command Handler has an unsupported signature.
	ErrInvalidCommandHandler error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidCommandHandler,
		"command Handler signature is invalid",
	)
	// ErrInvalidCommandArguments means a Command's arguments do not match its Handler.
	ErrInvalidCommandArguments error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidCommandArguments,
		"command arguments are invalid",
	)
	// ErrInvalidCallContext means Call received a nil context.Context.
	ErrInvalidCallContext error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeInvalidCallContext,
		"call context is invalid",
	)
	// ErrCallCycle means a Service attempted to Call itself.
	ErrCallCycle error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeCallCycle,
		"self call is not allowed",
	)
	// ErrCallTimeout means Runtime MaxCallWait elapsed before Reply completion.
	ErrCallTimeout error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeCallTimeout,
		"call timed out",
	)
	// ErrHandlerFailed means a Handler returned an unclassified error.
	ErrHandlerFailed error = NewStatusError(
		ErrorClassRuntime,
		ErrorCodeHandlerFailed,
		"service Handler failed",
	)
)
