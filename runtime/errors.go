package runtime

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
	ErrorCodeIDExhausted           ErrorCode = "runtime.id_exhausted"
	ErrorCodeInvalidConfig         ErrorCode = "runtime.invalid_config"
	ErrorCodeRuntimeAlreadyStarted ErrorCode = "runtime.already_started"
	ErrorCodeRuntimeStopping       ErrorCode = "runtime.stopping"
	ErrorCodeRuntimeStopped        ErrorCode = "runtime.stopped"
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
)
