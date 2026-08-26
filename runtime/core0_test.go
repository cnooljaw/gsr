package runtime

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// 验证 ID 从 1 开始，并在达到 uint64 上限后永久耗尽。
func TestIDAllocatorStartsAtOneAndExhaustsWithoutWraparound(t *testing.T) {
	allocator := newIDAllocator(1)

	first, err := allocator.next()
	if err != nil {
		t.Fatalf("first allocation returned error: %v", err)
	}
	if first != 1 {
		t.Fatalf("first allocation = %d, want 1", first)
	}

	allocator = newIDAllocator(math.MaxUint64 - 1)
	lastBeforeExhaustion, err := allocator.next()
	if err != nil {
		t.Fatalf("allocation near limit returned error: %v", err)
	}
	if lastBeforeExhaustion != math.MaxUint64-1 {
		t.Fatalf("allocation near limit = %d, want %d", lastBeforeExhaustion, uint64(math.MaxUint64-1))
	}

	last, err := allocator.next()
	if err != nil {
		t.Fatalf("last allocation returned error: %v", err)
	}
	if last != math.MaxUint64 {
		t.Fatalf("last allocation = %d, want %d", last, uint64(math.MaxUint64))
	}

	for attempt := 0; attempt < 2; attempt++ {
		if _, err := allocator.next(); !errors.Is(err, ErrIDExhausted) {
			t.Fatalf("allocation after exhaustion error = %v, want ErrIDExhausted", err)
		}
	}
}

// 验证结构化错误只按 Class 和 Code 判断，Message 仅用于诊断。
func TestStatusErrorMatchesClassAndCodeNotMessage(t *testing.T) {
	err := NewStatusError(ErrorClassRuntime, ErrorCode("runtime.example"), "first message")
	sameCode := NewStatusError(ErrorClassRuntime, ErrorCode("runtime.example"), "second message")
	differentCode := NewStatusError(ErrorClassRuntime, ErrorCode("runtime.other"), "first message")
	differentClass := NewStatusError(ErrorClassDomain, ErrorCode("runtime.example"), "first message")

	if err.Class != ErrorClassRuntime || err.Code != ErrorCode("runtime.example") {
		t.Fatalf("status = (%q, %q), want (runtime, runtime.example)", err.Class, err.Code)
	}
	if !errors.Is(err, sameCode) {
		t.Fatal("same class and code should match with errors.Is")
	}
	if errors.Is(err, differentCode) {
		t.Fatal("different code should not match with errors.Is")
	}
	if errors.Is(err, differentClass) {
		t.Fatal("different class should not match with errors.Is")
	}
	if err.Error() == sameCode.Error() {
		t.Fatal("safe error text should preserve the message for diagnostics")
	}
}

// 验证配置默认值、完整配置校验和非法值拒绝行为。
func TestConfigDefaultsAndValidation(t *testing.T) {
	defaults := DefaultConfig()
	if err := defaults.Validate(); err != nil {
		t.Fatalf("default config is invalid: %v", err)
	}
	if err := (Config{}).Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("zero config validation error = %v, want ErrInvalidConfig", err)
	}

	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime with zero config returned error: %v", err)
	}
	if runtime.config != defaults {
		t.Fatalf("normalized config = %#v, want defaults %#v", runtime.config, defaults)
	}

	invalid := defaults
	invalid.MaxCallWait = -time.Nanosecond
	if err := invalid.Validate(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid MaxCallWait error = %v, want ErrInvalidConfig", err)
	}

	invalid = defaults
	invalid.MailboxCapacity = -1
	if _, err := NewRuntime(invalid); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("invalid MailboxCapacity error = %v, want ErrInvalidConfig", err)
	}
}

// 验证 Runtime 为 Service、Session 和 Timer 分配独立的本地 ID。
func TestRuntimeOwnedIDsStartAtOne(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	serviceID, err := runtime.nextServiceID()
	if err != nil || serviceID != ServiceID(1) {
		t.Fatalf("first ServiceID = %d, %v; want 1, nil", serviceID, err)
	}
	sessionID, err := runtime.nextSessionID()
	if err != nil || sessionID != SessionID(1) {
		t.Fatalf("first SessionID = %d, %v; want 1, nil", sessionID, err)
	}
	timerID, err := runtime.nextTimerID()
	if err != nil || timerID != TimerID(1) {
		t.Fatalf("first TimerID = %d, %v; want 1, nil", timerID, err)
	}
}

// 验证 Runtime 的启动、幂等关闭和关闭后再次启动行为。
func TestRuntimeStartAndCloseLifecycle(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeCreated {
		t.Fatalf("initial state = %v, want %v", got, RuntimeCreated)
	}
	if err := runtime.Start(); err != nil {
		t.Fatalf("first Start returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeRunning {
		t.Fatalf("state after Start = %v, want %v", got, RuntimeRunning)
	}
	if err := runtime.Start(); !errors.Is(err, ErrRuntimeAlreadyStarted) {
		t.Fatalf("second Start error = %v, want ErrRuntimeAlreadyStarted", err)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeStopped {
		t.Fatalf("state after Close = %v, want %v", got, RuntimeStopped)
	}
	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("idempotent Close returned error: %v", err)
	}
	if err := runtime.Start(); !errors.Is(err, ErrRuntimeStopped) {
		t.Fatalf("Start after Close error = %v, want ErrRuntimeStopped", err)
	}
}

// 验证未启动的 Runtime 也能安全关闭并进入 Stopped 状态。
func TestRuntimeCloseFromCreatedIsIdempotent(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	if err := runtime.Close(context.Background()); err != nil {
		t.Fatalf("Close from Created returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeStopped {
		t.Fatalf("state after Close from Created = %v, want %v", got, RuntimeStopped)
	}
}

// 验证一个 Close 等待者取消时，不会取消已经开始的关闭流程。
func TestRuntimeCloseWaiterCanStopWaitingWithoutCancellingClose(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	closeStarted := make(chan struct{})
	releaseClose := make(chan struct{})
	runtime.closeFn = func() error {
		close(closeStarted)
		<-releaseClose
		return nil
	}

	firstClose := make(chan error, 1)
	go func() {
		firstClose <- runtime.Close(context.Background())
	}()

	select {
	case <-closeStarted:
	case <-time.After(time.Second):
		t.Fatal("close workflow did not start")
	}

	waiterContext, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.Close(waiterContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled waiter error = %v, want context.Canceled", err)
	}
	if got := runtime.State(); got != RuntimeStopping {
		t.Fatalf("state after cancelled waiter = %v, want %v", got, RuntimeStopping)
	}

	close(releaseClose)
	if err := <-firstClose; err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeStopped {
		t.Fatalf("final state = %v, want %v", got, RuntimeStopped)
	}
}
