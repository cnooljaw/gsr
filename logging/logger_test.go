package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"
	"time"
)

// 验证并发生产者共享严格递增的入队序号，绑定来源不创建第二个队列。
func TestLoggerConcurrentOrder(t *testing.T) {
	var output bytes.Buffer
	logger, err := New(Config{Writer: &output, Capacity: 1024})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for id := uint64(1); id <= 8; id++ {
		workers.Add(1)
		go func(id uint64) {
			defer workers.Done()
			log := logger.WithService(id)
			for i := 0; i < 100; i++ {
				if err := log.Info("event", "index", i); err != nil {
					t.Error(err)
				}
			}
		}(id)
	}
	workers.Wait()
	if err := logger.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	for seq := uint64(1); seq <= 800; seq++ {
		var entry record
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if entry.Sequence != seq || entry.Service == 0 || entry.File == "" || entry.Line == 0 || entry.Time.IsZero() {
			t.Fatalf("entry=%+v", entry)
		}
	}
}

// 验证时间和代码位置来自调用处，引用字段在入队前快照，不由写入者延迟读取。
func TestLoggerSnapshotAndCaller(t *testing.T) {
	writer := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	logger, _ := New(Config{Writer: writer, Capacity: 2})
	defer func() { close(writer.release); logger.Close(context.Background()) }()
	fields := map[string]int{"value": 1}
	before := time.Now()
	_, file, line, _ := runtime.Caller(0)
	if err := logger.Info("snapshot", "data", fields); err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	fields["value"] = 2
	<-writer.entered
	var entry record
	if err := json.Unmarshal(writer.first, &entry); err != nil {
		t.Fatal(err)
	}
	if entry.File != file || entry.Line != line+1 || entry.Time.Before(before) || entry.Time.After(after) || !bytes.Contains(entry.Fields, []byte(`"value":1`)) {
		t.Fatalf("entry=%+v", entry)
	}
}

// 验证慢输出不会阻塞入队，满载明确报错，关闭超时后仍能继续等待排空。
func TestLoggerOverflowAndClose(t *testing.T) {
	writer := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	logger, _ := New(Config{Writer: writer, Capacity: 1})
	var once sync.Once
	release := func() { once.Do(func() { close(writer.release) }) }
	defer func() { release(); logger.Close(context.Background()) }()
	if err := logger.Info("first"); err != nil {
		t.Fatal(err)
	}
	<-writer.entered
	if err := logger.Info("second"); err != nil {
		t.Fatal(err)
	}
	if err := logger.Info("third"); !errors.Is(err, ErrQueueFull) || logger.DroppedCount() != 1 {
		t.Fatalf("err=%v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := logger.Close(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if err := logger.Info("closed"); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	release()
	if err := logger.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// 验证写入错误和 panic 不逃出后台任务，关闭返回错误且失败数量可查询。
func TestLoggerWriterFailure(t *testing.T) {
	for _, panicWrite := range []bool{false, true} {
		logger, _ := New(Config{Writer: failingWriter{panicWrite: panicWrite}})
		if err := logger.Error("failure"); err != nil {
			t.Fatal(err)
		}
		if err := logger.Close(context.Background()); err == nil || logger.WriteFailureCount() != 1 {
			t.Fatalf("close=%v", err)
		}
	}
}

type blockingWriter struct {
	entered, release chan struct{}
	first            []byte
	once             sync.Once
}

func (writer *blockingWriter) Write(data []byte) (int, error) {
	writer.once.Do(func() { writer.first = append([]byte(nil), data...); close(writer.entered); <-writer.release })
	return len(data), nil
}

type failingWriter struct{ panicWrite bool }

func (writer failingWriter) Write([]byte) (int, error) {
	if writer.panicWrite {
		panic("writer")
	}
	return 0, io.ErrClosedPipe
}
