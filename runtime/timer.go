package runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// runtimeClock is the small clock seam owned by Core. Production uses the
// monotonic clock behind time.Timer; tests can provide a deterministic clock.
type runtimeClock interface {
	Now() time.Time
	NewTimer(time.Duration) runtimeTimer
}

type runtimeTimer interface {
	C() <-chan time.Time
	Stop() bool
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

func (realClock) NewTimer(duration time.Duration) runtimeTimer {
	return realTimer{timer: time.NewTimer(duration)}
}

type realTimer struct {
	timer *time.Timer
}

func (timer realTimer) C() <-chan time.Time {
	return timer.timer.C
}

func (timer realTimer) Stop() bool {
	return timer.timer.Stop()
}

type timerState uint8

const (
	timerScheduled timerState = iota
	timerCancelled
	timerFired
)

type timerEntry struct {
	id       TimerID
	owner    *serviceInstance
	command  CommandID
	args     []any
	deadline time.Time

	cancel chan struct{}
	timer  runtimeTimer
	armed  bool
	state  timerState
}

// timerMetrics is kept with the Timer owner so Core4 Inspection can expose a
// consistent snapshot without making the scheduling path depend on metrics.
type timerMetrics struct {
	created           atomic.Uint64
	cancelled         atomic.Uint64
	fired             atomic.Uint64
	admitted          atomic.Uint64
	admissionRejected atomic.Uint64
}

type timerMetricsSnapshot struct {
	Created           uint64
	Cancelled         uint64
	Fired             uint64
	Admitted          uint64
	AdmissionRejected uint64
}

type timerAdmissionFailure struct {
	TimerID TimerID
	Owner   ServiceID
	Error   error
}

func (metrics *timerMetrics) snapshot() timerMetricsSnapshot {
	return timerMetricsSnapshot{
		Created:           metrics.created.Load(),
		Cancelled:         metrics.cancelled.Load(),
		Fired:             metrics.fired.Load(),
		Admitted:          metrics.admitted.Load(),
		AdmissionRejected: metrics.admissionRejected.Load(),
	}
}

// timerManager owns one-shot timers and only creates ordinary local Messages
// when a timer expires. It never invokes a business callback directly.
type timerManager struct {
	runtime *Runtime
	clock   runtimeClock

	mu       sync.Mutex
	timers   map[TimerID]*timerEntry
	workers  int
	stopping bool

	stopStarted bool
	stopDone    chan struct{}
	stopResult  error
	wg          sync.WaitGroup
	metrics     timerMetrics
	lastFailure *timerAdmissionFailure
}

func newTimerManager(runtime *Runtime, clock runtimeClock) *timerManager {
	if clock == nil {
		clock = realClock{}
	}
	return &timerManager{
		runtime:  runtime,
		clock:    clock,
		timers:   make(map[TimerID]*timerEntry),
		stopDone: make(chan struct{}),
	}
}

func (*timerManager) lifecycleName() string {
	return "timers"
}

func (*timerManager) start(context.Context) error {
	return nil
}

func (manager *timerManager) stop(ctx context.Context) error {
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
	entries := make([]*timerEntry, 0, len(manager.timers))
	for id, entry := range manager.timers {
		entry.state = timerCancelled
		close(entry.cancel)
		entries = append(entries, entry)
		delete(manager.timers, id)
		manager.metrics.cancelled.Add(1)
	}
	manager.mu.Unlock()

	for _, entry := range entries {
		if entry.timer != nil {
			entry.timer.Stop()
		}
	}
	result := manager.waitWorkers(ctx)

	manager.mu.Lock()
	manager.stopResult = result
	close(manager.stopDone)
	manager.mu.Unlock()
	return result
}

func (manager *timerManager) waitWorkers(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		manager.wg.Wait()
		close(done)
	}()
	return waitForSignal(ctx, done)
}

func (manager *timerManager) after(owner *serviceInstance, duration time.Duration, command CommandID, args ...any) (TimerID, error) {
	if owner == nil || owner.runtime != manager.runtime {
		return 0, ErrInvalidService
	}

	manager.runtime.mu.Lock()
	if manager.runtime.state != RuntimeRunning {
		err := manager.runtime.runtimeAdmissionErrorLocked()
		manager.runtime.mu.Unlock()
		return 0, err
	}
	manager.runtime.mu.Unlock()

	ownerState := owner.currentState()
	switch ownerState {
	case serviceStarting, serviceRunning:
	default:
		return 0, serviceAdmissionError(ownerState)
	}

	manager.mu.Lock()
	if manager.stopping {
		manager.mu.Unlock()
		return 0, ErrRuntimeStopping
	}
	id, err := manager.runtime.nextTimerID()
	if err != nil {
		manager.mu.Unlock()
		return 0, err
	}
	entry := &timerEntry{
		id:       id,
		owner:    owner,
		command:  command,
		args:     append([]any(nil), args...),
		deadline: manager.clock.Now().Add(duration),
		cancel:   make(chan struct{}),
	}
	manager.timers[id] = entry
	manager.metrics.created.Add(1)
	manager.mu.Unlock()

	if owner.currentState() == serviceRunning {
		manager.armEntry(entry)
	}
	return id, nil
}

func (manager *timerManager) cancel(owner *serviceInstance, id TimerID) bool {
	if owner == nil || owner.runtime != manager.runtime || id == 0 {
		return false
	}

	manager.mu.Lock()
	entry, exists := manager.timers[id]
	if !exists || entry.owner != owner || entry.state != timerScheduled {
		manager.mu.Unlock()
		return false
	}
	entry.state = timerCancelled
	delete(manager.timers, id)
	close(entry.cancel)
	manager.metrics.cancelled.Add(1)
	timer := entry.timer
	manager.mu.Unlock()

	if timer != nil {
		timer.Stop()
	}
	return true
}

func (manager *timerManager) cancelOwner(owner *serviceInstance) {
	if owner == nil {
		return
	}
	var timers []runtimeTimer
	manager.mu.Lock()
	for id, entry := range manager.timers {
		if entry.owner != owner || entry.state != timerScheduled {
			continue
		}
		entry.state = timerCancelled
		delete(manager.timers, id)
		close(entry.cancel)
		manager.metrics.cancelled.Add(1)
		if entry.timer != nil {
			timers = append(timers, entry.timer)
		}
	}
	manager.mu.Unlock()
	for _, timer := range timers {
		timer.Stop()
	}
}

func (manager *timerManager) armOwner(owner *serviceInstance) {
	var entries []*timerEntry
	manager.mu.Lock()
	if manager.stopping {
		manager.mu.Unlock()
		return
	}
	for _, entry := range manager.timers {
		if entry.owner != owner || entry.state != timerScheduled || entry.armed {
			continue
		}
		manager.armEntryLocked(entry)
		entries = append(entries, entry)
	}
	manager.mu.Unlock()

	for _, entry := range entries {
		go manager.waitForExpiry(entry)
	}
}

func (manager *timerManager) armEntry(entry *timerEntry) {
	// Timers created after publication are armed directly; scanning the whole
	// owner set is reserved for the OnStart-to-Running publication boundary.
	manager.mu.Lock()
	if manager.stopping || entry == nil || entry.state != timerScheduled || entry.armed {
		manager.mu.Unlock()
		return
	}
	manager.armEntryLocked(entry)
	manager.mu.Unlock()
	go manager.waitForExpiry(entry)
}

func (manager *timerManager) armEntryLocked(entry *timerEntry) {
	entry.armed = true
	entry.timer = manager.clock.NewTimer(entry.deadline.Sub(manager.clock.Now()))
	manager.wg.Add(1)
	manager.workers++
}

func (manager *timerManager) waitForExpiry(entry *timerEntry) {
	defer manager.wg.Done()
	defer manager.workerDone()
	select {
	case <-entry.timer.C():
		manager.fire(entry)
	case <-entry.cancel:
	}
}

func (manager *timerManager) workerDone() {
	manager.mu.Lock()
	manager.workers--
	manager.mu.Unlock()
}

func (manager *timerManager) fire(entry *timerEntry) {
	manager.mu.Lock()
	if manager.stopping || entry.state != timerScheduled || !entry.armed {
		manager.mu.Unlock()
		return
	}
	entry.state = timerFired
	delete(manager.timers, entry.id)
	manager.metrics.fired.Add(1)
	manager.mu.Unlock()

	err := manager.runtime.dispatcher.dispatchTimer(entry.owner.id, entry.command, entry.args...)
	if err != nil {
		manager.metrics.admissionRejected.Add(1)
		manager.recordAdmissionFailure(entry, err)
		return
	}
	manager.metrics.admitted.Add(1)
}

func (manager *timerManager) recordAdmissionFailure(entry *timerEntry, err error) {
	// Timer has no caller to which an expiry error can be returned, so retain
	// the latest structured failure for the future Inspection owner path.
	manager.mu.Lock()
	manager.lastFailure = &timerAdmissionFailure{
		TimerID: entry.id,
		Owner:   entry.owner.id,
		Error:   err,
	}
	manager.mu.Unlock()
}

func (manager *timerManager) hasTimers() bool {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.timers) > 0 || manager.workers > 0
}

func (manager *timerManager) count() int {
	if manager == nil {
		return 0
	}
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return len(manager.timers)
}

func (manager *timerManager) metricsSnapshot() timerMetricsSnapshot {
	return manager.metrics.snapshot()
}

func (manager *timerManager) lastAdmissionFailure() *timerAdmissionFailure {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.lastFailure == nil {
		return nil
	}
	failure := *manager.lastFailure
	return &failure
}

func (runtime *Runtime) after(owner *serviceInstance, duration time.Duration, command CommandID, args ...any) (TimerID, error) {
	if runtime.timers == nil {
		return 0, ErrRuntimeClosed
	}
	return runtime.timers.after(owner, duration, command, args...)
}

func (runtime *Runtime) cancelTimer(owner *serviceInstance, id TimerID) bool {
	if runtime.timers == nil {
		return false
	}
	return runtime.timers.cancel(owner, id)
}

func (runtime *Runtime) cancelTimersFor(owner *serviceInstance) {
	if runtime.timers != nil {
		runtime.timers.cancelOwner(owner)
	}
}
