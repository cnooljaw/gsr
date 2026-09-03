package runtime

import (
	"context"
	stdruntime "runtime"
	"testing"
	"time"
)

// 验证 1,000 个空闲 Service 可以完整创建和关闭，并记录 per-Service runner 的第一轮基线。
func TestRuntimeScale1000ServicesReportsBaseline(t *testing.T) {
	runRuntimeScaleBaseline(t, 1000)
}

// 验证 10,000 个空闲 Service 可以完整创建和关闭，并记录 per-Service runner 的规模趋势。
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

// BenchmarkRuntimeCreateServices1000 measures the first per-Service runner
// implementation at the requested 1,000-Service scale.
func BenchmarkRuntimeCreateServices1000(b *testing.B) {
	benchmarkRuntimeCreateServices(b, 1000)
}

// BenchmarkRuntimeCreateServices10000 measures the first per-Service runner
// implementation at the requested 10,000-Service scale.
func BenchmarkRuntimeCreateServices10000(b *testing.B) {
	benchmarkRuntimeCreateServices(b, 10000)
}

// BenchmarkRuntimeSendHotService measures bounded Send admission into one hot
// Service while its Runtime-owned runner consumes the same mailbox.
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

// BenchmarkRuntimeCreateTimers10000 measures the current one-waiter-per-Timer
// implementation at the requested 10,000-Timer scale.
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
