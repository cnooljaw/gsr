package runtime

import (
	"bufio"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
)

// 验证暂定每秒一万条输入的短时定速场景，接纳消息恰好处理一次并记录实际速率和到达延迟。
func TestRuntimePaced10000Load(t *testing.T) {
	if os.Getenv("GSR_LOAD_TEST") != "1" {
		t.Skip("显式设置 GSR_LOAD_TEST=1 运行定速负载测量")
	}
	t.Run("logging_disabled", func(t *testing.T) { measurePaced10000(t, false) })
	t.Run("file_per_message", func(t *testing.T) { measurePaced10000(t, true) })
}

func measurePaced10000(t *testing.T, fileLogging bool) {
	t.Helper()
	const rate = 10000
	const attempts = rate * 10
	const batch = 100
	var logger *logging.Logger
	var logPath string
	if fileLogging {
		directory := t.TempDir()
		var err error
		logger, err = logging.New(logging.Config{Directory: directory})
		if err != nil {
			t.Fatal(err)
		}
		logPath = filepath.Join(directory, "gsr.log")
		t.Cleanup(func() {
			wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := logger.Close(wait); err != nil {
				t.Error(err)
			}
		})
	}
	local, err := NewRuntime(Config{Logger: logger})
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
	samples := make(chan pacedSample, attempts)
	id, err := local.CreateService(context.Background(), &pacedLoadService{samples: samples, logger: logger})
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
	handledLatencies := make([]time.Duration, 0, admitted)
	logRejected := 0
	seen := make([]bool, attempts)
	for len(latencies) < admitted {
		select {
		case sample := <-samples:
			if sample.index < 0 || sample.index >= attempts || !accepted[sample.index] || seen[sample.index] {
				t.Fatalf("unexpected or duplicate message: %d", sample.index)
			}
			seen[sample.index] = true
			latencies = append(latencies, sample.latency)
			handledLatencies = append(handledLatencies, sample.handledLatency)
			if errors.Is(sample.logErr, logging.ErrQueueFull) {
				logRejected++
			} else if sample.logErr != nil {
				t.Fatalf("unexpected log failure: %v", sample.logErr)
			}
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
	logLines := 0
	if fileLogging {
		wait, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := logger.Close(wait); err != nil {
			t.Fatal(err)
		}
		if logger.DroppedCount() != uint64(logRejected) || logger.WriteFailureCount() != 0 {
			t.Fatal("log counters differ from observed results")
		}
		file, err := os.Open(logPath)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			logLines++
		}
		if err := errors.Join(scanner.Err(), file.Close()); err != nil {
			t.Fatal(err)
		}
		if logLines != admitted-logRejected {
			t.Fatalf("log lines=%d accepted=%d rejected=%d", logLines, admitted, logRejected)
		}
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	sort.Slice(handledLatencies, func(i, j int) bool { return handledLatencies[i] < handledLatencies[j] })
	percentile := func(percent int) time.Duration {
		return latencies[(len(latencies)*percent+99)/100-1]
	}
	t.Logf("target_rate=%d actual_attempts_per_sec=%.2f capacity=%d attempts=%d admitted=%d full=%d handled=%d input_elapsed=%s arrival_p50=%s arrival_p95=%s arrival_p99=%s arrival_max=%s",
		rate, float64(attempts)/inputElapsed.Seconds(), DefaultConfig().MailboxCapacity, attempts, admitted,
		attempts-admitted, len(latencies), inputElapsed, percentile(50), percentile(95), percentile(99), latencies[len(latencies)-1])
	t.Logf("file_logging=%t log_lines=%d log_rejected=%d handler_observed_p99=%s",
		fileLogging, logLines, logRejected, handledLatencies[(len(handledLatencies)*99+99)/100-1])
}

type pacedSample struct {
	index          int
	latency        time.Duration
	handledLatency time.Duration
	logErr         error
}

type pacedLoadService struct {
	samples chan<- pacedSample
	logger  *logging.Logger
}

func (service *pacedLoadService) OnStart(_ context.Context, ctx ServiceContext) error {
	service.logger = service.logger.WithService(uint64(ctx.Self()))
	return nil
}

func (service *pacedLoadService) Handle(_ CommandContext, _ CommandID, args ...any) (any, error) {
	message := args[0].(hotMessage)
	arrival := time.Since(message.sent)
	var logErr error
	if service.logger != nil {
		logErr = service.logger.Info("message handled", "operation", message.index)
	}
	service.samples <- pacedSample{index: message.index, latency: arrival,
		handledLatency: time.Since(message.sent), logErr: logErr}
	return nil, nil
}

func (service *pacedLoadService) OnStop(context.Context) error {
	return nil
}
