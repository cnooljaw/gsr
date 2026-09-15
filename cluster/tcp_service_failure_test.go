package cluster

import (
	"context"
	"errors"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证远端 Handler panic 仅停止故障 Service，Call 安全回包，健康 Service 与新实例继续工作且旧 ID 不误投。
func TestTCPCallServicePanicAndReplacement(t *testing.T) {
	link := newTCPClusterTestLink(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failed := &tcpFailureProbe{panicOnHandle: true, stopped: make(chan struct{})}
	oldID, err := link.adapter.runtime.CreateService(ctx, failed)
	if err != nil {
		t.Fatal(err)
	}
	_, err = link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: oldID}, clusterTestCommand)
	if !errors.Is(err, runtime.ErrServicePanicked) {
		t.Fatalf("panic Call error=%v", err)
	}
	if strings.Contains(err.Error(), "private-panic-detail") {
		t.Fatal("panic details escaped the remote error boundary")
	}
	select {
	case <-failed.stopped:
	case <-ctx.Done():
		t.Fatal("panicked Service did not stop")
	}
	value, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "healthy")
	if err != nil || !reflect.DeepEqual(value, []any{"healthy"}) {
		t.Fatalf("healthy Call=%#v error=%v", value, err)
	}
	replacement := &tcpFailureProbe{stopped: make(chan struct{})}
	newID, err := link.adapter.runtime.CreateService(ctx, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if newID <= oldID {
		t.Fatalf("replacement ID=%d old ID=%d", newID, oldID)
	}
	_, err = link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: oldID}, clusterTestCommand)
	if !errors.Is(err, runtime.ErrServiceClosed) {
		t.Fatalf("stale ID Call error=%v", err)
	}
	if replacement.calls.Load() != 0 {
		t.Fatal("stale ID reached replacement")
	}
	value, err = link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: newID}, clusterTestCommand)
	if err != nil || value != "replacement" {
		t.Fatalf("replacement Call=%#v error=%v", value, err)
	}
	if failed.calls.Load() != 1 || replacement.calls.Load() != 1 {
		t.Fatalf("unexpected invocation counts: failed=%d replacement=%d", failed.calls.Load(), replacement.calls.Load())
	}
	if os.Getenv("GSR_CALL_STABILITY") == "1" {
		// 专项持续验证不拖慢默认测试；不追赶迟到的 tick，也不将闭环成功率当作容量。
		wait, stop := context.WithTimeout(context.Background(), 125*time.Second)
		defer stop()
		duration := time.NewTimer(2 * time.Minute)
		defer duration.Stop()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		started := time.Now()
		rounds := 0
	stability:
		for {
			select {
			case <-duration.C:
				break stability
			case <-wait.Done():
				t.Fatal("stability run exceeded budget")
			case <-ticker.C:
			}
			_, err := link.client.Call(wait, "node-b", ServiceTarget{ServiceID: oldID}, clusterTestCommand)
			if !errors.Is(err, runtime.ErrServiceClosed) {
				t.Fatalf("stale ID changed behavior: %v", err)
			}
			value, err := link.client.Call(wait, "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "healthy")
			if err != nil || !reflect.DeepEqual(value, []any{"healthy"}) {
				t.Fatalf("healthy Call=%#v error=%v", value, err)
			}
			value, err = link.client.Call(wait, "node-b", ServiceTarget{ServiceID: newID}, clusterTestCommand)
			if err != nil || value != "replacement" {
				t.Fatalf("replacement Call=%#v error=%v", value, err)
			}
			rounds++
			if failed.calls.Load() != 1 || replacement.calls.Load() != int32(rounds+1) ||
				link.calls.PendingCount() != 0 || link.adapter.runtime.Inspect().PendingCallCount != 0 {
				t.Fatal("invocation count or pending cleanup changed during stability run")
			}
		}
		t.Logf("stability_elapsed=%s rounds=%d expected_closed=%d successful_calls=%d", time.Since(started), rounds, rounds, rounds*2)
	}
	if link.calls.PendingCount() != 0 || link.adapter.runtime.Inspect().PendingCallCount != 0 {
		t.Fatal("completed Calls left pending sessions")
	}
	select {
	case event := <-link.diagnostics:
		t.Fatalf("Service failure became transport failure: %+v", event)
	default:
	}
}

type tcpFailureProbe struct {
	panicOnHandle bool
	stopped       chan struct{}
	calls         atomic.Int32
}

func (*tcpFailureProbe) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *tcpFailureProbe) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	service.calls.Add(1)
	if service.panicOnHandle {
		panic("private-panic-detail")
	}
	return "replacement", nil
}

func (service *tcpFailureProbe) OnStop(context.Context) error {
	close(service.stopped)
	return nil
}
