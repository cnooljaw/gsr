package cluster

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证远端 Mailbox 满载通过 Call 返回稳定错误，不影响健康 Service，排空后原连接与目标恢复正常。
func TestTCPCallMailboxFullAndRecovery(t *testing.T) {
	link := newTCPClusterTestLink(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	capacity := runtime.DefaultConfig().MailboxCapacity
	service := &tcpMailboxProbe{entered: make(chan struct{}), release: make(chan struct{}), handled: make(chan string, capacity+2)}
	var once sync.Once
	release := func() { once.Do(func() { close(service.release) }) }
	defer release()
	id, err := link.adapter.runtime.CreateService(ctx, service)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		value, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "block")
		if err == nil && value != "block" {
			t.Errorf("blocked Call result=%#v", value)
		}
		result <- err
	}()
	defer func() { cancel(); release(); <-done }()
	select {
	case <-service.entered:
	case <-ctx.Done():
		t.Fatal("Handler did not enter")
	}
	connection := link.connection(t)
	for index := 0; index < capacity; index++ {
		if err := link.adapter.runtime.Send(id, clusterTestCommand, "queued"); err != nil {
			t.Fatal(err)
		}
	}
	_, err = link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "rejected")
	if !errors.Is(err, runtime.ErrMailboxFull) {
		t.Fatalf("full Mailbox Call=%v", err)
	}
	value, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "healthy")
	if err != nil || !reflect.DeepEqual(value, []any{"healthy"}) {
		t.Fatalf("healthy Call=%#v error=%v", value, err)
	}
	release()
	for index := 0; index <= capacity; index++ {
		select {
		case value := <-service.handled:
			want := "queued"
			if index == 0 {
				want = "block"
			}
			if value != want {
				t.Fatalf("handled=%q want=%q", value, want)
			}
		case <-ctx.Done():
			t.Fatal("Mailbox did not drain")
		}
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	value, err = link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "recovered")
	if err != nil || value != "recovered" {
		t.Fatalf("recovered Call=%#v error=%v", value, err)
	}
	if link.connection(t) != connection || link.calls.PendingCount() != 0 || link.adapter.runtime.Inspect().PendingCallCount != 0 {
		t.Fatal("connection changed or pending Call leaked")
	}
	if link.adapter.runtime.Inspect().Metrics.AdmissionRejected != 1 {
		t.Fatal("unexpected admission rejection count")
	}
}

type tcpMailboxProbe struct {
	entered chan struct{}
	release chan struct{}
	handled chan string
}

func (*tcpMailboxProbe) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *tcpMailboxProbe) Handle(_ runtime.CommandContext, _ runtime.CommandID, args ...any) (any, error) {
	value := args[0].(string)
	if value == "block" {
		close(service.entered)
		<-service.release
	}
	service.handled <- value
	return value, nil
}

func (*tcpMailboxProbe) OnStop(context.Context) error {
	return nil
}
