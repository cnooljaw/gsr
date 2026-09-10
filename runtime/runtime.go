package runtime

import (
	"context"
	"sync"
	"sync/atomic"
)

// RuntimeState 表示 Runtime 的生命周期状态。
type RuntimeState uint8

const (
	// RuntimeCreated 表示 Runtime 已构造但尚未启动。
	RuntimeCreated RuntimeState = iota
	// RuntimeStarting 表示正在启动 Runtime 所管理的组件。
	RuntimeStarting
	// RuntimeRunning 表示 Runtime 正在接纳工作。
	RuntimeRunning
	// RuntimeStopping 表示拒绝新工作并等待停止完成。
	RuntimeStopping
	// RuntimeStopped 表示有序停止流程已完成。
	RuntimeStopped
	// RuntimeClosed 表示 Runtime 资源已释放，且不能重新启动。
	RuntimeClosed
)

// String 返回 RuntimeState 的稳定显示名称。
func (state RuntimeState) String() string {
	switch state {
	case RuntimeCreated:
		return "Created"
	case RuntimeStarting:
		return "Starting"
	case RuntimeRunning:
		return "Running"
	case RuntimeStopping:
		return "Stopping"
	case RuntimeStopped:
		return "Stopped"
	case RuntimeClosed:
		return "Closed"
	default:
		return "Unknown"
	}
}

// Runtime 管理 Core 配置、Service 接纳与生命周期。
type Runtime struct {
	config Config

	mu                 sync.Mutex
	state              RuntimeState
	startDone          chan struct{}
	stopDone           chan struct{}
	stopQueued         bool
	stopStarted        bool
	stopResult         error
	closeResult        error
	lifecycle          *lifecycleCoordinator
	dispatcher         *localDispatcher
	timers             *timerManager
	pendingCalls       *pendingCallManager
	scheduler          *scheduler
	metrics            runtimeMetrics
	diagnostics        chan Diagnostic
	droppedDiagnostics atomic.Uint64

	serviceIDs *idAllocator
	sessionIDs *idAllocator
	timerIDs   *idAllocator

	servicesMu    sync.RWMutex
	services      map[ServiceID]*serviceInstance
	creating      sync.WaitGroup
	creatingCount int
}

// NewRuntime 构造处于 Created 状态的 Runtime。
//
// 零值配置字段使用 DefaultConfig 中的默认值。
// 负值及其它无效值返回 ErrInvalidConfig。
func NewRuntime(config Config) (*Runtime, error) {
	return newRuntimeWithClock(config, realClock{})
}

func newRuntimeWithClock(config Config, clock runtimeClock) (*Runtime, error) {
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{
		config:      normalized,
		state:       RuntimeCreated,
		startDone:   make(chan struct{}),
		stopDone:    make(chan struct{}),
		serviceIDs:  newIDAllocator(1),
		sessionIDs:  newIDAllocator(1),
		timerIDs:    newIDAllocator(1),
		services:    make(map[ServiceID]*serviceInstance),
		diagnostics: make(chan Diagnostic, diagnosticCapacity),
	}
	runtime.dispatcher = &localDispatcher{runtime: runtime}
	runtime.timers = newTimerManager(runtime, clock)
	runtime.pendingCalls = newPendingCallManager(runtime)
	runtime.scheduler = newScheduler(runtime)
	runtime.lifecycle = newCoreLifecycleCoordinator(
		runtime.timers,
		runtime.pendingCalls,
		&serviceLifecycle{runtime: runtime},
		runtime.scheduler,
	)
	return runtime, nil
}

// Config 返回不可变的 Runtime 配置。
func (runtime *Runtime) Config() Config {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.config
}

// State 返回 Runtime 当前的生命周期状态。
func (runtime *Runtime) State() RuntimeState {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.state
}

// Start 启动 Runtime 所管理的组件，将状态从 Created 转为 Running。
// 启动失败会回滚已成功启动的组件，并永久停止该 Runtime。
func (runtime *Runtime) Start() error {
	runtime.mu.Lock()
	switch runtime.state {
	case RuntimeCreated:
		runtime.state = RuntimeStarting
		runtime.mu.Unlock()
	default:
		runtime.mu.Unlock()
		return runtime.startStateError()
	}

	err := runtime.lifecycle.start(context.Background())

	runtime.mu.Lock()
	close(runtime.startDone)
	if err != nil {
		runtime.stopResult = err
		runtime.state = RuntimeStopped
		close(runtime.stopDone)
		runtime.mu.Unlock()
		return err
	}
	if runtime.stopQueued {
		runtime.state = RuntimeStopping
		runtime.mu.Unlock()
		runtime.beginStop()
		return nil
	}
	runtime.state = RuntimeRunning
	runtime.mu.Unlock()
	return nil
}

func (runtime *Runtime) startStateError() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	switch runtime.state {
	case RuntimeStarting, RuntimeRunning:
		return ErrRuntimeAlreadyStarted
	case RuntimeStopping:
		return ErrRuntimeStopping
	case RuntimeStopped:
		return ErrRuntimeStopped
	case RuntimeClosed:
		return ErrRuntimeClosed
	default:
		return ErrRuntimeStopped
	}
}

// Stop 关闭 Runtime 接纳入口，并执行一次有序停止流程。
// 调用方 context 只控制本次调用的等待，
// 不会取消已由其它调用方发起的停止流程。
func (runtime *Runtime) Stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		runtime.mu.Lock()
		switch runtime.state {
		case RuntimeCreated:
			runtime.state = RuntimeStopping
			runtime.mu.Unlock()
			runtime.beginStop()
			return runtime.waitForStop(ctx)
		case RuntimeStarting:
			runtime.stopQueued = true
			startDone := runtime.startDone
			runtime.mu.Unlock()
			select {
			case <-startDone:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		case RuntimeRunning:
			runtime.state = RuntimeStopping
			runtime.mu.Unlock()
			runtime.beginStop()
			return runtime.waitForStop(ctx)
		case RuntimeStopping:
			done := runtime.stopDone
			runtime.mu.Unlock()
			return runtime.waitForStopChannel(ctx, done)
		case RuntimeStopped:
			result := runtime.stopResult
			runtime.mu.Unlock()
			return result
		case RuntimeClosed:
			runtime.mu.Unlock()
			return ErrRuntimeClosed
		default:
			runtime.mu.Unlock()
			return ErrRuntimeStopped
		}
	}
}

// Close 幂等地执行 Runtime 的最终资源释放。
//
// Close 不接收调用方 context，内部停止等待使用 ShutdownTimeout。
// Runtime 在 Stop 或 Close 后都不能重新启动。
func (runtime *Runtime) Close() error {
	shutdownContext, cancel := context.WithTimeout(context.Background(), runtime.config.ShutdownTimeout)
	defer cancel()

	for {
		runtime.mu.Lock()
		switch runtime.state {
		case RuntimeCreated, RuntimeRunning:
			runtime.state = RuntimeStopping
			runtime.mu.Unlock()
			runtime.beginStop()
		case RuntimeStarting:
			runtime.stopQueued = true
			startDone := runtime.startDone
			runtime.mu.Unlock()
			select {
			case <-startDone:
				continue
			case <-shutdownContext.Done():
				return shutdownContext.Err()
			}
		case RuntimeStopping:
			done := runtime.stopDone
			runtime.mu.Unlock()
			if err := waitForSignal(shutdownContext, done); err != nil {
				return err
			}
		case RuntimeStopped:
			result := runtime.stopResult
			runtime.mu.Unlock()
			if !runtime.resourcesReleased() {
				return result
			}
			runtime.mu.Lock()
			if runtime.state != RuntimeStopped {
				result := runtime.closeResult
				runtime.mu.Unlock()
				return result
			}
			runtime.closeResult = result
			runtime.state = RuntimeClosed
			close(runtime.diagnostics)
			runtime.mu.Unlock()
			return result
		case RuntimeClosed:
			result := runtime.closeResult
			runtime.mu.Unlock()
			return result
		default:
			runtime.mu.Unlock()
			return ErrRuntimeStopped
		}
	}
}

func (runtime *Runtime) beginStop() {
	runtime.mu.Lock()
	if runtime.stopStarted {
		runtime.mu.Unlock()
		return
	}
	runtime.stopStarted = true
	runtime.mu.Unlock()
	go runtime.finishStop()
}

func (runtime *Runtime) finishStop() {
	shutdownContext, cancel := context.WithTimeout(context.Background(), runtime.config.ShutdownTimeout)
	defer cancel()
	result := runtime.lifecycle.stop(shutdownContext)
	runtime.recordStopResult(result)

	runtime.mu.Lock()
	runtime.stopResult = result
	if runtime.state != RuntimeClosed {
		runtime.state = RuntimeStopped
	}
	close(runtime.stopDone)
	runtime.mu.Unlock()
}

func (runtime *Runtime) waitForStop(ctx context.Context) error {
	runtime.mu.Lock()
	done := runtime.stopDone
	runtime.mu.Unlock()
	return runtime.waitForStopChannel(ctx, done)
}

func (runtime *Runtime) waitForStopChannel(ctx context.Context, done <-chan struct{}) error {
	if err := waitForSignal(ctx, done); err != nil {
		return err
	}
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.stopResult
}

func waitForSignal(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (runtime *Runtime) nextServiceID() (ServiceID, error) {
	value, err := runtime.serviceIDs.next()
	return ServiceID(value), err
}

func (runtime *Runtime) nextSessionID() (SessionID, error) {
	value, err := runtime.sessionIDs.next()
	return SessionID(value), err
}

func (runtime *Runtime) nextTimerID() (TimerID, error) {
	value, err := runtime.timerIDs.next()
	return TimerID(value), err
}
