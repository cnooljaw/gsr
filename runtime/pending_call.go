package runtime

import (
	"context"
	"sync"
)

// callResult is the single completion value delivered to one waiting Call.
// The channel is buffered so completion never needs the caller runner to be
// scheduled at the exact moment a Handler returns.
type callResult struct {
	value any
	err   error
}

type pendingCall struct {
	session SessionID
	caller  *serviceInstance
	target  ServiceID
	done    chan callResult
}

// pendingCallManager owns the Runtime-local SessionID table. It never invokes
// business code; it only removes a pending entry and publishes one result.
type pendingCallManager struct {
	runtime *Runtime

	mu       sync.Mutex
	pending  map[SessionID]*pendingCall
	stopping bool

	stopStarted bool
	stopDone    chan struct{}
	stopResult  error
}

func newPendingCallManager(runtime *Runtime) *pendingCallManager {
	return &pendingCallManager{
		runtime:  runtime,
		pending:  make(map[SessionID]*pendingCall),
		stopDone: make(chan struct{}),
	}
}

func (*pendingCallManager) lifecycleName() string {
	return "pending-calls"
}

func (*pendingCallManager) start(context.Context) error {
	return nil
}

func (manager *pendingCallManager) begin(caller *serviceInstance, target ServiceID) (*pendingCall, error) {
	if manager == nil || manager.runtime == nil {
		return nil, ErrRuntimeClosed
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.stopping {
		return nil, ErrRuntimeStopping
	}
	if manager.pending == nil {
		manager.pending = make(map[SessionID]*pendingCall)
	}
	session, err := manager.runtime.nextSessionID()
	if err != nil {
		return nil, err
	}
	pending := &pendingCall{
		session: session,
		caller:  caller,
		target:  target,
		done:    make(chan callResult, 1),
	}
	manager.pending[session] = pending
	return pending, nil
}

func (manager *pendingCallManager) complete(session SessionID, result callResult) bool {
	pending, ok := manager.take(session)
	if !ok {
		return false
	}
	pending.done <- result
	return true
}

func (manager *pendingCallManager) cancel(session SessionID, err error) bool {
	return manager.complete(session, callResult{err: err})
}

func (manager *pendingCallManager) take(session SessionID) (*pendingCall, bool) {
	if manager == nil || session == 0 {
		return nil, false
	}
	manager.mu.Lock()
	pending, ok := manager.pending[session]
	if ok {
		delete(manager.pending, session)
	}
	manager.mu.Unlock()
	return pending, ok
}

func (manager *pendingCallManager) failCaller(caller *serviceInstance, err error) int {
	if caller == nil {
		return 0
	}
	return manager.failMatching(func(pending *pendingCall) bool {
		return pending.caller == caller
	}, err)
}

func (manager *pendingCallManager) failTarget(target ServiceID, err error) int {
	if target == 0 {
		return 0
	}
	return manager.failMatching(func(pending *pendingCall) bool {
		return pending.target == target
	}, err)
}

func (manager *pendingCallManager) failMatching(matches func(*pendingCall) bool, err error) int {
	if manager == nil || matches == nil {
		return 0
	}
	manager.mu.Lock()
	matched := make([]*pendingCall, 0)
	for session, pending := range manager.pending {
		if !matches(pending) {
			continue
		}
		delete(manager.pending, session)
		matched = append(matched, pending)
	}
	manager.mu.Unlock()
	for _, pending := range matched {
		pending.done <- callResult{err: err}
	}
	return len(matched)
}

func (manager *pendingCallManager) stop(ctx context.Context) error {
	if manager == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	manager.mu.Lock()
	if manager.stopStarted {
		done := manager.stopDone
		manager.mu.Unlock()
		if err := waitForSignal(ctx, done); err != nil {
			return err
		}
		manager.mu.Lock()
		result := manager.stopResult
		manager.mu.Unlock()
		return result
	}
	manager.stopStarted = true
	manager.stopping = true
	pending := make([]*pendingCall, 0, len(manager.pending))
	for session, call := range manager.pending {
		delete(manager.pending, session)
		pending = append(pending, call)
	}
	manager.mu.Unlock()

	for _, call := range pending {
		call.done <- callResult{err: ErrRuntimeStopping}
	}

	manager.mu.Lock()
	manager.stopResult = nil
	close(manager.stopDone)
	manager.mu.Unlock()
	return nil
}

func (manager *pendingCallManager) hasPending() bool {
	if manager == nil {
		return false
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.pending) > 0
}

func (manager *pendingCallManager) count() int {
	if manager == nil {
		return 0
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.pending)
}

func (runtime *Runtime) failPendingCallsFor(instance *serviceInstance) {
	if runtime == nil || runtime.pendingCalls == nil || instance == nil {
		return
	}
	runtime.pendingCalls.failCaller(instance, ErrServiceStopping)
	runtime.pendingCalls.failTarget(instance.id, ErrServiceStopping)
}
