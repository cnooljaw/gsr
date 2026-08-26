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
	// RuntimeStopped has completed shutdown.
	RuntimeStopped
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
	default:
		return "Unknown"
	}
}

// Runtime owns Core configuration and lifecycle.
//
// Core 0 intentionally has no Service, Mailbox, Timer or Cluster behavior.
type Runtime struct {
	config Config

	mu          sync.Mutex
	state       RuntimeState
	startDone   chan struct{}
	closeQueued bool
	closeDone   chan struct{}
	closeResult error
	lifecycle   *lifecycleCoordinator

	serviceIDs *idAllocator
	sessionIDs *idAllocator
	timerIDs   *idAllocator
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
	return &Runtime{
		config:     normalized,
		state:      RuntimeCreated,
		startDone:  make(chan struct{}),
		closeDone:  make(chan struct{}),
		lifecycle:  newCoreLifecycleCoordinator(),
		serviceIDs: newIDAllocator(1),
		sessionIDs: newIDAllocator(1),
		timerIDs:   newIDAllocator(1),
	}, nil
}

// Config returns the immutable Runtime configuration.
func (runtime *Runtime) Config() Config {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.config
}

// State returns the current lifecycle state.
func (runtime *Runtime) State() RuntimeState {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.state
}

// Start starts Runtime-owned components and transitions from Created to
// Running. A failed startup rolls back successful components and permanently
// stops the Runtime.
func (runtime *Runtime) Start() error {
	runtime.mu.Lock()
	switch runtime.state {
	case RuntimeCreated:
		runtime.state = RuntimeStarting
		runtime.mu.Unlock()

		err := runtime.lifecycle.start(context.Background())

		shouldClose := false
		runtime.mu.Lock()
		close(runtime.startDone)
		if err != nil {
			runtime.closeResult = err
			runtime.state = RuntimeStopped
			close(runtime.closeDone)
			runtime.mu.Unlock()
			return err
		}
		if runtime.closeQueued {
			runtime.state = RuntimeStopping
			shouldClose = true
		} else {
			runtime.state = RuntimeRunning
		}
		runtime.mu.Unlock()
		if shouldClose {
			go runtime.finishClose()
		}
		return nil
	case RuntimeStarting:
		runtime.mu.Unlock()
		return ErrRuntimeAlreadyStarted
	case RuntimeRunning:
		runtime.mu.Unlock()
		return ErrRuntimeAlreadyStarted
	case RuntimeStopping:
		runtime.mu.Unlock()
		return ErrRuntimeStopping
	case RuntimeStopped:
		runtime.mu.Unlock()
		return ErrRuntimeStopped
	default:
		runtime.mu.Unlock()
		return ErrRuntimeStopped
	}
}

// Close begins an idempotent Runtime shutdown.
//
// The first caller owns the shutdown workflow. Other callers wait for the
// same result, but their own context may return early without cancelling the
// workflow already in progress.
func (runtime *Runtime) Close(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	for {
		runtime.mu.Lock()
		switch runtime.state {
		case RuntimeStopped:
			result := runtime.closeResult
			runtime.mu.Unlock()
			return result
		case RuntimeStarting:
			runtime.closeQueued = true
			startDone := runtime.startDone
			runtime.mu.Unlock()
			select {
			case <-startDone:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		case RuntimeStopping:
			done := runtime.closeDone
			runtime.mu.Unlock()
			return runtime.waitForClose(ctx, done)
		case RuntimeCreated, RuntimeRunning:
			runtime.state = RuntimeStopping
			done := runtime.closeDone
			runtime.mu.Unlock()

			go runtime.finishClose()
			return runtime.waitForClose(ctx, done)
		default:
			runtime.mu.Unlock()
			return ErrRuntimeStopped
		}
	}
}

func (runtime *Runtime) finishClose() {
	shutdownContext, cancel := context.WithTimeout(context.Background(), runtime.config.ShutdownTimeout)
	defer cancel()
	result := runtime.lifecycle.stop(shutdownContext)

	runtime.mu.Lock()
	runtime.closeResult = result
	runtime.state = RuntimeStopped
	close(runtime.closeDone)
	runtime.mu.Unlock()
}

func (runtime *Runtime) waitForClose(ctx context.Context, done <-chan struct{}) error {
	select {
	case <-done:
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		return runtime.closeResult
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
