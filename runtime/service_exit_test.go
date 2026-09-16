package runtime

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 验证 OnStart 可以创建并调用其它 Service，入口退出不停止新实例，消息来源仍为入口 ID。
func TestServiceCreateAndExitOnStart(t *testing.T) {
	local := newCallTestRuntime(t, Config{})
	var childID, source ServiceID
	var saved ServiceContext
	var stops atomic.Int32
	child := &exitProbe{handle: func(ctx CommandContext) (any, error) { return ctx.Source(), nil }}
	entry := &exitProbe{start: func(ctx context.Context, service ServiceContext) error {
		saved = service
		var err error
		childID, err = service.CreateService(ctx, child)
		if err != nil {
			return err
		}
		value, err := service.Call(ctx, childID, callTestCommand)
		if err != nil {
			return err
		}
		source = value.(ServiceID)
		if _, err := service.After(0, callTestCommand); err != nil {
			return err
		}
		service.Exit()
		service.Exit()
		return nil
	}, stop: func(context.Context) error { stops.Add(1); return nil }}
	entryID, err := local.CreateService(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	waitExitedService(t, local, entryID)
	if source != entryID || stops.Load() != 1 || local.Inspect().TimerCount != 0 || entry.calls.Load() != 0 {
		t.Fatalf("source=%d entry=%d stops=%d inspection=%+v", source, entryID, stops.Load(), local.Inspect())
	}
	if _, err := local.Call(context.Background(), childID, callTestCommand); err != nil {
		t.Fatal(err)
	}
	if err := local.Send(entryID, callTestCommand); !errors.Is(err, ErrServiceClosed) {
		t.Fatal(err)
	}
	// 即使误保留已停止上下文，也不能继续创建新的运行单元。
	if _, err := saved.CreateService(context.Background(), &exitProbe{}); !errors.Is(err, ErrServiceClosed) {
		t.Fatal(err)
	}
}

// 验证 Exit 等当前 Handler 返回后再清理，当前 Call 得到返回值，而已排队的 Call 不执行。
func TestServiceExitCompletesCurrentCallAndRejectsQueue(t *testing.T) {
	local := newCallTestRuntime(t, Config{})
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var stops atomic.Int32
	service := &exitProbe{start: func(_ context.Context, ctx ServiceContext) error {
		_, err := ctx.After(time.Hour, callTestCommand)
		return err
	}, handle: func(ctx CommandContext) (any, error) {
		ctx.Exit()
		ctx.Exit()
		close(entered)
		<-release
		return "finished", nil
	}, stop: func(context.Context) error { stops.Add(1); return nil }}
	id, err := local.CreateService(context.Background(), service)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	first, queued := make(chan callResult, 1), make(chan error, 1)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		value, err := local.Call(ctx, id, callTestCommand)
		first <- callResult{value: value, err: err}
	}()
	defer func() {
		cancel()
		unblock()
		workers.Wait()
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, err := local.Call(ctx, id, callTestCommand)
		queued <- err
	}()
	waitRuntimeInspection(t, local, func(value RuntimeInspection) bool {
		for _, service := range value.Services {
			if service.ID == id {
				return service.MailboxDepth == 1
			}
		}
		return false
	})
	if stops.Load() != 0 {
		t.Fatal("OnStop ran before Handler returned")
	}
	unblock()
	result := <-first
	if result.err != nil || result.value != "finished" {
		t.Fatalf("current Call=%+v", result)
	}
	if err := <-queued; !errors.Is(err, ErrServiceStopping) {
		t.Fatalf("queued Call=%v", err)
	}
	waitExitedService(t, local, id)
	if service.calls.Load() != 1 || stops.Load() != 1 || local.Inspect().PendingCallCount != 0 || local.Inspect().TimerCount != 0 {
		t.Fatal("exit cleanup incomplete")
	}
}

// 验证创建者启动失败不回滚已经创建成功的 Service，失败实例的 Exit 不改变原有错误与清理规则。
func TestServiceCreateSurvivesCreatorStartFailure(t *testing.T) {
	local := newCallTestRuntime(t, Config{})
	failure := errors.New("startup failed")
	var childID ServiceID
	var stops atomic.Int32
	entry := &exitProbe{start: func(ctx context.Context, service ServiceContext) error {
		if _, err := service.CreateService(ctx, &exitProbe{start: func(context.Context, ServiceContext) error { return failure }}); !errors.Is(err, failure) {
			return errors.New("child startup error lost")
		}
		var err error
		childID, err = service.CreateService(ctx, &exitProbe{})
		if err != nil {
			return err
		}
		service.Exit()
		return failure
	}, stop: func(context.Context) error { stops.Add(1); return nil }}
	if id, err := local.CreateService(context.Background(), entry); id != 0 || !errors.Is(err, failure) {
		t.Fatalf("id=%d error=%v", id, err)
	}
	if stops.Load() != 1 {
		t.Fatal("failed startup cleanup repeated")
	}
	if _, err := local.Call(context.Background(), childID, callTestCommand); err != nil {
		t.Fatal(err)
	}
}

// 验证 Handler 创建的实例独立存活，退出者的慢 OnStop 不阻塞当前 Reply 或新实例处理消息。
func TestServiceCreateInHandlerAndExitWithSlowStop(t *testing.T) {
	local := newCallTestRuntime(t, Config{})
	stopEntered, releaseStop := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseStop) }) }
	defer release()
	var stops atomic.Int32
	entry := &exitProbe{
		handle: func(ctx CommandContext) (any, error) {
			child := &exitProbe{handle: func(childContext CommandContext) (any, error) {
				return childContext.Source(), nil
			}}
			id, err := ctx.CreateService(context.Background(), child)
			if err != nil {
				return nil, err
			}
			// 普通 Handler 与 OnStart 一样，以当前 Service 身份通信。
			source, err := ctx.Call(context.Background(), id, callTestCommand)
			if err != nil {
				return nil, err
			}
			if source != ctx.Self() {
				return nil, errors.New("child received incorrect source")
			}
			ctx.Exit()
			return id, nil
		},
		stop: func(context.Context) error {
			stops.Add(1)
			close(stopEntered)
			<-releaseStop
			return nil
		},
	}
	id, err := local.CreateService(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := local.Call(ctx, id, callTestCommand)
	if err != nil {
		t.Fatal(err)
	}
	childID, ok := value.(ServiceID)
	if !ok || childID == 0 || childID == id {
		t.Fatalf("invalid child ID: %v", value)
	}
	select {
	case <-stopEntered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := local.Send(id, callTestCommand); !errors.Is(err, ErrServiceStopping) {
		t.Fatalf("exiting service admission: %v", err)
	}
	if _, err := local.Call(ctx, childID, callTestCommand); err != nil {
		t.Fatalf("child blocked by creator OnStop: %v", err)
	}
	release()
	waitExitedService(t, local, id)
	if _, err := local.Call(ctx, childID, callTestCommand); err != nil {
		t.Fatalf("child stopped with creator: %v", err)
	}
	if stops.Load() != 1 || local.Inspect().PendingCallCount != 0 {
		t.Fatal("exit cleanup incomplete or repeated")
	}
}

// 验证 Exit 尚未完成时 Runtime Stop 仍能结束等待，迟到返回不会覆盖停止错误或重复执行 OnStop。
func TestServiceExitOverlapsRuntimeStop(t *testing.T) {
	local := newCallTestRuntime(t, Config{})
	entered, releaseHandler := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(releaseHandler) }) }
	defer release()
	var stops atomic.Int32
	service := &exitProbe{
		handle: func(ctx CommandContext) (any, error) {
			ctx.Exit()
			close(entered)
			<-releaseHandler
			return "late result", nil
		},
		stop: func(context.Context) error {
			stops.Add(1)
			return nil
		},
	}
	id, err := local.CreateService(context.Background(), service)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan callOutcome, 1)
	callDone := make(chan struct{})
	go func() {
		defer close(callDone)
		value, err := local.Call(ctx, id, callTestCommand)
		result <- callOutcome{value: value, err: err}
	}()
	defer func() {
		cancel()
		release()
		<-callDone
	}()
	waitSignal(t, entered)
	// 取消的是本次停止等待，不是 Runtime 自己的关闭流程。
	waitContext, cancelWait := context.WithCancel(context.Background())
	cancelWait()
	if err := local.Stop(waitContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("Stop wait error: %v", err)
	}
	outcome := waitCallOutcome(t, result)
	if !errors.Is(outcome.err, ErrRuntimeStopping) || outcome.value != nil {
		t.Fatalf("Call result: %+v", outcome)
	}
	if stops.Load() != 0 {
		t.Fatal("OnStop ran before Handler returned")
	}
	release()
	if err := local.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	if stops.Load() != 1 || local.Inspect().PendingCallCount != 0 {
		t.Fatal("stop cleanup incomplete or repeated")
	}
}

func waitExitedService(t *testing.T, local *Runtime, id ServiceID) {
	t.Helper()
	waitRuntimeInspection(t, local, func(value RuntimeInspection) bool {
		for _, service := range value.Services {
			if service.ID == id {
				return service.State == ServiceStateStopped && service.RunnerCompleted
			}
		}
		return false
	})
}

type exitProbe struct {
	start  func(context.Context, ServiceContext) error
	handle func(CommandContext) (any, error)
	stop   func(context.Context) error
	calls  atomic.Int32
}

func (service *exitProbe) OnStart(ctx context.Context, scoped ServiceContext) error {
	if service.start != nil {
		return service.start(ctx, scoped)
	}
	return nil
}

func (service *exitProbe) Handle(ctx CommandContext, _ CommandID, _ ...any) (any, error) {
	service.calls.Add(1)
	if service.handle != nil {
		return service.handle(ctx)
	}
	return nil, nil
}

func (service *exitProbe) OnStop(ctx context.Context) error {
	if service.stop != nil {
		return service.stop(ctx)
	}
	return nil
}
