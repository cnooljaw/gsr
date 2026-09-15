package logging

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// 验证并发文件日志的接纳记录恰好输出一次，文件顺序遵循入队序号，满载丢弃单独计数。
func TestLoggerConcurrentFileLoad(t *testing.T) {
	if os.Getenv("GSR_LOAD_TEST") != "1" {
		t.Skip("显式设置 GSR_LOAD_TEST=1 运行并发文件日志负载")
	}
	const producers = 8
	const perProducer = 12500
	directory := t.TempDir()
	logger, err := New(Config{Directory: directory})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := logger.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	// 每个生产者只写自己的计数和接纳表，主测试在 Wait 后读取。
	accepted := make([][]bool, producers)
	dropped := make([]int, producers)
	for producer := range accepted {
		accepted[producer] = make([]bool, perProducer)
	}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for producer := range accepted {
		workers.Add(1)
		go func(producer int) {
			defer workers.Done()
			bound := logger.WithService(uint64(producer + 1))
			<-start
			for index := 0; index < perProducer; index++ {
				err := bound.Info("concurrent operation", "index", index)
				if errors.Is(err, ErrQueueFull) {
					dropped[producer]++
					continue
				}
				if err != nil {
					t.Error(err)
					return
				}
				accepted[producer][index] = true
			}
		}(producer)
	}
	started := time.Now()
	close(start)
	workers.Wait()
	inputElapsed := time.Since(started)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := logger.Close(ctx); err != nil {
		t.Fatal(err)
	}
	drainElapsed := time.Since(started) - inputElapsed
	file, err := os.Open(filepath.Join(directory, "gsr.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	lines := 0
	for {
		var entry record
		err := decoder.Decode(&entry)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var fields struct {
			Index *int `json:"index"`
		}
		if err := json.Unmarshal(entry.Fields, &fields); err != nil {
			t.Fatal(err)
		}
		if entry.Sequence != uint64(lines+1) || entry.Service < 1 || entry.Service > producers ||
			fields.Index == nil || *fields.Index < 0 || *fields.Index >= perProducer {
			t.Fatalf("invalid record: %+v", entry)
		}
		pending := accepted[entry.Service-1]
		if !pending[*fields.Index] {
			t.Fatalf("duplicate or rejected record: service=%d index=%d", entry.Service, *fields.Index)
		}
		pending[*fields.Index] = false
		lines++
	}
	totalDropped := 0
	for producer, pending := range accepted {
		totalDropped += dropped[producer]
		for index, missing := range pending {
			if missing {
				t.Fatalf("accepted record missing: producer=%d index=%d", producer, index)
			}
		}
	}
	if lines+totalDropped != producers*perProducer || logger.DroppedCount() != uint64(totalDropped) || logger.WriteFailureCount() != 0 {
		t.Fatalf("lines=%d dropped=%d logger_dropped=%d failures=%d", lines, totalDropped, logger.DroppedCount(), logger.WriteFailureCount())
	}
	t.Logf("producers=%d attempts=%d written=%d dropped=%d input_elapsed=%s drain_elapsed=%s", producers, producers*perProducer, lines, totalDropped, inputElapsed, drainElapsed)
}
