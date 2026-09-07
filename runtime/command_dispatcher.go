package runtime

import (
	"fmt"
	"reflect"
)

var (
	commandContextType = reflect.TypeOf((*CommandContext)(nil)).Elem()
	errorType          = reflect.TypeOf((*error)(nil)).Elem()
)

// CommandHandlers 将稳定的 CommandID 映射到强类型 Service 执行函数。
// NewCommandDispatcher 返回前会复制此映射表。
type CommandHandlers map[CommandID]any

// CommandDispatcher 是每个 Service 的不可变命令表。
//
// 这是可选能力：Service 可以嵌入它以获得固定的参数校验，
// 也可以直接实现 Handle，自定义分派与组合方式。
type CommandDispatcher struct {
	handlers map[CommandID]compiledCommandHandler
}

type compiledCommandHandler struct {
	function reflect.Value
	typeInfo reflect.Type
	result   bool
}

// NewCommandDispatcher 校验并编译一个 Service 的命令处理函数表。
func NewCommandDispatcher(handlers CommandHandlers) (*CommandDispatcher, error) {
	compiled := make(map[CommandID]compiledCommandHandler, len(handlers))
	for command, handler := range handlers {
		if command == 0 {
			return nil, wrapStatusError(ErrInvalidCommandHandler, "CommandID must be non-zero")
		}
		compiledHandler, err := compileCommandHandler(handler)
		if err != nil {
			return nil, fmt.Errorf("command %d: %w", command, err)
		}
		compiled[command] = compiledHandler
	}
	return &CommandDispatcher{handlers: compiled}, nil
}

// MustCommandDispatcher 编译静态处理函数表；
// 程序员定义的函数签名无效时触发 panic。
func MustCommandDispatcher(handlers CommandHandlers) *CommandDispatcher {
	dispatcher, err := NewCommandDispatcher(handlers)
	if err != nil {
		panic(err)
	}
	return dispatcher
}

// Handle 将 CommandID 分派给已注册的强类型执行函数。
func (dispatcher *CommandDispatcher) Handle(context CommandContext, command CommandID, args ...any) (any, error) {
	if dispatcher == nil {
		return nil, ErrInvalidCommandHandler
	}
	handler, exists := dispatcher.handlers[command]
	if !exists {
		return nil, fmt.Errorf("%w: %d", ErrUnknownCommand, command)
	}
	return handler.call(context, command, args)
}

func compileCommandHandler(handler any) (compiledCommandHandler, error) {
	if handler == nil {
		return compiledCommandHandler{}, ErrInvalidCommandHandler
	}
	function := reflect.ValueOf(handler)
	typeInfo := function.Type()
	if typeInfo.Kind() != reflect.Func || function.IsNil() {
		return compiledCommandHandler{}, ErrInvalidCommandHandler
	}
	if typeInfo.NumIn() == 0 || typeInfo.In(0) != commandContextType {
		return compiledCommandHandler{}, fmt.Errorf(
			"%w: first parameter must be CommandContext",
			ErrInvalidCommandHandler,
		)
	}
	if typeInfo.IsVariadic() {
		return compiledCommandHandler{}, fmt.Errorf(
			"%w: variadic execution functions are not supported",
			ErrInvalidCommandHandler,
		)
	}
	if typeInfo.NumOut() != 1 && typeInfo.NumOut() != 2 {
		return compiledCommandHandler{}, fmt.Errorf(
			"%w: handler must return error or (result, error)",
			ErrInvalidCommandHandler,
		)
	}
	if typeInfo.Out(typeInfo.NumOut()-1) != errorType {
		return compiledCommandHandler{}, fmt.Errorf(
			"%w: final result must be error",
			ErrInvalidCommandHandler,
		)
	}
	return compiledCommandHandler{
		function: function,
		typeInfo: typeInfo,
		result:   typeInfo.NumOut() == 2,
	}, nil
}

func (handler compiledCommandHandler) call(context CommandContext, command CommandID, args []any) (any, error) {
	arguments, err := handler.arguments(context, command, args)
	if err != nil {
		return nil, err
	}
	results := handler.function.Call(arguments)
	if handlerError := results[len(results)-1]; !handlerError.IsNil() {
		return nil, handlerError.Interface().(error)
	}
	if !handler.result {
		return nil, nil
	}
	return results[0].Interface(), nil
}

func (handler compiledCommandHandler) arguments(
	context CommandContext,
	command CommandID,
	args []any,
) ([]reflect.Value, error) {
	argumentCount := handler.typeInfo.NumIn() - 1
	if len(args) != argumentCount {
		return nil, fmt.Errorf(
			"%w: command %d requires %d arguments, got %d",
			ErrInvalidCommandArguments,
			command,
			argumentCount,
			len(args),
		)
	}
	contextValue := reflect.ValueOf(context)
	if !contextValue.IsValid() || !contextValue.Type().AssignableTo(commandContextType) {
		return nil, fmt.Errorf("%w: CommandContext is nil", ErrInvalidCommandArguments)
	}
	arguments := make([]reflect.Value, 1, len(args)+1)
	arguments[0] = contextValue
	for index, argument := range args {
		expected := handler.typeInfo.In(index + 1)
		value, err := commandArgumentValue(command, index, argument, expected)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, value)
	}
	return arguments, nil
}

func commandArgumentValue(
	command CommandID,
	index int,
	argument any,
	expected reflect.Type,
) (reflect.Value, error) {
	if argument == nil {
		if commandArgumentCanBeNil(expected) {
			return reflect.Zero(expected), nil
		}
		return reflect.Value{}, fmt.Errorf(
			"%w: command %d argument %d must be %s, got nil",
			ErrInvalidCommandArguments,
			command,
			index,
			expected,
		)
	}
	value := reflect.ValueOf(argument)
	if !value.Type().AssignableTo(expected) {
		return reflect.Value{}, fmt.Errorf(
			"%w: command %d argument %d must be %s, got %s",
			ErrInvalidCommandArguments,
			command,
			index,
			expected,
			value.Type(),
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
