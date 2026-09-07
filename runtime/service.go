package runtime

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Service 通过 Runtime 本地的 ServiceID 对外提供入口，并独占其可变状态。
//
// OnStart 和 OnStop 是由 Runtime 管理的生命周期钩子。
// 对已接纳的 Message，Handle 是 Runtime 调用的唯一业务入口。
type Service interface {
	OnStart(context.Context, ServiceContext) error
	Handle(CommandContext, CommandID, ...any) (any, error)
	OnStop(context.Context) error
}

// ServiceContext 提供当前 Service 可使用的 Core 能力。
//
// Core 3 提供本地身份、Send、有等待上限的 Call 和一次性 Timer 操作。
// Call 等待目标 Handler 的返回值，期间不会重入调用方 Service 的 runner。
type ServiceContext interface {
	Self() ServiceID
	Send(ServiceID, CommandID, ...any) error
	Call(context.Context, ServiceID, CommandID, ...any) (any, error)
	After(time.Duration, CommandID, ...any) (TimerID, error)
	CancelTimer(TimerID) bool
}

// CommandContext 描述当前处理消息的 Service 及消息来源。
//
// 此上下文仅在 Handle 返回前有效，不提供 Reply 方法；
// Runtime 使用 Handle 的返回值完成 Call。
type CommandContext interface {
	ServiceContext
	Source() ServiceID
}

// ServiceState 是已注册 Service 对外可观测的生命周期状态。
type ServiceState uint8

const (
	// ServiceStateStarting 表示 OnStart 尚未完成 Service 的发布。
	ServiceStateStarting ServiceState = iota
	// ServiceStateRunning 表示 Service 接收普通消息。
	ServiceStateRunning
	// ServiceStateStopping 表示 Service 拒绝新消息，正在收尾并等待 runner 退出。
	ServiceStateStopping
	// ServiceStateStopped 表示 Service runner 和 OnStop 均已完成。
	ServiceStateStopped
	// ServiceStateFailed 为外部判定失败的 Service 预留；
	// 第一版 Runtime 不会自动转入此状态。
	ServiceStateFailed
)

// String 返回 ServiceState 的稳定显示名称。
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
	// Send 没有完成结果通道；Inspection 展示此诊断，
	// 但不改变单向投递的接纳契约。
	runtime.recordHandlerMetric(err)
}

func (runtime *Runtime) failureContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), runtime.config.ShutdownTimeout)
}

func isStatusCode(err error, code ErrorCode) bool {
	var status *StatusError
	return errors.As(err, &status) && status.Code == code
}
