package runtime_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const (
	serviceTestCommand = runtime.CommandID(0x02000101)
)

// 验证 CreateService 失败只回滚当前实例、消费其 ServiceID，并不影响 Runtime 继续创建服务。
func TestCreateServiceFailureRollsBackOnlyCurrentService(t *testing.T) {
	runtimeInstance := newRunningRuntime(t, runtime.Config{})

	startErr := errors.New("service start failed")
	failing := &lifecycleTestService{startErr: startErr, stopped: make(chan struct{})}
	failedID, err := runtimeInstance.CreateService(context.Background(), failing)
	if failedID != 0 {
		t.Fatalf("failed ServiceID = %d, want zero", failedID)
	}
	if !errors.Is(err, startErr) {
		t.Fatalf("CreateService() error = %v, want %v", err, startErr)
	}
	waitClosed(t, failing.stopped)
	if got := failing.stopCount.Load(); got != 1 {
		t.Fatalf("failed service OnStop count = %d, want 1", got)
	}
	if got := runtimeInstance.State(); got != runtime.RuntimeRunning {
		t.Fatalf("Runtime state after failed service = %v, want Running", got)
	}

	working := &lifecycleTestService{handled: make(chan struct{}, 1)}
	workingID, err := runtimeInstance.CreateService(context.Background(), working)
	if err != nil {
		t.Fatalf("second CreateService() error = %v", err)
	}
	if workingID != 2 {
		t.Fatalf("second ServiceID = %d, want 2 after failed ID 1", workingID)
	}
	if err := runtimeInstance.Send(workingID, serviceTestCommand); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	waitClosed(t, working.handled)
}

// 验证启动期自身 Send 被拒绝，而启动成功后同一 Service 可以正常接收消息。
func TestServiceOnStartCannotSendToItself(t *testing.T) {
	runtimeInstance, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtimeInstance.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	service := &selfSendingService{}
	serviceID, err := runtimeInstance.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if service.selfID != serviceID {
		t.Fatalf("self ID recorded in OnStart = %d, want %d", service.selfID, serviceID)
	}
	if !errors.Is(service.selfSendErr, runtime.ErrServiceStarting) {
		t.Fatalf("self Send error = %v, want ErrServiceStarting", service.selfSendErr)
	}
	if err := runtimeInstance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// 验证本地 Mailbox 按成功接纳顺序串行处理，并在容量耗尽时快速拒绝。
func TestServiceMailboxIsSerialAndBounded(t *testing.T) {
	runtimeInstance := newRunningRuntime(t, runtime.Config{MailboxCapacity: 1})
	service := newSerialService()
	serviceID, err := runtimeInstance.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}

	if err := runtimeInstance.Send(serviceID, serviceTestCommand, "first"); err != nil {
		t.Fatalf("first Send() error = %v", err)
	}
	waitClosed(t, service.firstStarted)
	if err := runtimeInstance.Send(serviceID, serviceTestCommand, "second"); err != nil {
		t.Fatalf("second Send() error = %v", err)
	}
	if err := runtimeInstance.Send(serviceID, serviceTestCommand, "third"); !errors.Is(err, runtime.ErrMailboxFull) {
		t.Fatalf("third Send() error = %v, want ErrMailboxFull", err)
	}

	close(service.releaseFirst)
	if got := <-service.handled; got != "first" {
		t.Fatalf("first handled value = %q, want first", got)
	}
	if got := <-service.handled; got != "second" {
		t.Fatalf("second handled value = %q, want second", got)
	}
	if got := service.maxConcurrent.Load(); got != 1 {
		t.Fatalf("maximum concurrent handlers = %d, want 1", got)
	}
}

// 验证 Runtime 只复制 Args 外层切片，消息接纳后不会受到发送方替换参数槽位的影响。
func TestRuntimeSendCopiesArgumentSlice(t *testing.T) {
	runtimeInstance := newRunningRuntime(t, runtime.Config{})
	service := newArgumentSnapshotService()
	serviceID, err := runtimeInstance.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}

	args := []any{"before"}
	if err := runtimeInstance.Send(serviceID, serviceTestCommand, args...); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	waitClosed(t, service.entered)
	args[0] = "after"
	close(service.read)
	if got := <-service.received; got != "before" {
		t.Fatalf("received argument = %q, want before", got)
	}
}

// 验证 Handler panic 只隔离当前 Service，并让其它 Service 继续接收消息。
func TestHandlerPanicStopsOnlyCurrentService(t *testing.T) {
	runtimeInstance := newRunningRuntime(t, runtime.Config{})
	panicService := &panicTestService{stopped: make(chan struct{})}
	healthyService := &lifecycleTestService{handled: make(chan struct{}, 1)}
	panicID, err := runtimeInstance.CreateService(context.Background(), panicService)
	if err != nil {
		t.Fatalf("CreateService(panic) error = %v", err)
	}
	healthyID, err := runtimeInstance.CreateService(context.Background(), healthyService)
	if err != nil {
		t.Fatalf("CreateService(healthy) error = %v", err)
	}

	if err := runtimeInstance.Send(panicID, serviceTestCommand); err != nil {
		t.Fatalf("panic Send() error = %v", err)
	}
	waitClosed(t, panicService.stopped)
	if err := runtimeInstance.Send(panicID, serviceTestCommand); !errors.Is(err, runtime.ErrServiceClosed) {
		t.Fatalf("Send() to panicked service error = %v, want ErrServiceClosed", err)
	}
	if err := runtimeInstance.Send(healthyID, serviceTestCommand); err != nil {
		t.Fatalf("Send() to healthy service error = %v", err)
	}
	waitClosed(t, healthyService.handled)
}

// 验证 Stop 停止接纳 Service，Close 完成第二阶段并永久禁止再次 Start。
func TestRuntimeStopThenCloseHasTwoLifecycleStages(t *testing.T) {
	runtimeInstance := newRunningRuntime(t, runtime.Config{})
	service := &lifecycleTestService{stopped: make(chan struct{})}
	serviceID, err := runtimeInstance.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}

	if err := runtimeInstance.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	waitClosed(t, service.stopped)
	if got := runtimeInstance.State(); got != runtime.RuntimeStopped {
		t.Fatalf("state after Stop = %v, want Stopped", got)
	}
	if err := runtimeInstance.Send(serviceID, serviceTestCommand); !errors.Is(err, runtime.ErrRuntimeStopped) {
		t.Fatalf("Send() after Stop error = %v, want ErrRuntimeStopped", err)
	}
	if err := runtimeInstance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got := runtimeInstance.State(); got != runtime.RuntimeClosed {
		t.Fatalf("state after Close = %v, want Closed", got)
	}
	if err := runtimeInstance.Start(); !errors.Is(err, runtime.ErrRuntimeClosed) {
		t.Fatalf("Start() after Close error = %v, want ErrRuntimeClosed", err)
	}
}

// 验证停止截止时间到期时保留 Stopped 与残留 runner，待其真实返回后才进入 Closed。
func TestRuntimeCloseWaitsForResidualRunner(t *testing.T) {
	runtimeInstance := newRunningRuntimeWithoutCleanup(t, runtime.Config{ShutdownTimeout: 10 * time.Millisecond})
	service := newBlockingService()
	t.Cleanup(func() {
		service.releaseNow()
		if err := runtimeInstance.Close(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("cleanup Close() error = %v", err)
		}
	})
	serviceID, err := runtimeInstance.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if err := runtimeInstance.Send(serviceID, serviceTestCommand); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	waitClosed(t, service.started)

	if err := runtimeInstance.Stop(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v, want context deadline exceeded", err)
	}
	if got := runtimeInstance.State(); got != runtime.RuntimeStopped {
		t.Fatalf("state after timed Stop = %v, want Stopped", got)
	}
	if err := runtimeInstance.Close(); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() with residual runner error = %v, want context deadline exceeded", err)
	}
	if got := runtimeInstance.State(); got != runtime.RuntimeStopped {
		t.Fatalf("state with residual runner = %v, want Stopped", got)
	}

	service.releaseNow()
	waitClosed(t, service.stopped)
	deadline := time.After(time.Second)
	for runtimeInstance.State() != runtime.RuntimeClosed {
		select {
		case <-deadline:
			t.Fatal("Runtime did not enter Closed after residual runner returned")
		case <-time.After(time.Millisecond):
			if err := runtimeInstance.Close(); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("final Close() error = %v, want context deadline exceeded", err)
			}
		}
	}
}

// 验证 CreateService 只能在 Runtime 的 Running 阶段执行，并拒绝持有类型化 nil 的 Service 接口值。
func TestCreateServiceAdmissionStates(t *testing.T) {
	runtimeInstance, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if _, err := runtimeInstance.CreateService(context.Background(), (*lifecycleTestService)(nil)); !errors.Is(err, runtime.ErrInvalidService) {
		t.Fatalf("typed-nil CreateService() error = %v, want ErrInvalidService", err)
	}
	if _, err := runtimeInstance.CreateService(context.Background(), &lifecycleTestService{}); !errors.Is(err, runtime.ErrRuntimeNotRunning) {
		t.Fatalf("CreateService before Start error = %v, want ErrRuntimeNotRunning", err)
	}
	if err := runtimeInstance.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := runtimeInstance.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := runtimeInstance.CreateService(context.Background(), &lifecycleTestService{}); !errors.Is(err, runtime.ErrRuntimeClosed) {
		t.Fatalf("CreateService after Close error = %v, want ErrRuntimeClosed", err)
	}
}

func newRunningRuntime(t *testing.T, config runtime.Config) *runtime.Runtime {
	t.Helper()
	runtimeInstance := newRunningRuntimeWithoutCleanup(t, config)
	t.Cleanup(func() {
		if err := runtimeInstance.Close(); err != nil {
			t.Errorf("cleanup Close() error = %v", err)
		}
	})
	return runtimeInstance
}

func newRunningRuntimeWithoutCleanup(t *testing.T, config runtime.Config) *runtime.Runtime {
	t.Helper()
	runtimeInstance, err := runtime.NewRuntime(config)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtimeInstance.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	return runtimeInstance
}

func waitClosed(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for service signal")
	}
}

type lifecycleTestService struct {
	startErr  error
	stopErr   error
	stopCount atomic.Int32
	stopped   chan struct{}
	handled   chan struct{}
	stopOnce  sync.Once
}

func (service *lifecycleTestService) OnStart(context.Context, runtime.ServiceContext) error {
	return service.startErr
}

func (service *lifecycleTestService) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	if service.handled != nil {
		service.handled <- struct{}{}
	}
	return nil, nil
}

func (service *lifecycleTestService) OnStop(context.Context) error {
	service.stopCount.Add(1)
	if service.stopped != nil {
		service.stopOnce.Do(func() { close(service.stopped) })
	}
	return service.stopErr
}

type selfSendingService struct {
	selfID      runtime.ServiceID
	selfSendErr error
}

func (service *selfSendingService) OnStart(_ context.Context, serviceContext runtime.ServiceContext) error {
	service.selfID = serviceContext.Self()
	service.selfSendErr = serviceContext.Send(service.selfID, serviceTestCommand)
	return nil
}

func (service *selfSendingService) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, nil
}

func (*selfSendingService) OnStop(context.Context) error {
	return nil
}

type serialService struct {
	firstStarted  chan struct{}
	releaseFirst  chan struct{}
	handled       chan string
	firstOnce     sync.Once
	active        atomic.Int32
	maxConcurrent atomic.Int32
	calls         atomic.Int32
}

type argumentSnapshotService struct {
	entered   chan struct{}
	read      chan struct{}
	received  chan string
	enterOnce sync.Once
}

func newArgumentSnapshotService() *argumentSnapshotService {
	return &argumentSnapshotService{
		entered:  make(chan struct{}),
		read:     make(chan struct{}),
		received: make(chan string, 1),
	}
}

func (*argumentSnapshotService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *argumentSnapshotService) Handle(_ runtime.CommandContext, _ runtime.CommandID, args ...any) (any, error) {
	service.enterOnce.Do(func() { close(service.entered) })
	<-service.read
	service.received <- args[0].(string)
	return nil, nil
}

func (*argumentSnapshotService) OnStop(context.Context) error {
	return nil
}

func newSerialService() *serialService {
	return &serialService{
		firstStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
		handled:      make(chan string, 2),
	}
}

func (*serialService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *serialService) Handle(_ runtime.CommandContext, _ runtime.CommandID, args ...any) (any, error) {
	active := service.active.Add(1)
	for {
		old := service.maxConcurrent.Load()
		if active <= old || service.maxConcurrent.CompareAndSwap(old, active) {
			break
		}
	}
	call := service.calls.Add(1)
	if call == 1 {
		service.firstOnce.Do(func() { close(service.firstStarted) })
		<-service.releaseFirst
	}
	service.handled <- args[0].(string)
	service.active.Add(-1)
	return nil, nil
}

func (*serialService) OnStop(context.Context) error {
	return nil
}

type panicTestService struct {
	stopped  chan struct{}
	stopOnce sync.Once
}

type blockingService struct {
	started     chan struct{}
	release     chan struct{}
	stopped     chan struct{}
	startOnce   sync.Once
	releaseOnce sync.Once
	stopOnce    sync.Once
}

func newBlockingService() *blockingService {
	return &blockingService{
		started: make(chan struct{}),
		release: make(chan struct{}),
		stopped: make(chan struct{}),
	}
}

func (service *blockingService) releaseNow() {
	service.releaseOnce.Do(func() { close(service.release) })
}

func (*blockingService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *blockingService) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	service.startOnce.Do(func() { close(service.started) })
	<-service.release
	return nil, nil
}

func (service *blockingService) OnStop(context.Context) error {
	service.stopOnce.Do(func() { close(service.stopped) })
	return nil
}

func (*panicTestService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (*panicTestService) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	panic("test handler panic")
}

func (service *panicTestService) OnStop(context.Context) error {
	service.stopOnce.Do(func() { close(service.stopped) })
	return nil
}
