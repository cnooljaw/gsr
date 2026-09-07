package runtime

import (
	"context"
	"errors"
	"reflect"
	"sync"
)

// serviceLifecycle 在 Runtime 生命周期计划中负责 Service 的接纳管理。
type serviceLifecycle struct {
	runtime *Runtime
}

// localDispatcher 是由 Runtime 管理的本地路由边界。
// 它查询当前 Service 注册表，但不检查 CommandID 支持情况；
// 每条已接纳的 Message 都交给目标 Handler。
type localDispatcher struct {
	runtime *Runtime
}

func (component *serviceLifecycle) lifecycleName() string {
	return "services"
}

func (*serviceLifecycle) start(context.Context) error {
	return nil
}

func (component *serviceLifecycle) stop(ctx context.Context) error {
	return component.runtime.requestStopServices(ctx)
}

// scheduler 管理 runner goroutine 及其等待退出的生命周期。
// 不设全局就绪队列，每个 Service 持有自己的 FIFO Mailbox。
type scheduler struct {
	runtime *Runtime

	mu       sync.Mutex
	stopping bool
	runners  map[ServiceID]*serviceInstance
	watching map[ServiceID]struct{}
}

func newScheduler(runtime *Runtime) *scheduler {
	return &scheduler{
		runtime:  runtime,
		runners:  make(map[ServiceID]*serviceInstance),
		watching: make(map[ServiceID]struct{}),
	}
}

func (*scheduler) lifecycleName() string {
	return "scheduler"
}

func (*scheduler) start(context.Context) error {
	return nil
}

func (scheduler *scheduler) spawn(instance *serviceInstance) error {
	scheduler.mu.Lock()
	if scheduler.stopping {
		scheduler.mu.Unlock()
		return ErrRuntimeStopping
	}
	scheduler.runners[instance.id] = instance
	scheduler.mu.Unlock()
	go instance.run()
	return nil
}

func (scheduler *scheduler) stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	scheduler.mu.Lock()
	scheduler.stopping = true
	instances := make([]*serviceInstance, 0, len(scheduler.runners))
	for _, instance := range scheduler.runners {
		instances = append(instances, instance)
	}
	scheduler.mu.Unlock()

	var result error
	for _, instance := range instances {
		if err := waitForSignal(ctx, instance.done); err != nil {
			result = errors.Join(result, err)
			break
		}
		result = errors.Join(result, instance.result())
	}
	scheduler.removeCompleted(instances)
	for _, instance := range instances {
		scheduler.watchCompletion(instance)
	}
	return result
}

func (scheduler *scheduler) removeCompleted(instances []*serviceInstance) {
	scheduler.mu.Lock()
	for _, instance := range instances {
		select {
		case <-instance.done:
			delete(scheduler.runners, instance.id)
			delete(scheduler.watching, instance.id)
		default:
		}
	}
	scheduler.mu.Unlock()
	scheduler.runtime.removeStoppedServices(instances)
}

func (scheduler *scheduler) watchCompletion(instance *serviceInstance) {
	select {
	case <-instance.done:
		scheduler.removeCompleted([]*serviceInstance{instance})
		return
	default:
	}

	scheduler.mu.Lock()
	if _, exists := scheduler.watching[instance.id]; exists {
		scheduler.mu.Unlock()
		return
	}
	scheduler.watching[instance.id] = struct{}{}
	scheduler.mu.Unlock()
	go func() {
		<-instance.done
		scheduler.removeCompleted([]*serviceInstance{instance})
	}()
}

func (scheduler *scheduler) hasRunners() bool {
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	return len(scheduler.runners) > 0
}

func (scheduler *scheduler) runnerTracked(id ServiceID) bool {
	if scheduler == nil {
		return false
	}
	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	_, exists := scheduler.runners[id]
	return exists
}

// CreateService 在 Runtime.Start 后接纳一个已构造的 Service 实例。
// 即使 OnStart 失败，已分配的 ServiceID 也会被消耗且永不复用。
func (runtime *Runtime) CreateService(ctx context.Context, service Service) (ServiceID, error) {
	if serviceValueIsNil(service) {
		return 0, ErrInvalidService
	}
	if ctx == nil {
		ctx = context.Background()
	}

	runtime.mu.Lock()
	if runtime.state != RuntimeRunning {
		err := runtime.runtimeAdmissionErrorLocked()
		runtime.mu.Unlock()
		return 0, err
	}
	runtime.creating.Add(1)
	runtime.creatingCount++
	runtime.mu.Unlock()
	defer func() {
		runtime.mu.Lock()
		runtime.creatingCount--
		runtime.mu.Unlock()
		runtime.creating.Done()
	}()

	id, err := runtime.nextServiceID()
	if err != nil {
		return 0, err
	}
	instance := newServiceInstance(runtime, id, service)

	if startErr := invokeServiceHook(func() error {
		return service.OnStart(ctx, instance.context)
	}); startErr != nil {
		return 0, runtime.cleanupUnpublishedService(instance, ctx, startErr)
	}

	runtime.mu.Lock()
	if runtime.state != RuntimeRunning {
		err := runtime.runtimeAdmissionErrorLocked()
		runtime.mu.Unlock()
		return 0, runtime.cleanupUnpublishedService(instance, ctx, err)
	}
	runtime.servicesMu.Lock()
	instance.setRunning()
	runtime.services[id] = instance
	runtime.timers.armOwner(instance)
	spawnErr := runtime.scheduler.spawn(instance)
	if spawnErr != nil {
		delete(runtime.services, id)
	}
	runtime.servicesMu.Unlock()
	runtime.mu.Unlock()
	if spawnErr != nil {
		return 0, runtime.cleanupUnpublishedService(instance, ctx, spawnErr)
	}
	return id, nil
}

// Send 接纳 Runtime 基础设施发出的消息，Source 为零。
// Service 代码通常使用 ServiceContext.Send，由 Runtime 记录发送方 ServiceID。
func (runtime *Runtime) Send(target ServiceID, command CommandID, args ...any) error {
	if runtime.dispatcher == nil {
		return ErrRuntimeClosed
	}
	return runtime.dispatcher.dispatchRuntimeOwned(target, command, args...)
}

func (runtime *Runtime) sendFrom(source *serviceInstance, target ServiceID, command CommandID, args ...any) error {
	if runtime.dispatcher == nil {
		return ErrRuntimeClosed
	}
	return runtime.dispatcher.dispatchFromService(source, target, command, args...)
}

func (dispatcher *localDispatcher) dispatchFromService(source *serviceInstance, target ServiceID, command CommandID, args ...any) error {
	return dispatcher.dispatchFromServiceWithSession(source, target, command, args, 0)
}

func (dispatcher *localDispatcher) dispatchCallFromService(source *serviceInstance, target ServiceID, command CommandID, args []any, session SessionID) error {
	return dispatcher.dispatchFromServiceWithSession(source, target, command, args, session)
}

func (dispatcher *localDispatcher) dispatchFromServiceWithSession(source *serviceInstance, target ServiceID, command CommandID, args []any, session SessionID) error {
	if source == nil || source.runtime != dispatcher.runtime {
		return ErrInvalidService
	}
	if err := serviceSourceAdmissionError(source, target); err != nil {
		return err
	}
	return dispatcher.enqueue(Message{
		Source:  source.id,
		Target:  target,
		Session: session,
		Command: command,
		Args:    append([]any(nil), args...),
	})
}

func (dispatcher *localDispatcher) dispatchRuntimeOwned(target ServiceID, command CommandID, args ...any) error {
	return dispatcher.enqueue(Message{
		Source:  ServiceID(0),
		Target:  target,
		Session: 0,
		Command: command,
		Args:    append([]any(nil), args...),
	})
}

func (dispatcher *localDispatcher) dispatchTimer(target ServiceID, command CommandID, args ...any) error {
	return dispatcher.enqueue(Message{
		Source:  ServiceID(0),
		Target:  target,
		Session: 0,
		Command: command,
		Args:    append([]any(nil), args...),
	})
}

func (dispatcher *localDispatcher) enqueue(message Message) (err error) {
	runtime := dispatcher.runtime
	defer func() {
		if err != nil {
			runtime.recordAdmissionRejected()
		}
	}()
	runtime.mu.Lock()
	if runtime.state != RuntimeRunning {
		err := runtime.runtimeAdmissionErrorLocked()
		runtime.mu.Unlock()
		return err
	}
	runtime.mu.Unlock()

	runtime.servicesMu.RLock()
	targetInstance := runtime.services[message.Target]
	if targetInstance == nil {
		runtime.servicesMu.RUnlock()
		return ErrServiceNotFound
	}
	targetState := targetInstance.currentState()
	runtime.servicesMu.RUnlock()
	if targetErr := serviceAdmissionError(targetState); targetErr != nil {
		return targetErr
	}

	return targetInstance.mailbox.enqueue(message)
}

func serviceSourceAdmissionError(source *serviceInstance, target ServiceID) error {
	switch state := source.currentState(); state {
	case serviceStarting:
		if target == source.id {
			return ErrServiceStarting
		}
	case serviceStopping:
		return ErrServiceStopping
	case serviceStopped, serviceFailed:
		return ErrServiceClosed
	}
	return nil
}

func (runtime *Runtime) runtimeAdmissionErrorLocked() error {
	switch runtime.state {
	case RuntimeCreated, RuntimeStarting:
		return ErrRuntimeNotRunning
	case RuntimeStopping:
		return ErrRuntimeStopping
	case RuntimeStopped:
		return ErrRuntimeStopped
	case RuntimeClosed:
		return ErrRuntimeClosed
	default:
		return ErrRuntimeNotRunning
	}
}

func serviceAdmissionError(state serviceState) error {
	switch state {
	case serviceStarting:
		return ErrServiceStarting
	case serviceStopping:
		return ErrServiceStopping
	case serviceStopped, serviceFailed:
		return ErrServiceClosed
	default:
		return nil
	}
}

func serviceValueIsNil(service Service) bool {
	if service == nil {
		return true
	}
	value := reflect.ValueOf(service)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func (runtime *Runtime) cleanupUnpublishedService(instance *serviceInstance, ctx context.Context, cause error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cleanupContext, cancel := context.WithTimeout(ctx, runtime.config.ShutdownTimeout)
	defer cancel()
	instance.requestStop(cleanupContext)
	instance.finishStop()
	close(instance.done)
	stopErr := instance.result()
	if stopErr == nil {
		return cause
	}
	return errors.Join(cause, stopErr)
}

func (runtime *Runtime) requestStopServices(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	runtime.servicesMu.RLock()
	instances := make([]*serviceInstance, 0, len(runtime.services))
	for _, instance := range runtime.services {
		instances = append(instances, instance)
	}
	runtime.servicesMu.RUnlock()
	for _, instance := range instances {
		instance.requestStop(ctx)
	}

	creatingDone := make(chan struct{})
	go func() {
		runtime.creating.Wait()
		close(creatingDone)
	}()
	if err := waitForSignal(ctx, creatingDone); err != nil {
		return err
	}
	return nil
}

func (runtime *Runtime) removeStoppedServices(instances []*serviceInstance) {
	runtime.servicesMu.Lock()
	defer runtime.servicesMu.Unlock()
	for _, instance := range instances {
		if instance.currentState() == serviceStopped {
			delete(runtime.services, instance.id)
		}
	}
}

func (runtime *Runtime) resourcesReleased() bool {
	runtime.mu.Lock()
	creating := runtime.creatingCount
	runtime.mu.Unlock()
	if creating != 0 {
		return false
	}
	if runtime.scheduler != nil && runtime.scheduler.hasRunners() {
		return false
	}
	if runtime.timers != nil && runtime.timers.hasTimers() {
		return false
	}
	if runtime.pendingCalls != nil && runtime.pendingCalls.hasPending() {
		return false
	}
	runtime.servicesMu.RLock()
	defer runtime.servicesMu.RUnlock()
	return len(runtime.services) == 0
}
