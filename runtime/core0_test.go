package runtime

import (
	"context"
	"errors"
	"math"
	"strings"
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
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeClosed {
		t.Fatalf("state after Close = %v, want %v", got, RuntimeClosed)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("idempotent Close returned error: %v", err)
	}
	if err := runtime.Start(); !errors.Is(err, ErrRuntimeClosed) {
		t.Fatalf("Start after Close error = %v, want ErrRuntimeClosed", err)
	}
}

// 验证生命周期协调器严格按声明的启动和关闭顺序调用组件。
func TestLifecycleCoordinatorUsesExplicitOrder(t *testing.T) {
	events := make([]string, 0, 6)
	scheduler := &lifecycleProbe{name: "scheduler", events: &events}
	timers := &lifecycleProbe{name: "timers", events: &events}
	services := &lifecycleProbe{name: "services", events: &events}
	coordinator := newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler, timers, services},
		stopOrder:  []lifecycleComponent{timers, services, scheduler},
	})

	if err := coordinator.start(context.Background()); err != nil {
		t.Fatalf("coordinator start returned error: %v", err)
	}
	if err := coordinator.stop(context.Background()); err != nil {
		t.Fatalf("coordinator stop returned error: %v", err)
	}

	want := []string{
		"start scheduler",
		"start timers",
		"start services",
		"stop timers",
		"stop services",
		"stop scheduler",
	}
	if !slicesEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

// 验证组件启动失败时只回滚已经成功启动的组件，并保留原始错误。
func TestLifecycleCoordinatorRollsBackStartedComponents(t *testing.T) {
	startErr := errors.New("timers failed to start")
	events := make([]string, 0, 3)
	scheduler := &lifecycleProbe{name: "scheduler", events: &events}
	timers := &lifecycleProbe{
		name:   "timers",
		events: &events,
		startFn: func(context.Context) error {
			return startErr
		},
	}
	services := &lifecycleProbe{name: "services", events: &events}
	coordinator := newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler, timers, services},
		stopOrder:  []lifecycleComponent{timers, services, scheduler},
	})

	if err := coordinator.start(context.Background()); !errors.Is(err, startErr) {
		t.Fatalf("start error = %v, want %v", err, startErr)
	}
	if err := coordinator.stop(context.Background()); err != nil {
		t.Fatalf("stop after rollback returned error: %v", err)
	}

	want := []string{"start scheduler", "start timers", "stop scheduler"}
	if !slicesEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

// 验证 Runtime 启动失败后进入终态，且 Close 不会再次执行清理流程。
func TestRuntimeStartFailureRollsBackAndStops(t *testing.T) {
	startErr := errors.New("scheduler failed to start")
	events := make([]string, 0, 2)
	scheduler := &lifecycleProbe{
		name:   "scheduler",
		events: &events,
		startFn: func(context.Context) error {
			return startErr
		},
	}
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	runtime.lifecycle = newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler},
		stopOrder:  []lifecycleComponent{scheduler},
	})

	if err := runtime.Start(); !errors.Is(err, startErr) {
		t.Fatalf("Start error = %v, want %v", err, startErr)
	}
	if got := runtime.State(); got != RuntimeStopped {
		t.Fatalf("state after failed Start = %v, want %v", got, RuntimeStopped)
	}
	if err := runtime.Close(); !errors.Is(err, startErr) {
		t.Fatalf("Close after failed Start = %v, want %v", err, startErr)
	}
	if got := runtime.State(); got != RuntimeClosed {
		t.Fatalf("state after Close = %v, want %v", got, RuntimeClosed)
	}

	want := []string{"start scheduler"}
	if !slicesEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

// 验证 Close 会使用 Runtime 统一的停止截止时间，而不是无限等待组件。
func TestRuntimeCloseUsesShutdownTimeout(t *testing.T) {
	stopStarted := make(chan struct{})
	scheduler := &lifecycleProbe{
		name: "scheduler",
		stopFn: func(ctx context.Context) error {
			close(stopStarted)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	runtime, err := NewRuntime(Config{ShutdownTimeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	runtime.lifecycle = newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler},
		stopOrder:  []lifecycleComponent{scheduler},
	})
	if err := runtime.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if err := runtime.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close error = %v, want context deadline exceeded", err)
	}
	select {
	case <-stopStarted:
	default:
		t.Fatal("shutdown component was not called")
	}
	select {
	case <-runtime.stopDone:
	case <-time.After(time.Second):
		t.Fatal("timed shutdown workflow did not finish")
	}
	if err := runtime.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("final Close error = %v, want context deadline exceeded", err)
	}
	if got := runtime.State(); got != RuntimeClosed {
		t.Fatalf("state after timed shutdown = %v, want %v", got, RuntimeClosed)
	}
}

// 验证组件关闭失败时，协调器仍会继续关闭后续组件并返回原始错误。
func TestLifecycleCoordinatorContinuesAfterStopFailure(t *testing.T) {
	stopErr := errors.New("timers failed to stop")
	events := make([]string, 0, 4)
	timers := &lifecycleProbe{
		name:   "timers",
		events: &events,
		stopFn: func(context.Context) error {
			return stopErr
		},
	}
	scheduler := &lifecycleProbe{name: "scheduler", events: &events}
	coordinator := newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{timers, scheduler},
		stopOrder:  []lifecycleComponent{timers, scheduler},
	})
	if err := coordinator.start(context.Background()); err != nil {
		t.Fatalf("coordinator start returned error: %v", err)
	}

	if err := coordinator.stop(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("coordinator stop error = %v, want %v", err, stopErr)
	}

	want := []string{
		"start timers",
		"start scheduler",
		"stop timers",
		"stop scheduler",
	}
	if !slicesEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

// 验证组件关闭 panic 会被转换成错误，并不会阻止后续组件释放资源。
func TestLifecycleCoordinatorRecoversStopPanic(t *testing.T) {
	events := make([]string, 0, 4)
	timers := &lifecycleProbe{
		name:   "timers",
		events: &events,
		stopFn: func(context.Context) error {
			panic("timer stop panic")
		},
	}
	scheduler := &lifecycleProbe{name: "scheduler", events: &events}
	coordinator := newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{timers, scheduler},
		stopOrder:  []lifecycleComponent{timers, scheduler},
	})
	if err := coordinator.start(context.Background()); err != nil {
		t.Fatalf("coordinator start returned error: %v", err)
	}

	err := coordinator.stop(context.Background())
	if err == nil || !strings.Contains(err.Error(), "timer stop panic") {
		t.Fatalf("coordinator stop error = %v, want panic detail", err)
	}

	want := []string{
		"start timers",
		"start scheduler",
		"stop timers",
		"stop scheduler",
	}
	if !slicesEqual(events, want) {
		t.Fatalf("lifecycle events = %v, want %v", events, want)
	}
}

// 验证未启动的 Runtime 也能安全关闭并进入 Closed 状态。
func TestRuntimeCloseFromCreatedIsIdempotent(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	if err := runtime.Close(); err != nil {
		t.Fatalf("Close from Created returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeClosed {
		t.Fatalf("state after Close from Created = %v, want %v", got, RuntimeClosed)
	}
}

// 验证并发 Close 调用只等待同一套关闭流程且最终都观察到相同结果。
func TestRuntimeConcurrentCloseWaitsForSingleWorkflow(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	closeStarted := make(chan struct{})
	releaseClose := make(chan struct{})
	scheduler := &lifecycleProbe{
		name: "scheduler",
		stopFn: func(ctx context.Context) error {
			close(closeStarted)
			select {
			case <-releaseClose:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	runtime.lifecycle = newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler},
		stopOrder:  []lifecycleComponent{scheduler},
	})
	if err := runtime.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	firstClose := make(chan error, 1)
	go func() {
		firstClose <- runtime.Close()
	}()

	select {
	case <-closeStarted:
	case <-time.After(time.Second):
		t.Fatal("close workflow did not start")
	}

	secondClose := make(chan error, 1)
	go func() {
		secondClose <- runtime.Close()
	}()
	select {
	case err := <-secondClose:
		t.Fatalf("second Close returned before workflow finished: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	if got := runtime.State(); got != RuntimeStopping {
		t.Fatalf("state while closes wait = %v, want %v", got, RuntimeStopping)
	}

	close(releaseClose)
	if err := <-firstClose; err != nil {
		t.Fatalf("first Close returned error: %v", err)
	}
	if err := <-secondClose; err != nil {
		t.Fatalf("second Close returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeClosed {
		t.Fatalf("final state = %v, want %v", got, RuntimeClosed)
	}
}

// 验证 Stop 等待者取消只影响当前调用，不会取消 Runtime 已拥有的停止流程。
func TestRuntimeStopWaiterCancellationDoesNotCancelWorkflow(t *testing.T) {
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}

	stopStarted := make(chan struct{})
	releaseStop := make(chan struct{})
	scheduler := &lifecycleProbe{
		name: "scheduler",
		stopFn: func(ctx context.Context) error {
			close(stopStarted)
			select {
			case <-releaseStop:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	runtime.lifecycle = newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler},
		stopOrder:  []lifecycleComponent{scheduler},
	})
	if err := runtime.Start(); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	stopContext, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runtime.Stop(stopContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Stop error = %v, want context.Canceled", err)
	}
	select {
	case <-stopStarted:
	case <-time.After(time.Second):
		t.Fatal("stop workflow did not start")
	}
	if got := runtime.State(); got != RuntimeStopping {
		t.Fatalf("state after cancelled Stop = %v, want %v", got, RuntimeStopping)
	}

	secondStop := make(chan error, 1)
	go func() { secondStop <- runtime.Stop(context.Background()) }()
	select {
	case err := <-secondStop:
		t.Fatalf("second Stop returned before release: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(releaseStop)
	if err := <-secondStop; err != nil {
		t.Fatalf("second Stop returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeStopped {
		t.Fatalf("final state = %v, want %v", got, RuntimeStopped)
	}
}

// 验证 Close 遇到正在启动的组件时，会等待启动完成后再进入关闭流程。
func TestRuntimeCloseWaitsForStartToFinish(t *testing.T) {
	startStarted := make(chan struct{})
	releaseStart := make(chan struct{})
	scheduler := &lifecycleProbe{
		name: "scheduler",
		startFn: func(ctx context.Context) error {
			close(startStarted)
			select {
			case <-releaseStart:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	}
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	runtime.lifecycle = newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler},
		stopOrder:  []lifecycleComponent{scheduler},
	})

	startResult := make(chan error, 1)
	go func() { startResult <- runtime.Start() }()
	select {
	case <-startStarted:
	case <-time.After(time.Second):
		t.Fatal("start workflow did not start")
	}
	if got := runtime.State(); got != RuntimeStarting {
		t.Fatalf("state during Start = %v, want %v", got, RuntimeStarting)
	}

	closeResult := make(chan error, 1)
	go func() { closeResult <- runtime.Close() }()
	select {
	case err := <-closeResult:
		t.Fatalf("Close returned before Start finished: %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	close(releaseStart)
	if err := <-startResult; err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	if err := <-closeResult; err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeClosed {
		t.Fatalf("final state = %v, want %v", got, RuntimeClosed)
	}
}

// 验证启动期间调用 Close 会排队停止意图，并在启动完成后继续关闭。
func TestRuntimeCloseDuringStartKeepsShutdownQueued(t *testing.T) {
	startStarted := make(chan struct{})
	releaseStart := make(chan struct{})
	stopCalled := make(chan struct{})
	scheduler := &lifecycleProbe{
		name: "scheduler",
		startFn: func(context.Context) error {
			close(startStarted)
			<-releaseStart
			return nil
		},
		stopFn: func(context.Context) error {
			close(stopCalled)
			return nil
		},
	}
	runtime, err := NewRuntime(Config{})
	if err != nil {
		t.Fatalf("NewRuntime returned error: %v", err)
	}
	runtime.lifecycle = newLifecycleCoordinator(lifecyclePlan{
		startOrder: []lifecycleComponent{scheduler},
		stopOrder:  []lifecycleComponent{scheduler},
	})

	startResult := make(chan error, 1)
	go func() { startResult <- runtime.Start() }()
	select {
	case <-startStarted:
	case <-time.After(time.Second):
		t.Fatal("start workflow did not start")
	}

	closeResult := make(chan error, 1)
	go func() { closeResult <- runtime.Close() }()
	select {
	case err := <-closeResult:
		t.Fatalf("Close returned before Start finished: %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	close(releaseStart)
	if err := <-startResult; err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
	select {
	case <-stopCalled:
	case <-time.After(time.Second):
		t.Fatal("queued shutdown did not start")
	}
	if err := <-closeResult; err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
	if got := runtime.State(); got != RuntimeClosed {
		t.Fatalf("final state = %v, want %v", got, RuntimeClosed)
	}
}

type lifecycleProbe struct {
	name    string
	events  *[]string
	startFn func(context.Context) error
	stopFn  func(context.Context) error
}

func (probe *lifecycleProbe) lifecycleName() string {
	return probe.name
}

func (probe *lifecycleProbe) start(ctx context.Context) error {
	if probe.events != nil {
		*probe.events = append(*probe.events, "start "+probe.name)
	}
	if probe.startFn != nil {
		return probe.startFn(ctx)
	}
	return nil
}

func (probe *lifecycleProbe) stop(ctx context.Context) error {
	if probe.events != nil {
		*probe.events = append(*probe.events, "stop "+probe.name)
	}
	if probe.stopFn != nil {
		return probe.stopFn(ctx)
	}
	return nil
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
