package runtime

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Service owns mutable state behind one Runtime-local ServiceID.
//
// OnStart and OnStop are lifecycle hooks owned by Runtime. Handle is the only
// business entry Runtime invokes for an admitted Message.
type Service interface {
	OnStart(context.Context, ServiceContext) error
	Handle(CommandContext, CommandID, ...any) (any, error)
	OnStop(context.Context) error
}

// ServiceContext exposes the Core capabilities available to a Service.
//
// Core 3 exposes local identity, Send, bounded Call and one-shot Timer
// operations. Call waits for the target Handler's return value without
// re-entering the caller Service runner.
type ServiceContext interface {
	Self() ServiceID
	Send(ServiceID, CommandID, ...any) error
	Call(context.Context, ServiceID, CommandID, ...any) (any, error)
	After(time.Duration, CommandID, ...any) (TimerID, error)
	CancelTimer(TimerID) bool
}

// CommandContext describes the Service and source of the Message being handled.
//
// The context is valid only until Handle returns. It deliberately has no
// Reply method; Runtime completes Call results from Handle's return values.
type CommandContext interface {
	ServiceContext
	Source() ServiceID
}

// ServiceState is the observable lifecycle state of a registered Service.
type ServiceState uint8

const (
	// ServiceStateStarting means OnStart has not finished publishing the Service.
	ServiceStateStarting ServiceState = iota
	// ServiceStateRunning means the Service accepts ordinary messages.
	ServiceStateRunning
	// ServiceStateStopping means the Service rejects new messages and is draining its runner.
	ServiceStateStopping
	// ServiceStateStopped means the Service runner and OnStop have completed.
	ServiceStateStopped
	// ServiceStateFailed is reserved for an externally classified failed
	// Service; the first Runtime does not transition into it automatically.
	ServiceStateFailed
)

// String returns the stable display name of a ServiceState.
func (state ServiceState) String() string {
	switch state {
	case ServiceStateStarting:
		return "Starting"
	case ServiceStateRunning:
		return "Running"
	case ServiceStateStopping:
		return "Stopping"
	case ServiceStateStopped:
		return "Stopped"
	case ServiceStateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

type serviceState uint8

const (
	serviceStarting serviceState = iota
	serviceRunning
	serviceStopping
	serviceStopped
	serviceFailed
)

type serviceInstance struct {
	runtime *Runtime
	id      ServiceID
	service Service

	mailbox *mailbox
	context *serviceContext
	done    chan struct{}

	mu                   sync.Mutex
	state                serviceState
	stopRequested        bool
	stopContext          context.Context
	stopCancel           context.CancelFunc
	stopResult           error
	stopDeadlineExceeded bool
	onStopCompleted      bool
}

func newServiceInstance(runtime *Runtime, id ServiceID, service Service) *serviceInstance {
	instance := &serviceInstance{
		runtime: runtime,
		id:      id,
		service: service,
		mailbox: newMailbox(runtime.config.MailboxCapacity),
		done:    make(chan struct{}),
		state:   serviceStarting,
	}
	instance.context = &serviceContext{runtime: runtime, instance: instance}
	return instance
}

func (instance *serviceInstance) currentState() serviceState {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	return instance.state
}

func (instance *serviceInstance) setRunning() {
	instance.mu.Lock()
	instance.state = serviceRunning
	instance.mu.Unlock()
}

func (instance *serviceInstance) requestStop(ctx context.Context) {
	instance.requestStopWithCancel(ctx, nil)
}

func (instance *serviceInstance) requestStopWithCancel(ctx context.Context, cancel context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	var cancelAfterUnlock context.CancelFunc
	instance.mu.Lock()
	if !instance.stopRequested {
		instance.stopRequested = true
		instance.stopContext = ctx
		instance.stopCancel = cancel
	} else if cancel != nil {
		cancelAfterUnlock = cancel
	}
	if instance.state == serviceStarting || instance.state == serviceRunning {
		instance.state = serviceStopping
	}
	instance.mailbox.stop()
	instance.mu.Unlock()
	instance.runtime.cancelTimersFor(instance)
	instance.runtime.failPendingCallsFor(instance)
	if cancelAfterUnlock != nil {
		cancelAfterUnlock()
	}
}

func (instance *serviceInstance) run() {
	defer close(instance.done)
	for {
		message, ok := instance.mailbox.dequeue()
		if !ok {
			break
		}
		if _, err := instance.runtime.executeMessage(instance, message); err != nil {
			instance.runtime.recordHandlerError(instance, message, err)
			if err == ErrServicePanicked || isStatusCode(err, ErrorCodeServicePanicked) {
				failureContext, cancel := instance.runtime.failureContext()
				instance.requestStopWithCancel(failureContext, cancel)
			}
		}
	}

	instance.finishStop()
}

func (instance *serviceInstance) finishStop() {
	instance.mu.Lock()
	if instance.onStopCompleted {
		instance.mu.Unlock()
		return
	}
	instance.onStopCompleted = true
	stopContext := instance.stopContext
	stopCancel := instance.stopCancel
	if contextDeadlineExceeded(stopContext) {
		instance.stopDeadlineExceeded = true
	}
	instance.mu.Unlock()

	if stopContext == nil {
		stopContext = context.Background()
	}
	onStopErr := invokeServiceHook(func() error {
		return instance.service.OnStop(stopContext)
	})
	if stopCancel != nil {
		stopCancel()
	}

	instance.mu.Lock()
	if contextDeadlineExceeded(stopContext) {
		instance.stopDeadlineExceeded = true
	}
	instance.stopResult = onStopErr
	instance.state = serviceStopped
	instance.mu.Unlock()
}

func (instance *serviceInstance) deadlineExceeded() bool {
	instance.mu.Lock()
	if contextDeadlineExceeded(instance.stopContext) {
		instance.stopDeadlineExceeded = true
	}
	exceeded := instance.stopDeadlineExceeded
	instance.mu.Unlock()
	return exceeded
}

func contextDeadlineExceeded(ctx context.Context) bool {
	return ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func (instance *serviceInstance) result() error {
	instance.mu.Lock()
	defer instance.mu.Unlock()
	return instance.stopResult
}

func invokeServiceHook(fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = wrapStatusError(ErrServicePanicked, "service lifecycle panicked: %v", recovered)
		}
	}()
	return fn()
}

type serviceContext struct {
	runtime  *Runtime
	instance *serviceInstance
}

func (context *serviceContext) Self() ServiceID {
	return context.instance.id
}

func (context *serviceContext) Send(target ServiceID, command CommandID, args ...any) error {
	return context.runtime.sendFrom(context.instance, target, command, args...)
}

func (context *serviceContext) Call(callContext context.Context, target ServiceID, command CommandID, args ...any) (any, error) {
	return context.runtime.call(context.instance, callContext, target, command, args...)
}

func (context *serviceContext) After(duration time.Duration, command CommandID, args ...any) (TimerID, error) {
	return context.runtime.after(context.instance, duration, command, args...)
}

func (context *serviceContext) CancelTimer(id TimerID) bool {
	return context.runtime.cancelTimer(context.instance, id)
}

type commandContext struct {
	*serviceContext
	source ServiceID
}

func (context *commandContext) Source() ServiceID {
	return context.source
}

func (runtime *Runtime) executeMessage(instance *serviceInstance, message Message) (result any, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = wrapStatusError(ErrServicePanicked, "service Handler panicked: %v", recovered)
		}
		runtime.completeMessage(message, result, err)
	}()

	result, err = instance.service.Handle(&commandContext{
		serviceContext: instance.context,
		source:         message.Source,
	}, message.Command, message.Args...)
	return result, err
}

func (runtime *Runtime) recordHandlerError(_ *serviceInstance, _ Message, err error) {
	// Send has no completion channel; Inspection exposes this diagnostic
	// without changing the fire-and-forget admission contract.
	runtime.recordHandlerMetric(err)
}

func (runtime *Runtime) failureContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), runtime.config.ShutdownTimeout)
}

func isStatusCode(err error, code ErrorCode) bool {
	var status *StatusError
	return errors.As(err, &status) && status.Code == code
}
