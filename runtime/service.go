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
// Handle 是普通 Core Command 的业务入口；客户端消息由可选的 ClientMessageHandler 接收。
type Service interface {
	OnStart(context.Context, ServiceContext) error
	Handle(CommandContext, CommandID, ...any) (any, error)
	OnStop(context.Context) error
}

// ClientMessageHandler 是 Service 可选的客户端消息处理能力。
//
// 实现该接口后，Runtime 才会接纳 DeliverClientMessage 投递到此 Service 的不透明字节消息。
// 客户端消息与 Core Command 共用目标 Service 的 Mailbox 和串行 runner；Runtime 不解析消息格式。
type ClientMessageHandler interface {
	HandleClientMessage(CommandContext, []byte) error
}

// ServiceContext 提供当前 Service 可使用的 Core 能力。
//
// 提供本地身份、创建 Service、自身退出、Send、有等待上限的 Call 和一次性 Timer 操作。
// Call 等待目标 Handler 的返回值，期间不会重入调用方 Service 的 runner。
type ServiceContext interface {
	Self() ServiceID
	// CreateService 将实例交给所属 Runtime；创建者退出不停止新 Service。
	CreateService(context.Context, Service) (ServiceID, error)
	// Exit 请求自身在当前 OnStart/Handler 返回后退出；不会中断 Go 控制流，调用后应正常返回。
	Exit()
	Send(ServiceID, CommandID, ...any) error
	Call(context.Context, ServiceID, CommandID, ...any) (any, error)
	After(time.Duration, CommandID, ...any) (TimerID, error)
	CancelTimer(TimerID) bool
}

// CommandContext 描述当前处理消息的 Service 及消息来源。
//
// 此上下文仅在 Handle 或 HandleClientMessage 返回前有效，不提供 Reply 方法；
// Runtime 使用普通 Handle 的返回值完成 Core Call。
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
	exitRequested        bool
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
	// OnStart 可以请求 Exit，已进入收尾的实例不能重新开放接纳。
	if instance.state == serviceStarting {
		instance.state = serviceRunning
	}
	instance.mu.Unlock()
}

// stopAfterExit 只在当前执行返回之后调用，避免停止清理抢先完成当前 Call。
func (instance *serviceInstance) stopAfterExit() {
	instance.mu.Lock()
	exit := instance.exitRequested && !instance.stopRequested
	instance.mu.Unlock()
	if exit {
		ctx, cancel := instance.runtime.failureContext()
		instance.requestStopWithCancel(ctx, cancel)
	}
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
	defer func() {
		instance.runtime.scheduler.releaseRunner(instance.id)
		close(instance.done)
	}()
	for {
		message, ok := instance.mailbox.dequeue()
		if !ok {
			break
		}
		if _, err := instance.runtime.executeMessage(instance, message); err != nil {
			if errors.Is(err, ErrServicePanicked) {
				failureContext, cancel := instance.runtime.failureContext()
				instance.requestStopWithCancel(failureContext, cancel)
			}
		}
		instance.stopAfterExit()
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

	// 与客户端能力检查共用注册表锁，避免退役时清空 Service 与入站读取竞争。
	instance.runtime.servicesMu.Lock()
	instance.mu.Lock()
	if contextDeadlineExceeded(stopContext) {
		instance.stopDeadlineExceeded = true
	}
	instance.stopResult = onStopErr
	instance.state = serviceStopped
	// 保留 ID、状态、Mailbox 容量和收尾结果用于关闭语义及 Inspection，
	// 不再让历史记录持有业务实例、回调上下文或取消函数。
	instance.service = nil
	instance.context = nil
	instance.stopContext = nil
	instance.stopCancel = nil
	instance.mu.Unlock()
	instance.runtime.servicesMu.Unlock()
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

var _ ServiceContext = (*serviceContext)(nil)

func (context *serviceContext) Self() ServiceID {
	return context.instance.id
}

func (context *serviceContext) CreateService(ctx context.Context, service Service) (ServiceID, error) {
	switch context.instance.currentState() {
	case serviceStarting, serviceRunning:
		return context.runtime.CreateService(ctx, service)
	case serviceStopping:
		return 0, ErrServiceStopping
	default:
		return 0, ErrServiceClosed
	}
}

func (context *serviceContext) Exit() {
	instance := context.instance
	instance.mu.Lock()
	if instance.state == serviceStarting || instance.state == serviceRunning {
		instance.exitRequested = true
	}
	instance.mu.Unlock()
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

var _ CommandContext = (*commandContext)(nil)

func (context *commandContext) Source() ServiceID {
	return context.source
}

func (runtime *Runtime) executeMessage(instance *serviceInstance, message Message) (result any, err error) {
	defer func() {
		var stack string
		if recovered := recover(); recovered != nil {
			stack = handlerPanicStack()
			err = wrapStatusError(ErrServicePanicked, "service Handler panicked: %v", recovered)
		}
		cause := err
		var classificationStack string
		err, classificationStack = normalizeHandlerError(err)
		if classificationStack != "" {
			stack = classificationStack
			cause = wrapStatusError(ErrServicePanicked, "service error classification panicked")
		}
		runtime.completeMessage(message, result, err)
		if err != nil {
			runtime.recordHandlerError(instance, message, cause, err, stack)
		}
	}()

	messageContext := &commandContext{
		serviceContext: instance.context,
		source:         message.Source,
	}
	switch message.kind {
	case messageKindCommand:
		return instance.service.Handle(messageContext, message.Command, message.Args...)
	case messageKindClient:
		handler, ok := instance.service.(ClientMessageHandler)
		if !ok {
			return nil, ErrClientMessageUnsupported
		}
		return nil, handler.HandleClientMessage(messageContext, message.clientFrame)
	default:
		return nil, ErrClientMessageUnsupported
	}
}

func (runtime *Runtime) failureContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), runtime.config.ShutdownTimeout)
}
