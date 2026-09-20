package runtime_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const clientMessageTestCommand = runtime.CommandID(0x02000102)

// 验证不支持客户端消息的 Service 在 Mailbox 接纳前被拒绝，且拒绝不会占用队列容量。
func TestRuntimeRejectsClientMessageBeforeMailboxAdmission(t *testing.T) {
	local := newRunningRuntime(t, runtime.Config{MailboxCapacity: 1})
	service := &clientMessageUnsupportedService{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	serviceID, err := local.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	defer close(service.release)

	if err := local.Send(serviceID, clientMessageTestCommand, "running"); err != nil {
		t.Fatalf("Send(running) error = %v", err)
	}
	waitClosed(t, service.started)
	if err := local.Send(serviceID, clientMessageTestCommand, "queued"); err != nil {
		t.Fatalf("Send(queued) error = %v", err)
	}

	err = local.DeliverClientMessage(serviceID, []byte("client envelope"))
	if !errors.Is(err, runtime.ErrClientMessageUnsupported) {
		t.Fatalf("DeliverClientMessage() error = %v, want ErrClientMessageUnsupported", err)
	}
	inspection := local.Inspect()
	if len(inspection.Services) != 1 || inspection.Services[0].MailboxDepth != 1 {
		t.Fatalf("Service inspection = %#v, want exactly one queued Core Command", inspection.Services)
	}
	if inspection.Metrics.AdmissionRejected != 1 {
		t.Fatalf("AdmissionRejected = %d, want one rejected client message", inspection.Metrics.AdmissionRejected)
	}
}

// 验证客户端消息与 Core Command 共用目标 Service 的 Mailbox 和串行 runner，并在接纳时复制 Envelope。
func TestRuntimeDeliversClientMessageOnServiceRunner(t *testing.T) {
	local := newRunningRuntime(t, runtime.Config{})
	service := newClientMessageAwareService()
	serviceID, err := local.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	defer service.releaseClientHandler()

	envelope := []byte("original envelope")
	if err := local.DeliverClientMessage(serviceID, envelope); err != nil {
		t.Fatalf("DeliverClientMessage() error = %v", err)
	}
	waitClosed(t, service.clientStarted)
	envelope[0] = 'X'
	if err := local.Send(serviceID, clientMessageTestCommand, "ordinary command"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}

	select {
	case <-service.commandStarted:
		t.Fatal("Core Command started before the client handler returned")
	case <-time.After(20 * time.Millisecond):
	}

	service.releaseClientHandler()
	clientEvent := waitClientMessageEvent(t, service.events)
	if clientEvent.kind != "client" || clientEvent.source != 0 || clientEvent.value != "original envelope" {
		t.Fatalf("client event = %#v, want copied Envelope and Source zero", clientEvent)
	}
	commandEvent := waitClientMessageEvent(t, service.events)
	if commandEvent.kind != "command" || commandEvent.source != 0 || commandEvent.value != "ordinary command" {
		t.Fatalf("Command event = %#v, want Core Command after the client message", commandEvent)
	}
}

type clientMessageUnsupportedService struct {
	started chan struct{}
	release chan struct{}
}

func (*clientMessageUnsupportedService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *clientMessageUnsupportedService) Handle(_ runtime.CommandContext, _ runtime.CommandID, args ...any) (any, error) {
	if args[0] == "running" {
		close(service.started)
		<-service.release
	}
	return nil, nil
}

func (*clientMessageUnsupportedService) OnStop(context.Context) error {
	return nil
}

type clientMessageEvent struct {
	kind   string
	source runtime.ServiceID
	value  string
}

type clientMessageAwareService struct {
	clientStarted  chan struct{}
	commandStarted chan struct{}
	releaseClient  chan struct{}
	events         chan clientMessageEvent
	releaseOnce    sync.Once
}

func newClientMessageAwareService() *clientMessageAwareService {
	return &clientMessageAwareService{
		clientStarted:  make(chan struct{}),
		commandStarted: make(chan struct{}),
		releaseClient:  make(chan struct{}),
		events:         make(chan clientMessageEvent, 2),
	}
}

func (*clientMessageAwareService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *clientMessageAwareService) Handle(
	commandContext runtime.CommandContext,
	_ runtime.CommandID,
	args ...any,
) (any, error) {
	close(service.commandStarted)
	service.events <- clientMessageEvent{
		kind:   "command",
		source: commandContext.Source(),
		value:  args[0].(string),
	}
	return nil, nil
}

func (service *clientMessageAwareService) HandleClientMessage(commandContext runtime.CommandContext, envelope []byte) error {
	close(service.clientStarted)
	<-service.releaseClient
	service.events <- clientMessageEvent{
		kind:   "client",
		source: commandContext.Source(),
		value:  string(envelope),
	}
	return nil
}

func (service *clientMessageAwareService) OnStop(context.Context) error {
	return nil
}

func (service *clientMessageAwareService) releaseClientHandler() {
	service.releaseOnce.Do(func() { close(service.releaseClient) })
}

func waitClientMessageEvent(t *testing.T, events <-chan clientMessageEvent) clientMessageEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for a client message event")
		return clientMessageEvent{}
	}
}
