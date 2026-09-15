package logging

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 验证批量测量区分禁用与成功接纳，文件记录数等于接纳数，不能把丢弃算作写入成功。
func TestLoggingBatchAccounting(t *testing.T) {
	for _, mode := range []string{"disabled", "discard", "file"} {
		t.Run(mode, func(t *testing.T) {
			directory := t.TempDir()
			result := runLoggingBatch(t, mode, directory, 128)
			if result.accepted+result.dropped+result.disabled != 128 || result.dropped != 0 {
				t.Fatalf("accounting=%+v", result)
			}
			if mode == "disabled" && result.disabled != 128 {
				t.Fatal("disabled calls counted as accepted")
			}
			if mode == "file" {
				data, err := os.ReadFile(filepath.Join(directory, "gsr.log"))
				if err != nil || bytes.Count(data, []byte{'\n'}) != result.accepted {
					t.Fatalf("file records differ from admission: %v", err)
				}
			}
		})
	}
}

// BenchmarkLoggingBatch 比较固定批次的禁用、编码入队和文件输出成本，包含关闭排空并报告丢弃数。
func BenchmarkLoggingBatch(b *testing.B) {
	const attempts = 10000
	for _, mode := range []string{"disabled", "discard", "file"} {
		b.Run(mode, func(b *testing.B) {
			directory := b.TempDir()
			var total loggingBatchResult
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				result := runLoggingBatch(b, mode, directory, attempts)
				total.accepted += result.accepted
				total.dropped += result.dropped
				total.disabled += result.disabled
			}
			b.StopTimer()
			b.ReportMetric(float64(total.accepted)/float64(b.N), "accepted/batch")
			b.ReportMetric(float64(total.dropped)/float64(b.N), "dropped/batch")
			b.ReportMetric(float64(total.disabled)/float64(b.N), "disabled/batch")
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*attempts), "ns/attempt")
		})
	}
}

type loggingBatchResult struct {
	accepted int
	dropped  int
	disabled int
}

func runLoggingBatch(tb testing.TB, mode, directory string, attempts int) loggingBatchResult {
	tb.Helper()
	var logger *Logger
	var err error
	switch mode {
	case "disabled":
		// nil Logger 是已定义的禁用入口，不启动写入者。
	case "discard":
		logger, err = New(Config{Writer: io.Discard, Capacity: 1024})
	case "file":
		logger, err = New(Config{Directory: directory, Capacity: 1024})
	default:
		tb.Fatalf("unknown mode: %s", mode)
	}
	if err != nil {
		tb.Fatal(err)
	}
	var result loggingBatchResult
	for index := 0; index < attempts; index++ {
		err = logger.Info("battle operation", "operation", index, "state", "ready")
		if errors.Is(err, ErrQueueFull) {
			result.dropped++
			continue
		}
		if err != nil {
			break
		}
		if logger == nil {
			result.disabled++
		} else {
			result.accepted++
		}
	}
	wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 即使记录失败也尝试关闭；输出失败不是正常的队列满载，不能混入性能结果。
	closeErr := logger.Close(wait)
	if err != nil && !errors.Is(err, ErrQueueFull) {
		tb.Fatal(err)
	}
	if closeErr != nil || logger.WriteFailureCount() != 0 || logger.DroppedCount() != uint64(result.dropped) {
		tb.Fatalf("close=%v failures=%d dropped=%d counted=%d", closeErr, logger.WriteFailureCount(), logger.DroppedCount(), result.dropped)
	}
	return result
}
