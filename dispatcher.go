package gsr

import (
	"fmt"
	"reflect"

	"github.com/cnooljaw/gsr/runtime"
)

var (
	dispatcherContextType = reflect.TypeOf((*CommandContext)(nil)).Elem()
	dispatcherErrorType   = reflect.TypeOf((*error)(nil)).Elem()
)

// Dispatcher 是应用 Service 可持有的不可变强类型命令分发器。
//
// Service 应显式实现 Handle 并转发到 Dispatcher.Handle，避免匿名嵌入隐藏消息入口。
// 一个 Service 只需维护自己已实现的命令表；不需要该能力的 Service 可以自行分发。
type Dispatcher struct {
	handlers map[runtime.CommandID]compiledHandler
}

type compiledHandler struct {
	function      reflect.Value
	arguments     []reflect.Type
	returnsResult bool
}

// NewDispatcher 复制命令表并在 Service 启动前校验 Handler 签名。
// Handler 必须是 func(CommandContext, A, ...) error 或 func(CommandContext, A, ...) (R, error)，
// 且不得使用 variadic 参数；非法定义返回 runtime.ErrInvalidCommandHandler。
func NewDispatcher(handlers map[runtime.CommandID]any) (*Dispatcher, error) {
	compiled := make(map[runtime.CommandID]compiledHandler, len(handlers))
	for command, handler := range handlers {
		if command == 0 {
			return nil, fmt.Errorf("%w: CommandID must be non-zero", runtime.ErrInvalidCommandHandler)
		}
		entry, err := compileHandler(handler)
		if err != nil {
			return nil, fmt.Errorf("command %d: %w", command, err)
		}
		compiled[command] = entry
	}
	return &Dispatcher{handlers: compiled}, nil
}

// MustDispatcher 用于由程序员静态定义的命令表；定义无效时 panic。
func MustDispatcher(handlers map[runtime.CommandID]any) *Dispatcher {
	dispatcher, err := NewDispatcher(handlers)
	if err != nil {
		panic(err)
	}
	return dispatcher
}

// Handle 按 CommandID 调用已绑定的强类型 Handler，并返回其结果或错误。
// 未知命令和参数不匹配分别返回 runtime.ErrUnknownCommand 和 runtime.ErrInvalidCommandArguments。
func (dispatcher *Dispatcher) Handle(ctx CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if dispatcher == nil {
		return nil, runtime.ErrInvalidCommandHandler
	}
	handler, exists := dispatcher.handlers[command]
	if !exists {
		return nil, fmt.Errorf("%w: %d", runtime.ErrUnknownCommand, command)
	}
	return handler.call(ctx, command, args)
}

func compileHandler(handler any) (compiledHandler, error) {
	if handler == nil {
		return compiledHandler{}, runtime.ErrInvalidCommandHandler
	}
	function := reflect.ValueOf(handler)
	typeInfo := function.Type()
	if typeInfo.Kind() != reflect.Func || function.IsNil() {
		return compiledHandler{}, runtime.ErrInvalidCommandHandler
	}
	if typeInfo.NumIn() == 0 || typeInfo.In(0) != dispatcherContextType {
		return compiledHandler{}, fmt.Errorf("%w: first parameter must be gsr.CommandContext", runtime.ErrInvalidCommandHandler)
	}
	if typeInfo.IsVariadic() {
		return compiledHandler{}, fmt.Errorf("%w: variadic handlers are not supported", runtime.ErrInvalidCommandHandler)
	}
	if typeInfo.NumOut() != 1 && typeInfo.NumOut() != 2 {
		return compiledHandler{}, fmt.Errorf("%w: handler must return error or (result, error)", runtime.ErrInvalidCommandHandler)
	}
	if typeInfo.Out(typeInfo.NumOut()-1) != dispatcherErrorType {
		return compiledHandler{}, fmt.Errorf("%w: final result must be error", runtime.ErrInvalidCommandHandler)
	}
	arguments := make([]reflect.Type, typeInfo.NumIn()-1)
	for index := range arguments {
		arguments[index] = typeInfo.In(index + 1)
	}
	return compiledHandler{function: function, arguments: arguments, returnsResult: typeInfo.NumOut() == 2}, nil
}

func (handler compiledHandler) call(ctx CommandContext, command runtime.CommandID, args []any) (any, error) {
	if len(args) != len(handler.arguments) {
		return nil, fmt.Errorf(
			"%w: command %d requires %d arguments, got %d",
			runtime.ErrInvalidCommandArguments, command, len(handler.arguments), len(args),
		)
	}
	contextValue := reflect.ValueOf(ctx)
	if !contextValue.IsValid() || (commandArgumentCanBeNil(contextValue.Type()) && contextValue.IsNil()) {
		return nil, fmt.Errorf("%w: CommandContext is nil", runtime.ErrInvalidCommandArguments)
	}
	values := make([]reflect.Value, len(args)+1)
	values[0] = contextValue
	for index, argument := range args {
		value, err := commandArgumentValue(command, index, argument, handler.arguments[index])
		if err != nil {
			return nil, err
		}
		values[index+1] = value
	}
	results := handler.function.Call(values)
	if handlerError := results[len(results)-1]; !handlerError.IsNil() {
		return nil, handlerError.Interface().(error)
	}
	if handler.returnsResult {
		return results[0].Interface(), nil
	}
	return nil, nil
}

func commandArgumentValue(command runtime.CommandID, index int, argument any, expected reflect.Type) (reflect.Value, error) {
	if argument == nil {
		if commandArgumentCanBeNil(expected) {
			return reflect.Zero(expected), nil
		}
		return reflect.Value{}, fmt.Errorf(
			"%w: command %d argument %d must be %s, got nil",
			runtime.ErrInvalidCommandArguments, command, index, expected,
		)
	}
	value := reflect.ValueOf(argument)
	if !value.Type().AssignableTo(expected) {
		return reflect.Value{}, fmt.Errorf(
			"%w: command %d argument %d must be %s, got %s",
			runtime.ErrInvalidCommandArguments, command, index, expected, value.Type(),
		)
	}
	return value, nil
}

func commandArgumentCanBeNil(valueType reflect.Type) bool {
	switch valueType.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}
