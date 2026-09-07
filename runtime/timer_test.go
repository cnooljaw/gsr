package runtime

import (
	"context"
	"errors"
	stdruntime "runtime"
	"sync"
	"testing"
	"time"
)

const timerTestCommand CommandID = 0x02000201

// 验证本地 Dispatcher 不把缺失的 Service 来源解释成 Runtime-owned 零来源消息。
func TestLocalDispatcherRequiresExplicitServiceOrigin(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	service := &timerRecordingService{events: make(chan timerEvent, 1)}
	serviceID, err := runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if err := runtime.dispatcher.dispatchFromService(nil, serviceID, timerTestCommand); !errors.Is(err, ErrInvalidService) {
		t.Fatalf("dispatchFromService(nil) error = %v, want ErrInvalidService", err)
	}
}

// 验证 Runtime-owned 注入路径显式使用 ServiceID 零值，而不依赖 nil 哨兵。
func TestRuntimeOwnedDispatchUsesZeroSource(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	service := &timerRecordingService{events: make(chan timerEvent, 1)}
	serviceID, err := runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if err := runtime.Send(serviceID, timerTestCommand, "runtime-owned"); err != nil {
		t.Fatalf("Runtime.Send() error = %v", err)
	}
	event := waitTimerEvent(t, service.events)
	if event.source != 0 {
		t.Fatalf("Runtime-owned Source = %d, want zero", event.source)
	}
}

// 验证 After 到期后只投递普通 Message，并把 Timer 消息 Source 固定为零值。
func TestTimerAfterDeliversMessageWithZeroSource(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	service := &timerRecordingService{events: make(chan timerEvent, 1)}
	serviceID, err := runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}

	args := []any{"before"}
	timerID, err := service.context.After(time.Second, timerTestCommand, args...)
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if timerID == 0 {
		t.Fatal("After() returned zero TimerID")
	}
	args[0] = "after"
	clock.Advance(time.Second)

	event := waitTimerEvent(t, service.events)
	if event.source != 0 {
		t.Fatalf("timer Source = %d, want zero", event.source)
	}
	if event.target != serviceID || event.command != timerTestCommand {
		t.Fatalf("timer event = %#v, want target %d command %d", event, serviceID, timerTestCommand)
	}
	if len(event.args) != 1 || event.args[0] != "before" {
		t.Fatalf("timer args = %#v, want the admitted outer-slice snapshot", event.args)
	}
	metrics := runtime.timers.metricsSnapshot()
	if metrics.Fired != 1 || metrics.Admitted != 1 || metrics.AdmissionRejected != 0 {
		t.Fatalf("timer metrics = %#v, want one admitted expiry", metrics)
	}
}

// 验证 OnStart 中登记的 Timer 只有 Service 发布为 Running 后才会开始计时。
func TestTimerCreatedDuringOnStartArmsAfterPublish(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	service := &timerRecordingService{
		events:        make(chan timerEvent, 1),
		startTimer:    true,
		startDuration: 0,
	}
	if _, err := runtime.CreateService(context.Background(), service); err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	select {
	case event := <-service.events:
		t.Fatalf("OnStart timer fired before fake clock advance: %#v", event)
	default:
	}

	clock.Advance(0)
	event := waitTimerEvent(t, service.events)
	if event.source != 0 {
		t.Fatalf("timer Source = %d, want zero", event.source)
	}
}

// 验证 Timer 从 After 调用时开始计时，OnStart 期间到期后在发布时立即可投递。
func TestTimerDeadlineStartsDuringOnStart(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	service := &timerRecordingService{
		events:        make(chan timerEvent, 1),
		startTimer:    true,
		startDuration: time.Second,
		startEntered:  make(chan struct{}),
		startRelease:  make(chan struct{}),
	}
	created := make(chan error, 1)
	go func() {
		_, err := runtime.CreateService(context.Background(), service)
		created <- err
	}()
	waitSignal(t, service.startEntered)
	clock.Advance(time.Second)
	close(service.startRelease)
	if err := <-created; err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	clock.Advance(0)
	if event := waitTimerEvent(t, service.events); event.source != 0 {
		t.Fatalf("timer Source = %d, want zero", event.source)
	}
}

// 验证 Timer 只能由所属 Service 取消，且取消成功后不会再次触发消息。
func TestTimerCancelRequiresOwnerAndIsOneShot(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	owner := &timerRecordingService{events: make(chan timerEvent, 1)}
	other := &timerRecordingService{events: make(chan timerEvent, 1)}
	if _, err := runtime.CreateService(context.Background(), owner); err != nil {
		t.Fatalf("CreateService(owner) error = %v", err)
	}
	if _, err := runtime.CreateService(context.Background(), other); err != nil {
		t.Fatalf("CreateService(other) error = %v", err)
	}

	timerID, err := owner.context.After(time.Second, timerTestCommand)
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	if other.context.CancelTimer(timerID) {
		t.Fatal("non-owner CancelTimer unexpectedly succeeded")
	}
	if !owner.context.CancelTimer(timerID) {
		t.Fatal("owner CancelTimer returned false")
	}
	if owner.context.CancelTimer(timerID) {
		t.Fatal("second CancelTimer unexpectedly succeeded")
	}
	clock.Advance(time.Second)
	assertNoTimerEvent(t, owner.events)
	metrics := runtime.timers.metricsSnapshot()
	if metrics.Cancelled != 1 || metrics.Fired != 0 {
		t.Fatalf("timer metrics = %#v, want one cancellation and no fire", metrics)
	}
}

// 验证 OnStart 失败和 Service 停止都会取消该 Service 所属的未完成 Timer。
func TestTimerOwnerCleanupCancelsTimers(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	startErr := errors.New("start failed")
	failing := &timerRecordingService{
		events:        make(chan timerEvent, 1),
		startTimer:    true,
		startDuration: time.Second,
		startErr:      startErr,
	}
	if _, err := runtime.CreateService(context.Background(), failing); !errors.Is(err, startErr) {
		t.Fatalf("failed CreateService() error = %v, want %v", err, startErr)
	}
	clock.Advance(time.Second)
	assertNoTimerEvent(t, failing.events)

	working := &timerRecordingService{events: make(chan timerEvent, 1)}
	if _, err := runtime.CreateService(context.Background(), working); err != nil {
		t.Fatalf("CreateService(working) error = %v", err)
	}
	if _, err := working.context.After(time.Second, timerTestCommand); err != nil {
		t.Fatalf("working After() error = %v", err)
	}
	if err := runtime.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	clock.Advance(time.Second)
	assertNoTimerEvent(t, working.events)
	metrics := runtime.timers.metricsSnapshot()
	if metrics.Created != 2 || metrics.Cancelled != 2 || metrics.Fired != 0 {
		t.Fatalf("timer metrics = %#v, want both owner timers cancelled", metrics)
	}
}

// 验证 Timer 到期使用与 Send 相同的有界 Mailbox 接纳路径，失败后不自动重试。
func TestTimerAdmissionFailureIsNotRetried(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{MailboxCapacity: 1})
	service := newBlockingTimerService()
	serviceID, err := runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if err := runtime.Send(serviceID, timerTestCommand, "running"); err != nil {
		t.Fatalf("first Send() error = %v", err)
	}
	waitSignal(t, service.started)
	if err := runtime.Send(serviceID, timerTestCommand, "queued"); err != nil {
		t.Fatalf("second Send() error = %v", err)
	}
	if _, err := service.context.After(0, timerTestCommand, "timer"); err != nil {
		t.Fatalf("After() error = %v", err)
	}
	clock.Advance(0)
	waitTimerMetric(t, runtime, func(metrics timerMetricsSnapshot) bool {
		return metrics.AdmissionRejected == 1
	})

	close(service.release)
	first := waitTimerEvent(t, service.events)
	if len(first.args) != 1 || first.args[0] != "running" {
		t.Fatalf("handled first event = %#v, want running message", first)
	}
	queued := waitTimerEvent(t, service.events)
	if len(queued.args) != 1 || queued.args[0] != "queued" {
		t.Fatalf("handled queued event = %#v, want queued message", queued)
	}
	assertNoTimerEvent(t, service.events)
	metrics := runtime.timers.metricsSnapshot()
	if metrics.Fired != 1 || metrics.Admitted != 0 || metrics.AdmissionRejected != 1 {
		t.Fatalf("timer metrics = %#v, want one rejected fire without retry", metrics)
	}
	failure := runtime.timers.lastAdmissionFailure()
	if failure == nil || failure.Owner != serviceID || !errors.Is(failure.Error, ErrMailboxFull) {
		t.Fatalf("last timer admission failure = %#v, want owner %d and ErrMailboxFull", failure, serviceID)
	}
}

// 验证 CancelTimer 与到期竞争时最多产生一次成功状态转换和一次消息接纳。
func TestTimerCancelAndExpiryRaceIsAtMostOnce(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
	service := &timerRecordingService{events: make(chan timerEvent, 1)}
	if _, err := runtime.CreateService(context.Background(), service); err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	timerID, err := service.context.After(0, timerTestCommand)
	if err != nil {
		t.Fatalf("After() error = %v", err)
	}
	start := make(chan struct{})
	cancelResult := make(chan bool, 1)
	go func() {
		<-start
		cancelResult <- service.context.CancelTimer(timerID)
	}()
	go func() {
		<-start
		clock.Advance(0)
	}()
	close(start)
	cancelled := <-cancelResult
	waitTimerMetric(t, runtime, func(metrics timerMetricsSnapshot) bool {
		return metrics.Cancelled+metrics.Fired == 1
	})
	if eventCount := drainTimerEvents(service.events); eventCount > 1 {
		t.Fatalf("expiry/cancel race delivered %d messages, want at most one", eventCount)
	}
	metrics := runtime.timers.metricsSnapshot()
	if cancelled && metrics.Cancelled != 1 {
		t.Fatalf("CancelTimer reported success but metrics = %#v", metrics)
	}
	if !cancelled && metrics.Fired != 1 {
		t.Fatalf("CancelTimer reported failure but metrics = %#v", metrics)
	}
}

type timerEvent struct {
	source  ServiceID
	target  ServiceID
	command CommandID
	args    []any
}

type timerRecordingService struct {
	context       ServiceContext
	events        chan timerEvent
	startTimer    bool
	startDuration time.Duration
	startErr      error
	startEntered  chan struct{}
	startRelease  chan struct{}
}

func (service *timerRecordingService) OnStart(_ context.Context, serviceContext ServiceContext) error {
	service.context = serviceContext
	if service.startTimer {
		_, err := serviceContext.After(service.startDuration, timerTestCommand, "on-start")
		if err != nil {
			return err
		}
	}
	if service.startEntered != nil {
		close(service.startEntered)
		<-service.startRelease
	}
	return service.startErr
}

func (service *timerRecordingService) Handle(commandContext CommandContext, command CommandID, args ...any) (any, error) {
	if service.events != nil {
		service.events <- timerEvent{
			source:  commandContext.Source(),
			target:  commandContext.Self(),
			command: command,
			args:    args,
		}
	}
	return nil, nil
}

func (*timerRecordingService) OnStop(context.Context) error {
	return nil
}

type blockingTimerService struct {
	context ServiceContext
	started chan struct{}
	release chan struct{}
	events  chan timerEvent
}

func newBlockingTimerService() *blockingTimerService {
	return &blockingTimerService{
		started: make(chan struct{}),
		release: make(chan struct{}),
		events:  make(chan timerEvent, 2),
	}
}

func (service *blockingTimerService) OnStart(_ context.Context, serviceContext ServiceContext) error {
	service.context = serviceContext
	return nil
}

func (service *blockingTimerService) Handle(commandContext CommandContext, command CommandID, args ...any) (any, error) {
	if len(args) == 1 && args[0] == "running" {
		close(service.started)
		<-service.release
	}
	service.events <- timerEvent{
		source:  commandContext.Source(),
		target:  commandContext.Self(),
		command: command,
		args:    args,
	}
	return nil, nil
}

func (*blockingTimerService) OnStop(context.Context) error {
	return nil
}

type fakeClock struct {
	mu     sync.Mutex
	now    time.Time
	timers map[*fakeTimer]struct{}
}

func newFakeClock() *fakeClock {
	return &fakeClock{
		now:    time.Unix(0, 0),
		timers: make(map[*fakeTimer]struct{}),
	}
}

func (clock *fakeClock) Now() time.Time {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return clock.now
}

func (clock *fakeClock) NewTimer(duration time.Duration) runtimeTimer {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	timer := &fakeTimer{
		clock:    clock,
		deadline: clock.now.Add(duration),
		channel:  make(chan time.Time, 1),
	}
	clock.timers[timer] = struct{}{}
	return timer
}

func (clock *fakeClock) Advance(duration time.Duration) {
	clock.mu.Lock()
	clock.now = clock.now.Add(duration)
	now := clock.now
	due := make([]*fakeTimer, 0)
	for timer := range clock.timers {
		if timer.stopped || timer.fired || timer.deadline.After(now) {
			continue
		}
		timer.fired = true
		delete(clock.timers, timer)
		due = append(due, timer)
	}
	clock.mu.Unlock()
	for _, timer := range due {
		timer.channel <- now
	}
}

func (clock *fakeClock) active() int {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	return len(clock.timers)
}

type fakeTimer struct {
	clock    *fakeClock
	deadline time.Time
	channel  chan time.Time
	stopped  bool
	fired    bool
}

func (timer *fakeTimer) C() <-chan time.Time {
	return timer.channel
}

func (timer *fakeTimer) Stop() bool {
	timer.clock.mu.Lock()
	defer timer.clock.mu.Unlock()
	if timer.stopped || timer.fired {
		return false
	}
	timer.stopped = true
	delete(timer.clock.timers, timer)
	return true
}

func newTimerTestRuntime(t *testing.T, clock *fakeClock, config Config) *Runtime {
	t.Helper()
	runtime, err := newRuntimeWithClock(config, clock)
	if err != nil {
		t.Fatalf("newRuntimeWithClock() error = %v", err)
	}
	if err := runtime.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("cleanup Close() error = %v", err)
		}
	})
	return runtime
}

func waitTimerEvent(t *testing.T, events <-chan timerEvent) timerEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("timer event did not arrive")
		return timerEvent{}
	}
}

func assertNoTimerEvent(t *testing.T, events <-chan timerEvent) {
	t.Helper()
	select {
	case event := <-events:
		t.Fatalf("unexpected timer event: %#v", event)
	default:
	}
}

func drainTimerEvents(events <-chan timerEvent) int {
	count := 0
	for {
		select {
		case <-events:
			count++
		default:
			return count
		}
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("signal did not arrive")
	}
}

func waitTimerMetric(t *testing.T, runtime *Runtime, condition func(timerMetricsSnapshot) bool) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		if condition(runtime.timers.metricsSnapshot()) {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("timer metrics condition not reached: %#v", runtime.timers.metricsSnapshot())
		default:
			stdruntime.Gosched()
		}
	}
}
