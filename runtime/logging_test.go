package runtime

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
)

// 验证满载或已失败的日志出口不替换业务结果，不阻塞 Service 和 Runtime 收尾，也不关闭独立诊断。
func TestRuntimeLoggingFailureIsolation(t *testing.T) {
	for _, failed := range []bool{false, true} {
		name := "queue_full"
		if failed {
			name = "writer_failed"
		}
		t.Run(name, func(t *testing.T) {
			writer := &runtimeLogWriter{entered: make(chan struct{}), release: make(chan struct{}), fail: failed}
			var once sync.Once
			release := func() { once.Do(func() { close(writer.release) }) }
			logger, err := logging.New(logging.Config{Writer: writer, Capacity: 1})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				release()
				logger.Close(context.Background())
			})
			if err := logger.Info("blocked write"); err != nil {
				t.Fatal(err)
			}
			select {
			case <-writer.entered:
			case <-time.After(time.Second):
				t.Fatal("writer not entered")
			}
			if err := logger.Info("queued"); err != nil {
				t.Fatal(err)
			}
			wait, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			wantLogErr := logging.ErrQueueFull
			if failed {
				release()
				if err := logger.Close(wait); !errors.Is(err, io.ErrClosedPipe) {
					t.Fatalf("output failure=%v", err)
				}
				wantLogErr = io.ErrClosedPipe
			}
			local := newCallTestRuntime(t, Config{Logger: logger})
			// 失败断言时先释放阻塞 Writer，避免被测代码退化为同步日志导致清理挂住。
			t.Cleanup(release)
			logResults := make(chan error, 2)
			domainErr := NewStatusError(ErrorClassDomain, "room.full", "full")
			for _, businessErr := range []error{nil, domainErr} {
				id, err := local.CreateService(wait, &runtimeLoggingService{log: logger, results: logResults, err: businessErr})
				if err != nil {
					t.Fatal(err)
				}
				value, err := local.Call(wait, id, 1)
				if !errors.Is(err, businessErr) || (businessErr == nil && value != "business result") {
					t.Fatalf("value=%v error=%v want=%v", value, err, businessErr)
				}
				select {
				case logErr := <-logResults:
					if !errors.Is(logErr, wantLogErr) {
						t.Fatalf("log error=%v", logErr)
					}
				case <-wait.Done():
					t.Fatal("business logging blocked")
				}
				if businessErr != nil {
					event := awaitDiagnostic(t, local)
					if event.ServiceID != id || event.Class != ErrorClassDomain || event.Code != "room.full" {
						t.Fatalf("independent diagnostic changed: %+v", event)
					}
				}
			}
			if err := local.Stop(wait); err != nil {
				t.Fatal(err)
			}
			if err := local.Close(); err != nil {
				t.Fatal(err)
			}
			if local.DroppedDiagnosticCount() != 0 {
				t.Fatal("log failure counted as diagnostic overflow")
			}
			if failed {
				if logger.DroppedCount() != 0 || logger.WriteFailureCount() != 2 {
					t.Fatal("failed output accounting changed")
				}
				return
			}
			if logger.DroppedCount() != 3 || logger.Err() != nil {
				t.Fatalf("dropped=%d err=%v", logger.DroppedCount(), logger.Err())
			}
			// Runtime 退出后，应用仍拥有未结束的 Logger；只有应用发起 Close 才停止接纳。
			if err := logger.Info("still owned by application"); !errors.Is(err, logging.ErrQueueFull) {
				t.Fatalf("Runtime closed borrowed Logger: %v", err)
			}
			release()
			if err := logger.Close(wait); err != nil {
				t.Fatal(err)
			}
		})
	}
}

type runtimeLogWriter struct {
	entered, release chan struct{}
	once             sync.Once
	fail             bool
}

func (writer *runtimeLogWriter) Write(data []byte) (int, error) {
	writer.once.Do(func() { close(writer.entered) })
	<-writer.release
	if writer.fail {
		return 0, io.ErrClosedPipe
	}
	return len(data), nil
}

type runtimeLoggingService struct {
	log     *logging.Logger
	results chan<- error
	err     error
}

func (service *runtimeLoggingService) OnStart(_ context.Context, ctx ServiceContext) error {
	service.log = service.log.WithService(uint64(ctx.Self()))
	return nil
}

func (service *runtimeLoggingService) Handle(_ CommandContext, _ CommandID, _ ...any) (any, error) {
	service.results <- service.log.Info("business event")
	return "business result", service.err
}

func (service *runtimeLoggingService) OnStop(context.Context) error {
	return nil
}
