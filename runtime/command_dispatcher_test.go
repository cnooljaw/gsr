package runtime_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const (
	commandDispatcherGreet   runtime.CommandID = 0x01000101
	commandDispatcherNotify  runtime.CommandID = 0x01000102
	commandDispatcherUnknown runtime.CommandID = 0x010001ff
)

// 验证 CommandID 与现有协议命令字段保持 32 位宽度兼容。
func TestCommandIDMatchesProtocolWidth(t *testing.T) {
	if got := reflect.TypeOf(runtime.CommandID(0)).Size(); got != 4 {
		t.Fatalf("CommandID size = %d, want 4", got)
	}
}

// 验证标准 CommandDispatcher 能按固定参数类型调用 Handler 并返回业务结果。
func TestCommandDispatcherBindsTypedArgumentsAndReturnsResult(t *testing.T) {
	dispatcher, err := runtime.NewCommandDispatcher(runtime.CommandHandlers{
		commandDispatcherGreet: func(_ runtime.CommandContext, name string, sequence uint64) (string, error) {
			return fmt.Sprintf("%s/%d", name, sequence), nil
		},
		commandDispatcherNotify: func(_ runtime.CommandContext, message string) error {
			if message != "ready" {
				t.Fatalf("message = %q, want ready", message)
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("NewCommandDispatcher() error = %v", err)
	}

	result, err := dispatcher.Handle(testCommandContext{source: 7}, commandDispatcherGreet, "Alice", uint64(9))
	if err != nil {
		t.Fatalf("Handle(greet) error = %v", err)
	}
	if result != "Alice/9" {
		t.Fatalf("Handle(greet) result = %#v, want Alice/9", result)
	}

	result, err = dispatcher.Handle(testCommandContext{source: 7}, commandDispatcherNotify, "ready")
	if err != nil {
		t.Fatalf("Handle(notify) error = %v", err)
	}
	if result != nil {
		t.Fatalf("Handle(notify) result = %#v, want nil", result)
	}
}

// 验证 CommandDispatcher 在构造阶段拒绝无法安全绑定的 Handler 签名。
func TestNewCommandDispatcherRejectsInvalidHandlerSignatures(t *testing.T) {
	tests := []struct {
		name    string
		handler any
	}{
		{name: "nil", handler: nil},
		{name: "not function", handler: "handler"},
		{name: "missing context", handler: func(string) error { return nil }},
		{name: "missing error", handler: func(runtime.CommandContext) {}},
		{name: "wrong final result", handler: func(runtime.CommandContext) (string, string) { return "", "" }},
		{name: "variadic", handler: func(runtime.CommandContext, ...string) error { return nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := runtime.NewCommandDispatcher(runtime.CommandHandlers{
				commandDispatcherGreet: test.handler,
			})
			if !errors.Is(err, runtime.ErrInvalidCommandHandler) {
				t.Fatalf("NewCommandDispatcher() error = %v, want ErrInvalidCommandHandler", err)
			}
		})
	}
}

// 验证 Dispatcher 对参数数量、参数类型和未知命令返回可判定的 Runtime Error。
func TestCommandDispatcherRejectsInvalidArgumentsAndUnknownCommands(t *testing.T) {
	dispatcher, err := runtime.NewCommandDispatcher(runtime.CommandHandlers{
		commandDispatcherGreet: func(_ runtime.CommandContext, name string, sequence uint64) (string, error) {
			return fmt.Sprintf("%s/%d", name, sequence), nil
		},
	})
	if err != nil {
		t.Fatalf("NewCommandDispatcher() error = %v", err)
	}

	if _, err := dispatcher.Handle(testCommandContext{}, commandDispatcherGreet, "Alice"); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
		t.Fatalf("Handle(short args) error = %v, want ErrInvalidCommandArguments", err)
	}
	if _, err := dispatcher.Handle(testCommandContext{}, commandDispatcherGreet, "Alice", "wrong"); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
		t.Fatalf("Handle(wrong type) error = %v, want ErrInvalidCommandArguments", err)
	}
	if _, err := dispatcher.Handle(testCommandContext{}, commandDispatcherUnknown); !errors.Is(err, runtime.ErrUnknownCommand) {
		t.Fatalf("Handle(unknown) error = %v, want ErrUnknownCommand", err)
	}
}

// 验证 Dispatcher 构造后不受调用方继续修改 CommandHandlers 映射的影响。
func TestCommandDispatcherCopiesHandlerTableAtConstruction(t *testing.T) {
	handlers := runtime.CommandHandlers{
		commandDispatcherGreet: func(_ runtime.CommandContext) (string, error) {
			return "original", nil
		},
	}
	dispatcher, err := runtime.NewCommandDispatcher(handlers)
	if err != nil {
		t.Fatalf("NewCommandDispatcher() error = %v", err)
	}
	handlers[commandDispatcherGreet] = func(_ runtime.CommandContext) (string, error) {
		return "mutated", nil
	}
	delete(handlers, commandDispatcherNotify)

	result, err := dispatcher.Handle(testCommandContext{}, commandDispatcherGreet)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if result != "original" {
		t.Fatalf("Handle() result = %#v, want original", result)
	}
}

// 验证 CommandContext 只暴露来源读取能力，不提供业务手工 Reply 旁路。
func TestCommandContextDoesNotExposeManualReply(t *testing.T) {
	commandContextType := reflect.TypeOf((*runtime.CommandContext)(nil)).Elem()
	if _, ok := commandContextType.MethodByName("Reply"); ok {
		t.Fatal("CommandContext must not expose Reply")
	}
	if _, ok := commandContextType.MethodByName("Source"); !ok {
		t.Fatal("CommandContext must expose Source")
	}
}

// 验证 Runtime 不会在 Mailbox 接纳前预检查命令，而是把未知命令交给 Service.Handle。
func TestRuntimeDeliversUnknownCommandWithoutPreflight(t *testing.T) {
	runtimeInstance, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtimeInstance.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := runtimeInstance.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	service := &unknownCommandService{handled: make(chan runtime.CommandID, 1)}
	serviceID, err := runtimeInstance.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if err := runtimeInstance.Send(serviceID, commandDispatcherUnknown); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	select {
	case command := <-service.handled:
		if command != commandDispatcherUnknown {
			t.Fatalf("handled command = %d, want %d", command, commandDispatcherUnknown)
		}
	case <-time.After(time.Second):
		t.Fatal("unknown command did not reach Service.Handle")
	}
}

// 验证本地 Service 通过 ServiceContext.Send 投递时，Runtime 会填入真实的发起 ServiceID。
func TestRuntimeSetsSourceForLocalSend(t *testing.T) {
	runtimeInstance, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtimeInstance.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := runtimeInstance.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	receiver := &sourceRecordingService{source: make(chan runtime.ServiceID, 1)}
	receiverID, err := runtimeInstance.CreateService(context.Background(), receiver)
	if err != nil {
		t.Fatalf("CreateService(receiver) error = %v", err)
	}
	sender := &sourceForwardingService{target: receiverID}
	senderID, err := runtimeInstance.CreateService(context.Background(), sender)
	if err != nil {
		t.Fatalf("CreateService(sender) error = %v", err)
	}

	select {
	case source := <-receiver.source:
		if source != senderID {
			t.Fatalf("message source = %d, want sender ServiceID %d", source, senderID)
		}
	case <-time.After(time.Second):
		t.Fatal("forwarded message did not reach receiver")
	}
}

type testCommandContext struct {
	source runtime.ServiceID
}

func (context testCommandContext) Self() runtime.ServiceID {
	return 1
}

func (context testCommandContext) Source() runtime.ServiceID {
	return context.source
}

func (testCommandContext) Send(runtime.ServiceID, runtime.CommandID, ...any) error {
	return nil
}

func (testCommandContext) Call(context.Context, runtime.ServiceID, runtime.CommandID, ...any) (any, error) {
	return nil, nil
}

func (testCommandContext) After(time.Duration, runtime.CommandID, ...any) (runtime.TimerID, error) {
	return 1, nil
}

func (testCommandContext) CancelTimer(runtime.TimerID) bool {
	return true
}

type unknownCommandService struct {
	handled chan runtime.CommandID
}

func (*unknownCommandService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *unknownCommandService) Handle(_ runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if service.handled != nil {
		service.handled <- command
	}
	return nil, runtime.ErrUnknownCommand
}

func (*unknownCommandService) OnStop(context.Context) error {
	return nil
}

type sourceRecordingService struct {
	source chan runtime.ServiceID
}

func (*sourceRecordingService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *sourceRecordingService) Handle(context runtime.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	service.source <- context.Source()
	return nil, nil
}

func (*sourceRecordingService) OnStop(context.Context) error {
	return nil
}

type sourceForwardingService struct {
	target runtime.ServiceID
}

func (service *sourceForwardingService) OnStart(_ context.Context, serviceContext runtime.ServiceContext) error {
	return serviceContext.Send(service.target, commandDispatcherNotify, "source")
}

func (*sourceForwardingService) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, nil
}

func (*sourceForwardingService) OnStop(context.Context) error {
	return nil
}
