package logging

import (
	"context"
	"errors"
	"io"
	"testing"
)

// 验证短写属于输出故障，关闭保留首个写入错误且不再 Flush 或关闭借用 Writer。
func TestLoggerShortWriteStopsOutput(t *testing.T) {
	writer := &lifecycleWriter{shortWrite: true, flushErr: errors.New("later flush failure")}
	logger, err := New(Config{Writer: writer})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logger.Close(context.Background()) })
	if err := logger.Info("short write"); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(context.Background()); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("Close=%v", err)
	}
	if !errors.Is(logger.Err(), io.ErrShortWrite) || !errors.Is(logger.Info("late"), io.ErrShortWrite) {
		t.Fatal("first output failure not retained")
	}
	if logger.WriteFailureCount() != 1 || writer.writes != 1 || writer.flushes != 0 || writer.closes != 0 {
		t.Fatalf("failures=%d writer=%+v", logger.WriteFailureCount(), writer)
	}
}

// 验证正常关闭只 Flush 一次，Flush 错误或 panic 可查询且不虚构逐条失败数，不关闭借用 Writer。
func TestLoggerFlushAndBorrowedWriterOwnership(t *testing.T) {
	flushErr := errors.New("flush failed")
	for _, scenario := range []struct {
		name       string
		err        error
		panicFlush bool
	}{
		{name: "success"},
		{name: "error", err: flushErr},
		{name: "panic", panicFlush: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			writer := &lifecycleWriter{flushErr: scenario.err, panicFlush: scenario.panicFlush}
			logger, err := New(Config{Writer: writer})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { logger.Close(context.Background()) })
			if err := logger.Info("accepted"); err != nil {
				t.Fatal(err)
			}
			first := logger.Close(context.Background())
			wantFailure := scenario.err != nil || scenario.panicFlush
			if (first != nil) != wantFailure || (scenario.err != nil && !errors.Is(first, scenario.err)) {
				t.Fatalf("Close=%v", first)
			}
			if logger.Err() != first || logger.Close(context.Background()) != first {
				t.Fatal("repeated Close or Err changed result")
			}
			if writer.writes != 1 || writer.flushes != 1 || writer.closes != 0 || logger.WriteFailureCount() != 0 {
				t.Fatalf("writer=%+v failures=%d", writer, logger.WriteFailureCount())
			}
			late := logger.Info("late")
			if wantFailure && late != first {
				t.Fatal("late log lost output failure")
			}
			if !wantFailure && !errors.Is(late, ErrClosed) {
				t.Fatalf("late=%v", late)
			}
		})
	}
}

// lifecycleWriter 仅在唯一日志写入者中访问，测试等待 Close 完成后才检查计数。
type lifecycleWriter struct {
	shortWrite              bool
	panicFlush              bool
	flushErr                error
	writes, flushes, closes int
}

func (writer *lifecycleWriter) Write(data []byte) (int, error) {
	writer.writes++
	if writer.shortWrite {
		return len(data) - 1, nil
	}
	return len(data), nil
}

func (writer *lifecycleWriter) Flush() error {
	writer.flushes++
	if writer.panicFlush {
		panic("flush")
	}
	return writer.flushErr
}

func (writer *lifecycleWriter) Close() error {
	writer.closes++
	return nil
}
