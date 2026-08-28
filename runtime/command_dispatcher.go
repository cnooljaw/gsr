package runtime

import (
	"fmt"
	"reflect"
)

var (
	commandContextType = reflect.TypeOf((*CommandContext)(nil)).Elem()
	errorType          = reflect.TypeOf((*error)(nil)).Elem()
)

// CommandHandlers maps stable CommandIDs to typed Service execution functions.
// The map is copied when NewCommandDispatcher returns.
type CommandHandlers map[CommandID]any

// CommandDispatcher is an immutable per-Service command table.
//
// It is optional: a Service can embed it to get fixed argument validation, or
// implement Handle directly for custom dispatch and composition.
type CommandDispatcher struct {
	handlers map[CommandID]compiledCommandHandler
}

type compiledCommandHandler struct {
	function reflect.Value
	typeInfo reflect.Type
	result   bool
}

// NewCommandDispatcher validates and compiles one Service's command handlers.
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

// MustCommandDispatcher compiles a static handler table or panics when its
// programmer-defined signatures are invalid.
func MustCommandDispatcher(handlers CommandHandlers) *CommandDispatcher {
	dispatcher, err := NewCommandDispatcher(handlers)
	if err != nil {
		panic(err)
	}
	return dispatcher
}

// Handle dispatches a CommandID to its registered typed execution function.
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
