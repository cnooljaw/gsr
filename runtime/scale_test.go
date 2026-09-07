package runtime

import (
	"context"
	"errors"
	stdruntime "runtime"
	"testing"
	"time"
)

// 验证 1,000 个空闲 Service 可以完整创建和关闭，并记录每 Service 独立 runner 的第一轮基线。
func TestRuntimeScale1000ServicesReportsBaseline(t *testing.T) {
	runRuntimeScaleBaseline(t, 1000)
}

// 验证 10,000 个空闲 Service 可以完整创建和关闭，并记录每 Service 独立 runner 的规模趋势。
func TestRuntimeScale10000ServicesReportsBaseline(t *testing.T) {
	runRuntimeScaleBaseline(t, 10000)
}

func runRuntimeScaleBaseline(t *testing.T, serviceCount int) {
	beforeGoroutines := stdruntime.NumGoroutine()
	start := time.Now()
	runtime, err := NewRuntime(Config{MailboxCapacity: 1})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtime.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	for serviceIndex := 0; serviceIndex < serviceCount; serviceIndex++ {
		if _, err := runtime.CreateService(context.Background(), benchmarkService{}); err != nil {
			t.Fatalf("CreateService(%d) error = %v", serviceIndex, err)
		}
	}
	inspection := runtime.Inspect()
	if len(inspection.Services) != serviceCount {
		t.Fatalf("Service count = %d, want %d", len(inspection.Services), serviceCount)
	}
	creationElapsed := time.Since(start)
	afterGoroutines := stdruntime.NumGoroutine()
	var memory stdruntime.MemStats
	stdruntime.ReadMemStats(&memory)
	t.Logf("%d Service baseline: create=%s goroutines_delta=%d heap_alloc=%d bytes", serviceCount, creationElapsed, afterGoroutines-beforeGoroutines, memory.HeapAlloc)
	closeStart := time.Now()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	t.Logf("%d Service close: elapsed=%s", serviceCount, time.Since(closeStart))
	inspection = runtime.Inspect()
	if inspection.State != RuntimeClosed || len(inspection.Services) != 0 || inspection.PendingCallCount != 0 || inspection.TimerCount != 0 {
		t.Fatalf("post-Close inspection = %#v, want Closed with no residual Core resources", inspection)
	}
}

// BenchmarkRuntimeCreateServices1000 测量第一版每 Service 独立 runner 实现在 1,000 个 Service 规模下的表现。
func BenchmarkRuntimeCreateServices1000(b *testing.B) {
	benchmarkRuntimeCreateServices(b, 1000)
}

// BenchmarkRuntimeCreateServices10000 测量第一版每 Service 独立 runner 实现在 10,000 个 Service 规模下的表现。
func BenchmarkRuntimeCreateServices10000(b *testing.B) {
	benchmarkRuntimeCreateServices(b, 10000)
}

// BenchmarkRuntimeSendHotService 测量向单个热点 Service 执行有界 Send 接纳的性能，
// 同时由 Runtime 管理的 runner 消费同一 Mailbox。
func BenchmarkRuntimeSendHotService(b *testing.B) {
	runtime, err := NewRuntime(Config{MailboxCapacity: b.N + 1})
	if err != nil {
		b.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtime.Start(); err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	serviceID, err := runtime.CreateService(context.Background(), benchmarkService{})
	if err != nil {
		b.Fatalf("CreateService() error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for messageIndex := 0; messageIndex < b.N; messageIndex++ {
		if err := runtime.Send(serviceID, scaleSendCommand, messageIndex); err != nil {
			b.Fatalf("Send(%d) error = %v", messageIndex, err)
		}
	}
	b.StopTimer()
	if err := runtime.Close(); err != nil {
		b.Fatalf("Close() error = %v", err)
	}
}

// 验证 100 个并发 PendingCall 在 Runtime Stop 时全部完成清理，并等待目标 Handler 真实返回。
func TestRuntimeScalePendingCallsAreCleanedOnStop(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := newBlockingCallTargetService()
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	const callerCount = 100
	callers := make([]*callService, 0, callerCount)
	for callerIndex := 0; callerIndex < callerCount; callerIndex++ {
		caller := &callService{target: targetID, outcomes: make(chan callOutcome, 1)}
		callerID, err := runtime.CreateService(context.Background(), caller)
		if err != nil {
			t.Fatalf("CreateService(caller %d) error = %v", callerIndex, err)
		}
		if err := runtime.Send(callerID, callTestCommand); err != nil {
			t.Fatalf("Send(caller %d) error = %v", callerIndex, err)
		}
		callers = append(callers, caller)
	}
	waitSignal(t, target.entered)
	inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.PendingCallCount == callerCount
	})
	if inspection.PendingCallCount != callerCount {
		t.Fatalf("PendingCallCount before Stop = %d, want %d", inspection.PendingCallCount, callerCount)
	}
	stopResult := make(chan error, 1)
	go func() {
		stopResult <- runtime.Stop(context.Background())
	}()
	for callerIndex, caller := range callers {
		if outcome := waitCallOutcome(t, caller.outcomes); !errors.Is(outcome.err, ErrRuntimeStopping) {
			t.Fatalf("caller %d outcome error = %v, want ErrRuntimeStopping", callerIndex, outcome.err)
		}
	}
	inspection = waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.PendingCallCount == 0
	})
	if inspection.PendingCallCount != 0 {
		t.Fatalf("PendingCallCount during Stop = %d, want 0", inspection.PendingCallCount)
	}
	target.releaseNow()
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// BenchmarkRuntimeCallRoundTrip 测量本地 Call 从接纳到 Handler 完成的往返性能。
func BenchmarkRuntimeCallRoundTrip(b *testing.B) {
	runtime, err := NewRuntime(Config{MailboxCapacity: b.N + 1})
	if err != nil {
		b.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtime.Start(); err != nil {
		b.Fatalf("Start() error = %v", err)
	}
	targetID, err := runtime.CreateService(context.Background(), benchmarkService{})
	if err != nil {
		b.Fatalf("CreateService(target) error = %v", err)
	}
	caller := &callBenchmarkService{target: targetID, results: make(chan error, 1)}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		b.Fatalf("CreateService(caller) error = %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for messageIndex := 0; messageIndex < b.N; messageIndex++ {
		if err := runtime.Send(callerID, scaleCallCommand); err != nil {
			b.Fatalf("Send(%d) error = %v", messageIndex, err)
		}
		if err := <-caller.results; err != nil {
			b.Fatalf("Call(%d) error = %v", messageIndex, err)
		}
	}
	b.StopTimer()
	if err := runtime.Close(); err != nil {
		b.Fatalf("Close() error = %v", err)
	}
}

// 验证 10,000 个未触发 Timer 可以完成登记、观测和关闭，记录当前 Timer 等待 goroutine 基线。
func TestRuntimeScale10000TimersReportsBaseline(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{MailboxCapacity: 1})
	service := &timerRecordingService{events: make(chan timerEvent, 1)}
	if _, err := runtime.CreateService(context.Background(), service); err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	const timerCount = 10000
	beforeGoroutines := stdruntime.NumGoroutine()
	start := time.Now()
	for timerIndex := 0; timerIndex < timerCount; timerIndex++ {
		if _, err := service.context.After(time.Hour, timerTestCommand, timerIndex); err != nil {
			t.Fatalf("After(%d) error = %v", timerIndex, err)
		}
	}
	creationElapsed := time.Since(start)
	inspection := runtime.Inspect()
	if inspection.TimerCount != timerCount || inspection.Metrics.TimersCreated != timerCount {
		t.Fatalf("Timer inspection = %#v, want %d active and created", inspection, timerCount)
	}
	afterGoroutines := stdruntime.NumGoroutine()
	t.Logf("%d Timer baseline: create=%s goroutines_delta=%d", timerCount, creationElapsed, afterGoroutines-beforeGoroutines)
	closeStart := time.Now()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	t.Logf("%d Timer close: elapsed=%s", timerCount, time.Since(closeStart))
	inspection = runtime.Inspect()
	if inspection.State != RuntimeClosed || inspection.TimerCount != 0 || inspection.Metrics.TimersCancelled != timerCount {
		t.Fatalf("post-Close Timer inspection = %#v, want Closed with all timers cancelled", inspection)
	}
}

// 验证 10,000 个 Timer 的取消与触发竞争保持一次性计数，并通过普通 Mailbox 接纳路径。
func TestRuntimeScaleTimersCancelAndFire(t *testing.T) {
	clock := newFakeClock()
	const timerCount = 10000
	runtime := newTimerTestRuntime(t, clock, Config{MailboxCapacity: timerCount})
	service := &timerRecordingService{events: make(chan timerEvent, timerCount)}
	if _, err := runtime.CreateService(context.Background(), service); err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	timerIDs := make([]TimerID, 0, timerCount)
	for timerIndex := 0; timerIndex < timerCount; timerIndex++ {
		timerID, err := service.context.After(time.Second, timerTestCommand, timerIndex)
		if err != nil {
			t.Fatalf("After(%d) error = %v", timerIndex, err)
		}
		timerIDs = append(timerIDs, timerID)
	}
	for timerIndex := 0; timerIndex < timerCount; timerIndex += 2 {
		if !service.context.CancelTimer(timerIDs[timerIndex]) {
			t.Fatalf("CancelTimer(%d) returned false", timerIndex)
		}
	}
	clock.Advance(time.Second)
	const expectedCancelled = timerCount / 2
	const expectedFired = timerCount - expectedCancelled
	inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.TimerCount == 0 && value.Metrics.TimersFired == expectedFired && value.Metrics.TimersAdmitted == expectedFired
	})
	if inspection.Metrics.TimersCreated != timerCount || inspection.Metrics.TimersCancelled != expectedCancelled || inspection.Metrics.TimersFired != expectedFired || inspection.Metrics.TimersAdmitted != expectedFired || inspection.Metrics.TimerAdmissionRejected != 0 {
		t.Fatalf("Timer lifecycle inspection = %#v, want %d cancelled and %d admitted fires", inspection, expectedCancelled, expectedFired)
	}
}

// BenchmarkRuntimeCreateTimers10000 测量当前每 Timer 一个等待者的实现在 10,000 个 Timer 规模下的表现。
func BenchmarkRuntimeCreateTimers10000(b *testing.B) {
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		clock := newFakeClock()
		runtime, err := newRuntimeWithClock(Config{MailboxCapacity: 1}, clock)
		if err != nil {
			b.Fatalf("newRuntimeWithClock() error = %v", err)
		}
		if err := runtime.Start(); err != nil {
			b.Fatalf("Start() error = %v", err)
		}
		service := &timerRecordingService{events: make(chan timerEvent, 1)}
		if _, err := runtime.CreateService(context.Background(), service); err != nil {
			b.Fatalf("CreateService() error = %v", err)
		}
		for timerIndex := 0; timerIndex < 10000; timerIndex++ {
			if _, err := service.context.After(time.Hour, timerTestCommand, timerIndex); err != nil {
				b.Fatalf("After(%d) error = %v", timerIndex, err)
			}
		}
		if err := runtime.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

const scaleSendCommand CommandID = 0x04000401
const scaleCallCommand CommandID = 0x04000402

type callBenchmarkService struct {
	target  ServiceID
	results chan error
}

func (*callBenchmarkService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *callBenchmarkService) Handle(commandContext CommandContext, _ CommandID, _ ...any) (any, error) {
	_, err := commandContext.Call(context.Background(), service.target, scaleCallCommand)
	service.results <- err
	return nil, nil
}

func (*callBenchmarkService) OnStop(context.Context) error {
	return nil
}

func benchmarkRuntimeCreateServices(b *testing.B, serviceCount int) {
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		runtime, err := NewRuntime(Config{MailboxCapacity: 1})
		if err != nil {
			b.Fatalf("NewRuntime() error = %v", err)
		}
		if err := runtime.Start(); err != nil {
			b.Fatalf("Start() error = %v", err)
		}
		for serviceIndex := 0; serviceIndex < serviceCount; serviceIndex++ {
			if _, err := runtime.CreateService(context.Background(), benchmarkService{}); err != nil {
				b.Fatalf("CreateService(%d) error = %v", serviceIndex, err)
			}
		}
		if err := runtime.Close(); err != nil {
			b.Fatalf("Close() error = %v", err)
		}
	}
}

type benchmarkService struct{}

func (benchmarkService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (benchmarkService) Handle(CommandContext, CommandID, ...any) (any, error) {
	return nil, nil
}

func (benchmarkService) OnStop(context.Context) error {
	return nil
}
