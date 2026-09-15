package cluster

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

// 验证取消 Call 不强杀远端 Handler，迟到回复仅诊断并丢弃，同一连接的后续 Call 不受污染。
func TestTCPCallCancellationDiscardsLateReply(t *testing.T) {
	link := newTCPClusterTestLink(t)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(blocked.release) }) }
	defer release()
	budget, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	id, err := link.adapter.runtime.CreateService(budget, blocked)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(budget)
	result := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "block")
		result <- err
	}()
	defer func() {
		cancel()
		release()
		<-done
	}()
	select {
	case <-blocked.entered:
	case <-budget.Done():
		t.Fatal("remote Handler did not enter")
	}
	connection := link.connection(t)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled Call=%v", err)
		}
	case <-budget.Done():
		t.Fatal("cancelled caller did not resume")
	}
	if link.calls.PendingCount() != 0 {
		t.Fatal("cancelled Call left a local pending session")
	}
	// 旧 Handler 仍被受控阻塞；健康 Service 必须能先返回自己的结果。
	value, err := link.client.Call(budget, "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "before-late-reply")
	if err != nil || !reflect.DeepEqual(value, []any{"before-late-reply"}) {
		t.Fatalf("healthy Call=%#v error=%v", value, err)
	}
	release()
	select {
	case failure := <-link.diagnostics:
		if !errors.Is(failure.err, ErrClusterSessionNotFound) || failure.session == 0 {
			t.Fatalf("late reply diagnostic=%+v", failure)
		}
	case <-budget.Done():
		t.Fatal("late reply was not observed")
	}
	value, err = link.client.Call(budget, "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "after-late-reply")
	if err != nil || !reflect.DeepEqual(value, []any{"after-late-reply"}) {
		t.Fatalf("subsequent Call=%#v error=%v", value, err)
	}
	if link.connection(t) != connection {
		t.Fatal("late reply unnecessarily replaced the connection")
	}
	if link.calls.PendingCount() != 0 || link.adapter.runtime.Inspect().PendingCallCount != 0 {
		t.Fatal("late completion left pending sessions")
	}
}
