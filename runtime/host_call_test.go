package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 验证宿主 Call 经目标 Handler 返回结果，Source 固定为零且不遗留 PendingCall。
func TestRuntimeCallUsesInfrastructureSource(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := &callTargetService{response: "pong", events: make(chan callTargetEvent, 1)}
	id, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	value, err := runtime.Call(context.Background(), id, callTestCommand)
	if err != nil || value != "pong" {
		t.Fatalf("Call = %v, %v", value, err)
	}
	if event := waitCallTargetEvent(t, target.events); event.source != 0 {
		t.Fatalf("Source = %d", event.source)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("completed Call left a PendingCall")
	}
}

// 验证宿主 Call 的超时与取消不强杀 Handler，也不阻塞其它 Service。
func TestRuntimeCallWaitIsBoundedAndIsolated(t *testing.T) {
	for _, mode := range []string{"runtime timeout", "caller cancellation", "caller deadline"} {
		t.Run(mode, func(t *testing.T) {
			clock := newFakeClock()
			runtime := newTimerTestRuntime(t, clock, Config{MaxCallWait: time.Second})
			target := newBlockingCallTargetService()
			defer target.releaseNow()
			id, err := runtime.CreateService(context.Background(), target)
			if err != nil {
				t.Fatal(err)
			}
			other, err := runtime.CreateService(context.Background(), &callTargetService{response: "other"})
			if err != nil {
				t.Fatal(err)
			}
			var ctx context.Context
			var cancel context.CancelFunc
			if mode == "caller deadline" {
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			} else {
				ctx, cancel = context.WithCancel(context.Background())
			}
			defer cancel()
			outcomes := make(chan callOutcome, 1)
			done := make(chan struct{})
			go func() {
				defer close(done)
				value, err := runtime.Call(ctx, id, callTestCommand)
				outcomes <- callOutcome{value: value, err: err}
			}()
			defer func() { cancel(); target.releaseNow(); <-done }()
			waitSignal(t, target.entered)
			if value, err := runtime.Call(context.Background(), other, callTestCommand); err != nil || value != "other" {
				t.Fatalf("other Service Call = %v, %v", value, err)
			}
			want := context.Canceled
			if mode == "runtime timeout" {
				clock.Advance(time.Second)
				want = ErrCallTimeout
			} else if mode == "caller cancellation" {
				cancel()
			} else {
				want = context.DeadlineExceeded
			}
			outcome := waitCallOutcome(t, outcomes)
			if !errors.Is(outcome.err, want) {
				t.Fatalf("Call error = %v, want %v", outcome.err, want)
			}
			if runtime.pendingCalls.hasPending() {
				t.Fatal("unfinished PendingCall after wait ended")
			}
		})
	}
}

// 验证 Runtime 停止先恢复基础设施 Call，再等待未返回的目标 Handler 退出。
func TestRuntimeCallCompletesOnStop(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := newBlockingCallTargetService()
	defer target.releaseNow()
	id, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	outcomes := make(chan callOutcome, 1)
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		value, err := runtime.Call(context.Background(), id, callTestCommand)
		outcomes <- callOutcome{value: value, err: err}
	}()
	defer func() { target.releaseNow(); <-callDone }()
	waitSignal(t, target.entered)
	stopDone := make(chan error, 1)
	go func() { stopDone <- runtime.Stop(context.Background()) }()
	defer func() {
		target.releaseNow()
		if err := <-stopDone; err != nil {
			t.Error(err)
		}
	}()
	if outcome := waitCallOutcome(t, outcomes); !errors.Is(outcome.err, ErrRuntimeStopping) {
		t.Fatalf("Call error = %v", outcome.err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("stopping Runtime left a PendingCall")
	}
}

// 验证无效上下文、运行状态和目标在宿主 Call 路径中返回稳定错误且不遗留等待。
func TestRuntimeCallRejectsInvalidAdmission(t *testing.T) {
	var missing *Runtime
	if _, err := missing.Call(context.Background(), 1, callTestCommand); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatal(err)
	}
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { runtime.Close() })
	if _, err := runtime.Call(nil, 1, callTestCommand); !errors.Is(err, ErrInvalidCallContext) {
		t.Fatal(err)
	}
	if _, err := runtime.Call(context.Background(), 1, callTestCommand); !errors.Is(err, ErrRuntimeNotRunning) {
		t.Fatal(err)
	}
	if err := runtime.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Call(context.Background(), 999, callTestCommand); !errors.Is(err, ErrServiceNotFound) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runtime.Call(ctx, 999, callTestCommand); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("invalid admission left a PendingCall")
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Call(context.Background(), 1, callTestCommand); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatal(err)
	}
}

// 验证宿主 Call 与 Send 共用有界 Mailbox，不能绕过正在执行的 Handler 或队列容量。
func TestRuntimeCallSharesMailboxAdmission(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{MailboxCapacity: 1})
	target := newBlockingCallTargetService()
	defer target.releaseNow()
	id, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.Send(id, callTestCommand); err != nil {
		t.Fatal(err)
	}
	waitSignal(t, target.entered)
	if err := runtime.Send(id, callTestCommand); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Call(context.Background(), id, callTestCommand); !errors.Is(err, ErrMailboxFull) {
		t.Fatalf("Call into full mailbox = %v", err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("rejected Call left a PendingCall")
	}
}
