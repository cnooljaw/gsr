package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 验证独立实例的队列容量不改动 Runtime 默认，批量停止先拒绝全部成员并保留未结束 runner。
func TestServiceConfigAndBoundedStopServices(t *testing.T) {
	local := newCallTestRuntime(t, Config{MailboxCapacity: 8, ShutdownTimeout: time.Second})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var stopped atomic.Int32
	entered := make(chan struct{}, 2)
	ids := make([]ServiceID, 2)
	for i := range ids {
		id, err := local.CreateServiceWithConfig(context.Background(), &exitProbe{handle: func(_ CommandContext) (any, error) {
			entered <- struct{}{}
			<-release
			return nil, nil
		}, stop: func(_ context.Context) error {
			stopped.Add(1)
			return nil
		}}, ServiceConfig{MailboxCapacity: 1})
		if err != nil {
			t.Fatal(err)
		}
		ids[i] = id
		if err := local.Send(id, 1); err != nil {
			t.Fatal(err)
		}
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("handler did not enter")
		}
		if err := local.Send(id, 1); err != nil {
			t.Fatal(err)
		}
		if err := local.Send(id, 1); !errors.Is(err, ErrMailboxFull) {
			t.Fatal(err)
		}
	}
	if local.Config().MailboxCapacity != 8 {
		t.Fatal("instance configuration changed Runtime defaults")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := local.StopServices(ctx, ids...); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	for _, id := range ids {
		if err := local.Send(id, 1); !errors.Is(err, ErrServiceStopping) {
			t.Fatalf("member still admitting: %v", err)
		}
	}
	if stopped.Load() != 0 {
		t.Fatal("OnStop overlapped an active Handler")
	}
	unblock()
	if err := local.StopServices(context.Background(), ids...); err != nil {
		t.Fatal(err)
	}
	if stopped.Load() != 2 {
		t.Fatalf("stopped=%d", stopped.Load())
	}
	if _, err := local.CreateServiceWithConfig(context.Background(), &exitProbe{}, ServiceConfig{MailboxCapacity: -1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	if _, err := local.CreateService(context.Background(), &exitProbe{}); err != nil {
		t.Fatalf("stopping members stopped the Runtime: %v", err)
	}
}
