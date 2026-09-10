package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// 验证 Send/Call 错误产生独立值诊断，未知错误不泄露到 Reply，且每条消息只报告一次。
func TestDiagnosticsHandlerErrors(t *testing.T) {
	r := newCallTestRuntime(t, Config{})
	var typedNil *StatusError
	for _, failure := range []error{errors.New("private failure"), typedNil, NewStatusError(ErrorClassDomain, "room.full", "full")} {
		id, err := r.CreateService(context.Background(), &errorBoundaryService{err: failure})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.Send(id, 1); err != nil {
			t.Fatal(err)
		}
		_, callErr := r.Call(context.Background(), id, 1)
		if !errors.Is(callErr, normalizeCallError(failure)) {
			t.Fatalf("Call error = %v", callErr)
		}
		for index := 0; index < 2; index++ {
			event := awaitDiagnostic(t, r)
			if event.ServiceID != id || event.Source != 0 || event.Command != 1 || (event.Session == 0) != (index == 0) || event.Stack != "" {
				t.Fatalf("diagnostic = %+v", event)
			}
			var status *StatusError
			errors.As(normalizeCallError(failure), &status)
			if event.Class != status.Class || event.Code != status.Code || event.Message != failure.Error() {
				t.Fatalf("error snapshot = %+v", event)
			}
		}
	}
	select {
	case event := <-r.Diagnostics():
		t.Fatalf("duplicate event: %+v", event)
	default:
	}
}

// 验证真实 panic 带本地堆栈，普通返回 panic 状态不伪造堆栈，Reply 不包含私有内容。
func TestDiagnosticsPanicStack(t *testing.T) {
	for _, actualPanic := range []bool{false, true} {
		r := newCallTestRuntime(t, Config{})
		id, err := r.CreateService(context.Background(), &diagnosticService{handle: func(CommandContext) error {
			if actualPanic {
				panic("private panic detail")
			}
			return ErrServicePanicked
		}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = r.Call(context.Background(), id, 1)
		if !errors.Is(err, ErrServicePanicked) || strings.Contains(err.Error(), "private") {
			t.Fatalf("Reply = %v", err)
		}
		event := awaitDiagnostic(t, r)
		if event.Code != ErrorCodeServicePanicked || (event.Stack != "") != actualPanic || len(event.Stack) > 16<<10 {
			t.Fatalf("panic diagnostic = %+v", event)
		}
		if actualPanic && (!strings.Contains(event.Message, "private panic detail") || !strings.Contains(event.Stack, "TestDiagnosticsPanicStack")) {
			t.Fatalf("missing panic context: %+v", event)
		}
	}
}

// 验证无消费者时队列有界且不阻塞业务，关闭后仍可读缓冲事件与丢弃计数。
func TestDiagnosticsOverflowAndClose(t *testing.T) {
	r := newCallTestRuntime(t, Config{})
	id, err := r.CreateService(context.Background(), &errorBoundaryService{err: errors.New("failure")})
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 300; index++ {
		_, _ = r.Call(context.Background(), id, 1)
	}
	if _, err := r.Call(context.Background(), id, 2); err != nil {
		t.Fatal(err)
	}
	if r.Inspect().Metrics.HandlerErrors != 300 || r.DroppedDiagnosticCount() != 44 {
		t.Fatalf("count=%+v dropped=%d", r.Inspect(), r.DroppedDiagnosticCount())
	}
	if r.Diagnostics() != r.Diagnostics() {
		t.Fatal("diagnostics is not a shared stream")
	}
	var closing sync.WaitGroup
	for index := 0; index < 4; index++ {
		closing.Add(1)
		go func() {
			defer closing.Done()
			if err := r.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	closing.Wait()
	count := 0
	for range r.Diagnostics() {
		count++
	}
	if count != 256 || r.DroppedDiagnosticCount() != 44 {
		t.Fatalf("buffer=%d dropped=%d", count, r.DroppedDiagnosticCount())
	}
	var absent *Runtime
	if absent.Diagnostics() != nil || absent.DroppedDiagnosticCount() != 0 {
		t.Fatal("nil Runtime diagnostics")
	}
}

// 验证停止超时不会提前关闭诊断流，残留 Handler 返回后仍可报告并最终关闭。
func TestDiagnosticsLateHandlerAfterStopTimeout(t *testing.T) {
	r := newCallTestRuntime(t, Config{ShutdownTimeout: 20 * time.Millisecond})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	id, err := r.CreateService(context.Background(), &diagnosticService{handle: func(CommandContext) error {
		close(entered)
		<-release
		return errors.New("late failure")
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Send(id, 1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("Handler did not start")
	}
	if err := r.Stop(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop=%v", err)
	}
	_ = r.Close()
	if r.State() == RuntimeClosed {
		t.Fatal("closed with active Handler")
	}
	select {
	case event, ok := <-r.Diagnostics():
		t.Fatalf("early diagnostic/close: %+v %v", event, ok)
	default:
	}
	unblock()
	if event := awaitDiagnostic(t, r); event.Message != "late failure" {
		t.Fatalf("late event=%+v", event)
	}
	deadline := time.Now().Add(time.Second)
	for r.State() != RuntimeClosed && time.Now().Before(deadline) {
		_ = r.Close()
		time.Sleep(time.Millisecond)
	}
	if r.State() != RuntimeClosed {
		t.Fatal("Runtime did not close")
	}
	if _, ok := <-r.Diagnostics(); ok {
		t.Fatal("diagnostic stream remains open")
	}
}

// 验证文本截断、错误格式化 panic 隔离，以及事件不引用后续可变的错误对象。
func TestDiagnosticsTextSnapshot(t *testing.T) {
	r := newCallTestRuntime(t, Config{})
	mutable := &diagnosticTextError{text: strings.Repeat("x", 5000)}
	id, err := r.CreateService(context.Background(), &diagnosticService{handle: func(ctx CommandContext) error {
		if ctx.Source() == 0 {
			return mutable
		}
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = r.Call(context.Background(), id, 1)
	event := awaitDiagnostic(t, r)
	mutable.text = "changed"
	if event.Message != strings.Repeat("x", 4096) {
		t.Fatalf("message length=%d", len(event.Message))
	}
	badID, err := r.CreateService(context.Background(), &errorBoundaryService{err: diagnosticPanicError{}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = r.Call(context.Background(), badID, 1)
	if event := awaitDiagnostic(t, r); event.Message == "" || event.Stack != "" {
		t.Fatalf("formatting failure=%+v", event)
	}
}

// 验证来自 Service 的 Send/Call 诊断保留真实来源，多个 runner 可并发报告。
func TestDiagnosticsServiceSource(t *testing.T) {
	r := newCallTestRuntime(t, Config{})
	target, err := r.CreateService(context.Background(), &errorBoundaryService{err: errors.New("target error")})
	if err != nil {
		t.Fatal(err)
	}
	origin, err := r.CreateService(context.Background(), &diagnosticService{handle: func(ctx CommandContext) error {
		if err := ctx.Send(target, 1); err != nil {
			return err
		}
		_, _ = ctx.Call(context.Background(), target, 1)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Call(context.Background(), origin, 1); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 2; index++ {
		event := awaitDiagnostic(t, r)
		if event.Source != origin || event.ServiceID != target || (event.Session == 0) != (index == 0) {
			t.Fatalf("source event=%+v", event)
		}
	}
	var callers sync.WaitGroup
	for index := 0; index < 16; index++ {
		id, err := r.CreateService(context.Background(), &errorBoundaryService{err: errors.New("concurrent error")})
		if err != nil {
			t.Fatal(err)
		}
		callers.Add(1)
		go func() {
			defer callers.Done()
			_, _ = r.Call(context.Background(), id, 1)
		}()
	}
	callers.Wait()
	for index := 0; index < 16; index++ {
		_ = awaitDiagnostic(t, r)
	}
}

type diagnosticService struct{ handle func(CommandContext) error }

func (*diagnosticService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *diagnosticService) Handle(ctx CommandContext, _ CommandID, _ ...any) (any, error) {
	return nil, service.handle(ctx)
}
func (*diagnosticService) OnStop(context.Context) error {
	return nil
}

type diagnosticTextError struct{ text string }

func (err *diagnosticTextError) Error() string {
	return err.text
}

type diagnosticPanicError struct{}

func (diagnosticPanicError) Error() string {
	panic("bad Error method")
}

func awaitDiagnostic(t *testing.T, r *Runtime) Diagnostic {
	t.Helper()
	select {
	case event, ok := <-r.Diagnostics():
		if !ok {
			t.Fatal("diagnostics closed")
		}
		return event
	case <-time.After(time.Second):
		t.Fatal("missing diagnostic")
		return Diagnostic{}
	}
}
