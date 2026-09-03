package runtime

import (
	"context"
	stdruntime "runtime"
	"testing"
	"time"
)

// 验证 1,000 个空闲 Service 可以完整创建和关闭，并记录 per-Service runner 的第一轮基线。
func TestRuntimeScale1000ServicesReportsBaseline(t *testing.T) {
	beforeGoroutines := stdruntime.NumGoroutine()
	start := time.Now()
	runtime, err := NewRuntime(Config{MailboxCapacity: 1})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtime.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	for serviceIndex := 0; serviceIndex < 1000; serviceIndex++ {
		if _, err := runtime.CreateService(context.Background(), benchmarkService{}); err != nil {
			t.Fatalf("CreateService(%d) error = %v", serviceIndex, err)
		}
	}
	inspection := runtime.Inspect()
	if len(inspection.Services) != 1000 {
		t.Fatalf("Service count = %d, want 1000", len(inspection.Services))
	}
	creationElapsed := time.Since(start)
	afterGoroutines := stdruntime.NumGoroutine()
	var memory stdruntime.MemStats
	stdruntime.ReadMemStats(&memory)
	t.Logf("1000 Service baseline: create=%s goroutines_delta=%d heap_alloc=%d bytes", creationElapsed, afterGoroutines-beforeGoroutines, memory.HeapAlloc)
	closeStart := time.Now()
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	t.Logf("1000 Service close: elapsed=%s", time.Since(closeStart))
}

// BenchmarkRuntimeCreateServices1000 measures the first per-Service runner
// implementation at the requested 1,000-Service scale.
func BenchmarkRuntimeCreateServices1000(b *testing.B) {
	b.ReportAllocs()
	for iteration := 0; iteration < b.N; iteration++ {
		runtime, err := NewRuntime(Config{MailboxCapacity: 1})
		if err != nil {
			b.Fatalf("NewRuntime() error = %v", err)
		}
		if err := runtime.Start(); err != nil {
			b.Fatalf("Start() error = %v", err)
		}
		for serviceIndex := 0; serviceIndex < 1000; serviceIndex++ {
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
