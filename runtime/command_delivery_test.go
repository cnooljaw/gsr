package runtime_test

import (
	"context"
	"reflect"
	"testing"
	"time"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const (
	commandDispatcherNotify  runtime.CommandID = 0x01000102
	commandDispatcherUnknown runtime.CommandID = 0x010001ff
)

// 验证 CommandID 与现有协议命令字段保持 32 位宽度兼容。
func TestCommandIDMatchesProtocolWidth(t *testing.T) {
	if got := reflect.TypeOf(runtime.CommandID(0)).Size(); got != 4 {
		t.Fatalf("CommandID size = %d, want 4", got)
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
