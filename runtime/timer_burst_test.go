package runtime

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"
)

// 验证集中到期的 Timer 按普通 Mailbox 容量接纳，计数守恒且成功消息仅处理一次、Source 为零。
func TestRuntimeTimerBurstLoad(t *testing.T) {
	if os.Getenv("GSR_LOAD_TEST") != "1" {
		t.Skip("显式设置 GSR_LOAD_TEST=1 运行 Timer 集中到期测量")
	}
	for _, capacity := range []int{64, 1024} {
		t.Run(fmt.Sprintf("capacity_%d", capacity), func(t *testing.T) {
			const count = 10000
			clock := newFakeClock()
			local := newTimerTestRuntime(t, clock, Config{MailboxCapacity: capacity})
			samples := make(chan timerBurstSample, count)
			_, err := local.CreateService(context.Background(), &timerBurstService{count: count, samples: samples})
			if err != nil {
				t.Fatal(err)
			}
			if local.Inspect().TimerCount != count || clock.active() != count {
				t.Fatal("timers not all armed before burst")
			}
			start := time.Now()
			clock.Advance(time.Second)
			inspection := waitRuntimeInspection(t, local, func(value RuntimeInspection) bool {
				return value.TimerCount == 0 && value.Metrics.TimersFired == count &&
					value.Metrics.TimersAdmitted+value.Metrics.TimerAdmissionRejected == count
			})
			admissionElapsed := time.Since(start)
			metrics := inspection.Metrics
			if metrics.TimersCreated != count || metrics.TimersCancelled != 0 || metrics.TimersAdmitted == 0 {
				t.Fatalf("unexpected metrics: %+v", metrics)
			}
			latencies := make([]time.Duration, 0, metrics.TimersAdmitted)
			seen := make([]bool, count)
			wait, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			for uint64(len(latencies)) < metrics.TimersAdmitted {
				select {
				case sample := <-samples:
					if sample.index < 0 || sample.index >= count || seen[sample.index] || sample.source != 0 {
						t.Fatalf("invalid Timer observation: %+v", sample)
					}
					seen[sample.index] = true
					latencies = append(latencies, sample.observed.Sub(start))
				case <-wait.Done():
					t.Fatalf("handled=%d admitted=%d", len(latencies), metrics.TimersAdmitted)
				}
			}
			if err := local.Close(); err != nil {
				t.Fatal(err)
			}
			if len(samples) != 0 || local.Inspect().TimerCount != 0 {
				t.Fatal("extra message or residual Timer after Close")
			}
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			percentile := func(percent int) time.Duration {
				return latencies[(len(latencies)*percent+99)/100-1]
			}
			t.Logf("capacity=%d fired=%d admitted=%d rejected=%d handled=%d admission_observed=%s arrival_p50=%s arrival_p95=%s arrival_p99=%s arrival_max=%s",
				capacity, metrics.TimersFired, metrics.TimersAdmitted, metrics.TimerAdmissionRejected, len(latencies),
				admissionElapsed, percentile(50), percentile(95), percentile(99), latencies[len(latencies)-1])
		})
	}
}

type timerBurstSample struct {
	index    int
	source   ServiceID
	observed time.Time
}

type timerBurstService struct {
	count   int
	samples chan<- timerBurstSample
}

func (service *timerBurstService) OnStart(_ context.Context, ctx ServiceContext) error {
	// 在 OnStart 登记，共享同一个 fake deadline，Running 发布后才允许投递。
	for index := 0; index < service.count; index++ {
		if _, err := ctx.After(time.Second, timerTestCommand, index); err != nil {
			return err
		}
	}
	return nil
}

func (service *timerBurstService) Handle(ctx CommandContext, _ CommandID, args ...any) (any, error) {
	service.samples <- timerBurstSample{index: args[0].(int), source: ctx.Source(), observed: time.Now()}
	return nil, nil
}

func (service *timerBurstService) OnStop(context.Context) error {
	return nil
}
