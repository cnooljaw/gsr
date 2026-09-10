package runtime

import (
	"errors"
	goruntime "runtime"
	"strings"
)

const (
	diagnosticCapacity     = 256
	diagnosticMessageLimit = 4 << 10
	diagnosticStackLimit   = 16 << 10
)

// Diagnostic 是一次 Handler 错误的本地值快照，不包含业务对象或原始 error 引用。
// Class/Code 用于分类；Message/Stack 可能包含敏感信息，宿主负责输出时的脱敏。
type Diagnostic struct {
	ServiceID ServiceID
	Source    ServiceID
	Command   CommandID
	Session   SessionID
	Class     ErrorClass
	Code      ErrorCode
	// Message 最多 4 KiB，不作为安全 Reply 文本使用。
	Message string
	// Stack 最多 16 KiB，仅真实 Handler panic 时非空。
	Stack string
}

// Diagnostics 返回同一个有界单消费流，不是广播；只有 Runtime 进入 Closed 才关闭。
// Stop 或 Close 超时后可能仍有晚到事件，宿主消费任务必须具备自己的取消方式。
// Closed 后仍可读取缓冲事件；nil Runtime 返回 nil。
func (runtime *Runtime) Diagnostics() <-chan Diagnostic {
	if runtime == nil {
		return nil
	}
	return runtime.diagnostics
}

// DroppedDiagnosticCount 返回队列满载丢弃新事件的累计数，关闭后仍可查询。
// nil Runtime 返回零；此计数不代替 HandlerErrors 或 Panics。
func (runtime *Runtime) DroppedDiagnosticCount() uint64 {
	if runtime == nil {
		return 0
	}
	return runtime.droppedDiagnostics.Load()
}

func (runtime *Runtime) recordHandlerError(instance *serviceInstance, message Message, err error, stack string) {
	runtime.recordHandlerMetric(err)
	event := Diagnostic{
		ServiceID: instance.id, Source: message.Source, Command: message.Command, Session: message.Session,
		Class: ErrorClassRuntime, Code: ErrorCodeHandlerFailed,
		Message: diagnosticErrorText(err), Stack: stack,
	}
	var status *StatusError
	if errors.As(err, &status) && status != nil {
		event.Class = ErrorClass(strings.Clone(string(status.Class)))
		event.Code = ErrorCode(strings.Clone(string(status.Code)))
	}
	// 所有生产者都在 runner 内；Close 确认资源释放后才关闭队列。
	select {
	case runtime.diagnostics <- event:
	default:
		runtime.droppedDiagnostics.Add(1)
	}
}

func diagnosticErrorText(err error) (text string) {
	defer func() {
		if recover() != nil {
			text = "error formatting panicked"
		}
	}()
	text = err.Error()
	if len(text) > diagnosticMessageLimit {
		text = text[:diagnosticMessageLimit]
	}
	// 截断后复制，避免短诊断继续持有大字符串的底层存储。
	return strings.Clone(text)
}

func handlerPanicStack() string {
	buffer := make([]byte, diagnosticStackLimit)
	length := goruntime.Stack(buffer, false)
	return string(buffer[:length])
}
