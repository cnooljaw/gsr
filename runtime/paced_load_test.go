package runtime

import (
	"context"
	"errors"
	"os"
	"sort"
	"testing"
	"time"
)

// 验证暂定每秒一万条输入的短时定速场景，接纳消息恰好处理一次并记录实际速率和到达延迟。
func TestRuntimePaced10000Load(t *testing.T) {
	if os.Getenv("GSR_LOAD_TEST") != "1" {
		t.Skip("显式设置 GSR_LOAD_TEST=1 运行定速负载测量")
	}
	const rate = 10000
	const attempts = rate * 10
	const batch = 100
	local, err := NewRuntime(Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	samples := make(chan hotSample, attempts)
	id, err := local.CreateService(context.Background(), &hotLoadService{samples: samples})
	if err != nil {
		t.Fatal(err)
	}
	accepted := make([]bool, attempts)
	admitted := 0
	// 每 10ms 投入一批 100 条。调度落后时不追赶补发，最终报告实际发送速率。
	start := time.Now()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	budget, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for first := 0; first < attempts; first += batch {
		select {
		case <-ticker.C:
		case <-budget.Done():
			t.Fatalf("load generator could not finish within budget: attempts=%d", first)
		}
		for index := first; index < first+batch; index++ {
			err := local.Send(id, scaleSendCommand, hotMessage{index: index, sent: time.Now()})
			if err == nil {
				accepted[index] = true
				admitted++
				continue
			}
			if !errors.Is(err, ErrMailboxFull) {
				t.Fatalf("unexpected Send failure: %v", err)
			}
		}
	}
	inputElapsed := time.Since(start)
	ticker.Stop()
	if admitted == 0 {
		t.Fatal("no messages admitted")
	}
	latencies := make([]time.Duration, 0, admitted)
	seen := make([]bool, attempts)
	for len(latencies) < admitted {
		select {
		case sample := <-samples:
			if sample.index < 0 || sample.index >= attempts || !accepted[sample.index] || seen[sample.index] {
				t.Fatalf("unexpected or duplicate message: %d", sample.index)
			}
			seen[sample.index] = true
			latencies = append(latencies, sample.latency)
		case <-budget.Done():
			t.Fatalf("handled=%d admitted=%d", len(latencies), admitted)
		}
	}
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if len(samples) != 0 {
		t.Fatal("extra observations after Close")
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	percentile := func(percent int) time.Duration {
		return latencies[(len(latencies)*percent+99)/100-1]
	}
	t.Logf("target_rate=%d actual_attempts_per_sec=%.2f capacity=%d attempts=%d admitted=%d full=%d handled=%d input_elapsed=%s arrival_p50=%s arrival_p95=%s arrival_p99=%s arrival_max=%s",
		rate, float64(attempts)/inputElapsed.Seconds(), DefaultConfig().MailboxCapacity, attempts, admitted,
		attempts-admitted, len(latencies), inputElapsed, percentile(50), percentile(95), percentile(99), latencies[len(latencies)-1])
}
