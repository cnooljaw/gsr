package runtime

import (
	"context"
	"sync"
)

// RuntimeState is the lifecycle state of a Runtime.
type RuntimeState uint8

const (
	// RuntimeCreated is a constructed Runtime that has not started.
	RuntimeCreated RuntimeState = iota
	// RuntimeStarting is starting its Runtime-owned components.
	RuntimeStarting
	// RuntimeRunning accepts Runtime work.
	RuntimeRunning
	// RuntimeStopping rejects new work and is waiting for shutdown.
	RuntimeStopping
	// RuntimeStopped has completed its orderly stop workflow.
	RuntimeStopped
	// RuntimeClosed has released Runtime-owned resources and cannot restart.
	RuntimeClosed
)

// String returns the stable display name of a RuntimeState.
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

// Runtime owns Core configuration, Service admission and lifecycle.
type Runtime struct {
	config Config

	mu          sync.Mutex
	state       RuntimeState
	startDone   chan struct{}
	stopDone    chan struct{}
	stopQueued  bool
	stopStarted bool
	stopResult  error
	closeResult error
	lifecycle   *lifecycleCoordinator
	dispatcher  *localDispatcher
	scheduler   *scheduler

	serviceIDs *idAllocator
	sessionIDs *idAllocator
	timerIDs   *idAllocator

	servicesMu    sync.RWMutex
	services      map[ServiceID]*serviceInstance
	creating      sync.WaitGroup
	creatingCount int
}

// NewRuntime constructs a Runtime in the Created state.
//
// Zero-valued configuration fields receive DefaultConfig values. Negative
// values and other invalid values return ErrInvalidConfig.
func NewRuntime(config Config) (*Runtime, error) {
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{
		config:     normalized,
		state:      RuntimeCreated,
		startDone:  make(chan struct{}),
		stopDone:   make(chan struct{}),
		serviceIDs: newIDAllocator(1),
		sessionIDs: newIDAllocator(1),
		timerIDs:   newIDAllocator(1),
		services:   make(map[ServiceID]*serviceInstance),
	}
	runtime.dispatcher = &localDispatcher{runtime: runtime}
	runtime.scheduler = newScheduler(runtime)
	runtime.lifecycle = newCoreLifecycleCoordinator(
		&serviceLifecycle{runtime: runtime},
		runtime.scheduler,
	)
	return runtime, nil
}

// Config returns the immutable Runtime configuration.
func (runtime *Runtime) Config() Config {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.config
}

// State returns the current Runtime lifecycle state.
func (runtime *Runtime) State() RuntimeState {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.state
}

// Start starts Runtime-owned components and transitions from Created to Running.
// A failed startup rolls back successful components and permanently stops the Runtime.
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

// Stop closes Runtime admission and runs one orderly stop workflow.
// The caller's context controls only this call's wait; it never cancels a
// stop workflow already owned by another caller.
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

// Close performs the final, idempotent Runtime resource release.
//
// Close has no caller context. It uses ShutdownTimeout for the internal stop
// wait and never restarts a Runtime after Stop or Close.
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
