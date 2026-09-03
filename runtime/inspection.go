package runtime

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
)

// ServiceInspection is a read-only point-in-time view of one registered
// Service. It contains values only; it does not expose the Service or runner.
type ServiceInspection struct {
	ID                       ServiceID
	State                    ServiceState
	MailboxDepth             int
	MailboxCapacity          int
	RunnerActive             bool
	RunnerCompleted          bool
	ShutdownDeadlineExceeded bool
}

// RuntimeMetricsSnapshot contains Runtime-owned counters observed by Inspect.
type RuntimeMetricsSnapshot struct {
	AdmissionRejected      uint64
	HandlerErrors          uint64
	Panics                 uint64
	CallTimeouts           uint64
	TimerAdmissionRejected uint64
	TimersCreated          uint64
	TimersCancelled        uint64
	TimersFired            uint64
	TimersAdmitted         uint64
	StopTimeouts           uint64
}

// RuntimeInspection is a read-only point-in-time view of Core Runtime state.
// Services are sorted by ServiceID for deterministic inspection output.
type RuntimeInspection struct {
	State            RuntimeState
	Services         []ServiceInspection
	PendingCallCount int
	TimerCount       int
	Metrics          RuntimeMetricsSnapshot
}

type runtimeMetrics struct {
	admissionRejected atomic.Uint64
	handlerErrors     atomic.Uint64
	panics            atomic.Uint64
	callTimeouts      atomic.Uint64
	stopTimeouts      atomic.Uint64
}

func (metrics *runtimeMetrics) snapshot(timers timerMetricsSnapshot) RuntimeMetricsSnapshot {
	return RuntimeMetricsSnapshot{
		AdmissionRejected:      metrics.admissionRejected.Load(),
		HandlerErrors:          metrics.handlerErrors.Load(),
		Panics:                 metrics.panics.Load(),
		CallTimeouts:           metrics.callTimeouts.Load(),
		TimerAdmissionRejected: timers.AdmissionRejected,
		TimersCreated:          timers.Created,
		TimersCancelled:        timers.Cancelled,
		TimersFired:            timers.Fired,
		TimersAdmitted:         timers.Admitted,
		StopTimeouts:           metrics.stopTimeouts.Load(),
	}
}

// Inspect returns a value snapshot without exposing mutable Runtime-owned
// state. A concurrent operation may be reflected in only part of the result.
func (runtime *Runtime) Inspect() RuntimeInspection {
	if runtime == nil {
		return RuntimeInspection{}
	}

	inspection := RuntimeInspection{State: runtime.State()}
	runtime.servicesMu.RLock()
	instances := make([]*serviceInstance, 0, len(runtime.services))
	for _, instance := range runtime.services {
		instances = append(instances, instance)
	}
	runtime.servicesMu.RUnlock()

	inspection.Services = make([]ServiceInspection, 0, len(instances))
	for _, instance := range instances {
		state := instance.currentState()
		depth, capacity := instance.mailbox.snapshot()
		completed := signalClosed(instance.done)
		active := runtime.scheduler != nil && runtime.scheduler.runnerTracked(instance.id) && !completed
		inspection.Services = append(inspection.Services, ServiceInspection{
			ID:                       instance.id,
			State:                    inspectServiceState(state),
			MailboxDepth:             depth,
			MailboxCapacity:          capacity,
			RunnerActive:             active,
			RunnerCompleted:          completed,
			ShutdownDeadlineExceeded: instance.deadlineExceeded(),
		})
	}
	sort.Slice(inspection.Services, func(left, right int) bool {
		return inspection.Services[left].ID < inspection.Services[right].ID
	})

	inspection.PendingCallCount = runtime.pendingCalls.count()
	inspection.TimerCount = runtime.timers.count()
	var timerSnapshot timerMetricsSnapshot
	if runtime.timers != nil {
		timerSnapshot = runtime.timers.metricsSnapshot()
	}
	inspection.Metrics = runtime.metrics.snapshot(timerSnapshot)
	return inspection
}

func inspectServiceState(state serviceState) ServiceState {
	switch state {
	case serviceStarting:
		return ServiceStateStarting
	case serviceRunning:
		return ServiceStateRunning
	case serviceStopping:
		return ServiceStateStopping
	case serviceStopped:
		return ServiceStateStopped
	case serviceFailed:
		return ServiceStateFailed
	default:
		return ServiceStateFailed
	}
}

func signalClosed(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	default:
		return false
	}
}

func (runtime *Runtime) recordAdmissionRejected() {
	if runtime != nil {
		runtime.metrics.admissionRejected.Add(1)
	}
}

func (runtime *Runtime) recordHandlerMetric(err error) {
	if runtime == nil || err == nil {
		return
	}
	runtime.metrics.handlerErrors.Add(1)
	if isStatusCode(err, ErrorCodeServicePanicked) {
		runtime.metrics.panics.Add(1)
	}
}

func (runtime *Runtime) recordCallTimeout() {
	if runtime != nil {
		runtime.metrics.callTimeouts.Add(1)
	}
}

func (runtime *Runtime) recordStopResult(err error) {
	if runtime != nil && errors.Is(err, context.DeadlineExceeded) {
		runtime.metrics.stopTimeouts.Add(1)
	}
}
