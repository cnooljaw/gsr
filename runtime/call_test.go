package runtime

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

const callTestCommand CommandID = 0x04100306

// 验证 Call 与 Send 共用本地消息路径，并返回 Handler 结果和真实发起方 Source。
func TestCallReturnsResultAndPreservesSource(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := &callTargetService{response: "pong", events: make(chan callTargetEvent, 1)}
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	caller := &callService{target: targetID, outcomes: make(chan callOutcome, 1)}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		t.Fatalf("CreateService(caller) error = %v", err)
	}
	if err := runtime.Send(callerID, callTestCommand); err != nil {
		t.Fatalf("Send(call) error = %v", err)
	}

	outcome := waitCallOutcome(t, caller.outcomes)
	if outcome.err != nil || outcome.value != "pong" {
		t.Fatalf("Call outcome = %#v, want pong and nil error", outcome)
	}
	event := waitCallTargetEvent(t, target.events)
	if event.source != callerID {
		t.Fatalf("Call Source = %d, want caller ServiceID %d", event.source, callerID)
	}
}

// 验证 nil context 和对自身 Service 的 Call 都在建立 PendingCall 前被拒绝。
func TestCallRejectsInvalidContextAndSelf(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	service := &selfCallService{outcomes: make(chan callOutcome, 1)}
	serviceID, err := runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if _, err := service.context.Call(nil, serviceID, callTestCommand); !errors.Is(err, ErrInvalidCallContext) {
		t.Fatalf("nil context Call() error = %v, want ErrInvalidCallContext", err)
	}
	if err := runtime.Send(serviceID, callTestCommand); err != nil {
		t.Fatalf("Send(self call) error = %v", err)
	}
	outcome := waitCallOutcome(t, service.outcomes)
	if !errors.Is(outcome.err, ErrCallCycle) {
		t.Fatalf("self Call() error = %v, want ErrCallCycle", outcome.err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("self Call left a PendingCall")
	}
}

// 验证 OnStart 阶段的自身 Call 不会进入 Mailbox 或创建悬挂的 PendingCall。
func TestOnStartSelfCallIsRejected(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	service := &onStartSelfCallService{}
	if _, err := runtime.CreateService(context.Background(), service); err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if !errors.Is(service.callErr, ErrCallCycle) {
		t.Fatalf("OnStart self Call() error = %v, want ErrCallCycle", service.callErr)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("OnStart self Call left a PendingCall")
	}
}

// 验证 Runtime MaxCallWait 使用 Core 时钟生效，并在超时后清理 PendingCall。
func TestCallHonorsRuntimeMaxWait(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{MaxCallWait: time.Second})
	target := newBlockingCallTargetService()
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	caller := &callService{target: targetID, outcomes: make(chan callOutcome, 1)}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		t.Fatalf("CreateService(caller) error = %v", err)
	}
	if err := runtime.Send(callerID, callTestCommand); err != nil {
		t.Fatalf("Send(call) error = %v", err)
	}
	waitSignal(t, target.entered)
	clock.Advance(time.Second)
	outcome := waitCallOutcome(t, caller.outcomes)
	if !errors.Is(outcome.err, ErrCallTimeout) {
		t.Fatalf("Call timeout error = %v, want ErrCallTimeout", outcome.err)
	}
	if !errors.Is(outcome.err, context.DeadlineExceeded) {
		t.Fatalf("Call timeout error = %v, want context.DeadlineExceeded compatibility", outcome.err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("timed out Call left a PendingCall")
	}
	target.releaseNow()
}

// 验证调用方 context 可以提前取消 Call，且迟到的目标完成结果不会再次完成它。
func TestCallHonorsCallerCancellation(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := newBlockingCallTargetService()
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	callContext, cancel := context.WithCancel(context.Background())
	caller := &callService{target: targetID, callContext: callContext, outcomes: make(chan callOutcome, 1)}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		t.Fatalf("CreateService(caller) error = %v", err)
	}
	if err := runtime.Send(callerID, callTestCommand); err != nil {
		t.Fatalf("Send(call) error = %v", err)
	}
	waitSignal(t, target.entered)
	cancel()
	outcome := waitCallOutcome(t, caller.outcomes)
	if !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("cancelled Call error = %v, want context.Canceled", outcome.err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("cancelled Call left a PendingCall")
	}
	target.releaseNow()
}

// 验证 Call 接纳遇到有界 Mailbox 满载时立即失败并删除刚分配的 Session。
func TestCallAdmissionFailureCleansPending(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{MailboxCapacity: 1})
	target := newBlockingCallTargetService()
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	if err := runtime.Send(targetID, callTestCommand, "blocking"); err != nil {
		t.Fatalf("Send(blocking) error = %v", err)
	}
	waitSignal(t, target.entered)
	if err := runtime.Send(targetID, callTestCommand, "queued"); err != nil {
		t.Fatalf("Send(queued) error = %v", err)
	}
	caller := &callService{target: targetID, outcomes: make(chan callOutcome, 1)}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		t.Fatalf("CreateService(caller) error = %v", err)
	}
	if err := runtime.Send(callerID, callTestCommand); err != nil {
		t.Fatalf("Send(call) error = %v", err)
	}
	outcome := waitCallOutcome(t, caller.outcomes)
	if !errors.Is(outcome.err, ErrMailboxFull) {
		t.Fatalf("Call admission error = %v, want ErrMailboxFull", outcome.err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("admission-failed Call left a PendingCall")
	}
	target.releaseNow()
}

// 验证 Handler 的普通 error 被转换为安全稳定的 ErrHandlerFailed，不泄漏内部文本。
func TestCallNormalizesUnknownHandlerError(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := &callTargetService{handlerErr: errors.New("secret internal detail")}
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	caller := &callService{target: targetID, outcomes: make(chan callOutcome, 1)}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		t.Fatalf("CreateService(caller) error = %v", err)
	}
	if err := runtime.Send(callerID, callTestCommand); err != nil {
		t.Fatalf("Send(call) error = %v", err)
	}
	outcome := waitCallOutcome(t, caller.outcomes)
	if !errors.Is(outcome.err, ErrHandlerFailed) {
		t.Fatalf("Handler error = %v, want ErrHandlerFailed", outcome.err)
	}
	if strings.Contains(outcome.err.Error(), "secret internal detail") {
		t.Fatalf("Handler error leaked private text: %v", outcome.err)
	}
}

// 验证已分类的 Handler 错误保持稳定错误码，而 panic 完成结果隐藏 panic 文本并停止目标 Service。
func TestCallPreservesStatusAndPanicErrors(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	unknown := &callTargetService{handlerErr: ErrUnknownCommand}
	unknownID, err := runtime.CreateService(context.Background(), unknown)
	if err != nil {
		t.Fatalf("CreateService(unknown) error = %v", err)
	}
	unknownCaller := &callService{target: unknownID, outcomes: make(chan callOutcome, 1)}
	unknownCallerID, err := runtime.CreateService(context.Background(), unknownCaller)
	if err != nil {
		t.Fatalf("CreateService(unknown caller) error = %v", err)
	}
	if err := runtime.Send(unknownCallerID, callTestCommand); err != nil {
		t.Fatalf("Send(unknown call) error = %v", err)
	}
	unknownOutcome := waitCallOutcome(t, unknownCaller.outcomes)
	if !errors.Is(unknownOutcome.err, ErrUnknownCommand) {
		t.Fatalf("unknown Handler error = %v, want ErrUnknownCommand", unknownOutcome.err)
	}

	panicTarget := &callTargetService{panicValue: "private panic"}
	panicTargetID, err := runtime.CreateService(context.Background(), panicTarget)
	if err != nil {
		t.Fatalf("CreateService(panic target) error = %v", err)
	}
	panicCaller := &callService{target: panicTargetID, outcomes: make(chan callOutcome, 1)}
	panicCallerID, err := runtime.CreateService(context.Background(), panicCaller)
	if err != nil {
		t.Fatalf("CreateService(panic caller) error = %v", err)
	}
	if err := runtime.Send(panicCallerID, callTestCommand); err != nil {
		t.Fatalf("Send(panic call) error = %v", err)
	}
	panicOutcome := waitCallOutcome(t, panicCaller.outcomes)
	if !errors.Is(panicOutcome.err, ErrServicePanicked) {
		t.Fatalf("panic Handler error = %v, want ErrServicePanicked", panicOutcome.err)
	}
	if strings.Contains(panicOutcome.err.Error(), "private panic") {
		t.Fatalf("panic error leaked private text: %v", panicOutcome.err)
	}
	deadline := time.After(time.Second)
	for {
		if err := runtime.Send(panicTargetID, callTestCommand); errors.Is(err, ErrServiceClosed) {
			break
		}
		select {
		case <-deadline:
			t.Fatal("panic target did not stop")
		case <-time.After(time.Millisecond):
		}
	}
}

// 验证 Runtime Stop 先完成所有 PendingCall，再等待仍在执行的目标 Handler 真实返回。
func TestRuntimeStopCompletesPendingCalls(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := newBlockingCallTargetService()
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	caller := &callService{target: targetID, outcomes: make(chan callOutcome, 1)}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		t.Fatalf("CreateService(caller) error = %v", err)
	}
	if err := runtime.Send(callerID, callTestCommand); err != nil {
		t.Fatalf("Send(call) error = %v", err)
	}
	waitSignal(t, target.entered)
	stopResult := make(chan error, 1)
	go func() { stopResult <- runtime.Stop(context.Background()) }()
	outcome := waitCallOutcome(t, caller.outcomes)
	if !errors.Is(outcome.err, ErrRuntimeStopping) {
		t.Fatalf("Call during Stop error = %v, want ErrRuntimeStopping", outcome.err)
	}
	if runtime.pendingCalls.hasPending() {
		t.Fatal("Runtime Stop left a PendingCall")
	}
	target.releaseNow()
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
}

// 验证调用方 Handler 等待 Call 时保持 run-to-completion，不会重入处理后续消息。
func TestCallKeepsServiceRunToCompletion(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	target := newBlockingCallTargetService()
	targetID, err := runtime.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(target) error = %v", err)
	}
	caller := &runToCompletionCallService{target: targetID, firstDone: make(chan struct{}), secondHandled: make(chan struct{})}
	callerID, err := runtime.CreateService(context.Background(), caller)
	if err != nil {
		t.Fatalf("CreateService(caller) error = %v", err)
	}
	if err := runtime.Send(callerID, callTestCommand, "first"); err != nil {
		t.Fatalf("Send(first) error = %v", err)
	}
	waitSignal(t, target.entered)
	if err := runtime.Send(callerID, callTestCommand, "second"); err != nil {
		t.Fatalf("Send(second) error = %v", err)
	}
	select {
	case <-caller.secondHandled:
		t.Fatal("second message was handled while first Call was waiting")
	case <-time.After(20 * time.Millisecond):
	}
	target.releaseNow()
	waitSignal(t, caller.firstDone)
	waitSignal(t, caller.secondHandled)
}

// 验证超时或取消后的迟到完成结果只被丢弃，不会影响后来分配的其它 Session。
func TestPendingCallLateCompletionCannotCrossSessions(t *testing.T) {
	runtime := &Runtime{sessionIDs: newIDAllocator(1)}
	manager := newPendingCallManager(runtime)
	first, err := manager.begin(nil, 2)
	if err != nil {
		t.Fatalf("begin(first) error = %v", err)
	}
	second, err := manager.begin(nil, 3)
	if err != nil {
		t.Fatalf("begin(second) error = %v", err)
	}
	if !manager.cancel(first.session, context.Canceled) {
		t.Fatal("cancel(first) returned false")
	}
	if manager.complete(first.session, callResult{value: "late"}) {
		t.Fatal("late completion unexpectedly succeeded")
	}
	if !manager.complete(second.session, callResult{value: "second"}) {
		t.Fatal("second completion returned false")
	}
	firstResult := <-first.done
	if !errors.Is(firstResult.err, context.Canceled) {
		t.Fatalf("first result error = %v, want context.Canceled", firstResult.err)
	}
	secondResult := <-second.done
	if secondResult.value != "second" || secondResult.err != nil {
		t.Fatalf("second result = %#v, want second and nil error", secondResult)
	}
}

type callOutcome struct {
	value any
	err   error
}

type callTargetEvent struct {
	source ServiceID
}

type callTargetService struct {
	response   any
	handlerErr error
	panicValue any
	events     chan callTargetEvent
}

func (*callTargetService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *callTargetService) Handle(commandContext CommandContext, _ CommandID, _ ...any) (any, error) {
	if service.panicValue != nil {
		panic(service.panicValue)
	}
	if service.events != nil {
		service.events <- callTargetEvent{source: commandContext.Source()}
	}
	return service.response, service.handlerErr
}

func (*callTargetService) OnStop(context.Context) error {
	return nil
}

type blockingCallTargetService struct {
	entered     chan struct{}
	release     chan struct{}
	enteredOnce sync.Once
	releaseOnce sync.Once
}

func newBlockingCallTargetService() *blockingCallTargetService {
	return &blockingCallTargetService{
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}
}

func (*blockingCallTargetService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *blockingCallTargetService) Handle(CommandContext, CommandID, ...any) (any, error) {
	service.enteredOnce.Do(func() { close(service.entered) })
	<-service.release
	return "released", nil
}

func (*blockingCallTargetService) OnStop(context.Context) error {
	return nil
}

func (service *blockingCallTargetService) releaseNow() {
	service.releaseOnce.Do(func() { close(service.release) })
}

type callService struct {
	target      ServiceID
	callContext context.Context
	outcomes    chan callOutcome
}

func (*callService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *callService) Handle(commandContext CommandContext, _ CommandID, _ ...any) (any, error) {
	callContext := service.callContext
	if callContext == nil {
		callContext = context.Background()
	}
	value, err := commandContext.Call(callContext, service.target, callTestCommand)
	service.outcomes <- callOutcome{value: value, err: err}
	return nil, nil
}

func (*callService) OnStop(context.Context) error {
	return nil
}

type selfCallService struct {
	context  ServiceContext
	outcomes chan callOutcome
}

func (service *selfCallService) OnStart(_ context.Context, serviceContext ServiceContext) error {
	service.context = serviceContext
	return nil
}

func (service *selfCallService) Handle(commandContext CommandContext, _ CommandID, _ ...any) (any, error) {
	value, err := commandContext.Call(context.Background(), commandContext.Self(), callTestCommand)
	service.outcomes <- callOutcome{value: value, err: err}
	return nil, nil
}

func (*selfCallService) OnStop(context.Context) error {
	return nil
}

type onStartSelfCallService struct {
	callErr error
}

func (service *onStartSelfCallService) OnStart(_ context.Context, serviceContext ServiceContext) error {
	_, service.callErr = serviceContext.Call(context.Background(), serviceContext.Self(), callTestCommand)
	return nil
}

func (*onStartSelfCallService) Handle(CommandContext, CommandID, ...any) (any, error) {
	return nil, nil
}

func (*onStartSelfCallService) OnStop(context.Context) error {
	return nil
}

type runToCompletionCallService struct {
	target        ServiceID
	firstDone     chan struct{}
	secondHandled chan struct{}
	firstOnce     sync.Once
	secondOnce    sync.Once
}

func (*runToCompletionCallService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *runToCompletionCallService) Handle(commandContext CommandContext, _ CommandID, args ...any) (any, error) {
	if len(args) == 1 && args[0] == "first" {
		_, err := commandContext.Call(context.Background(), service.target, callTestCommand)
		service.firstOnce.Do(func() { close(service.firstDone) })
		return nil, err
	}
	if len(args) == 1 && args[0] == "second" {
		service.secondOnce.Do(func() { close(service.secondHandled) })
	}
	return nil, nil
}

func (*runToCompletionCallService) OnStop(context.Context) error {
	return nil
}

func waitCallOutcome(t *testing.T, outcomes <-chan callOutcome) callOutcome {
	t.Helper()
	select {
	case outcome := <-outcomes:
		return outcome
	case <-time.After(time.Second):
		t.Fatal("Call outcome did not arrive")
		return callOutcome{}
	}
}

func waitCallTargetEvent(t *testing.T, events <-chan callTargetEvent) callTargetEvent {
	t.Helper()
	select {
	case event := <-events:
		return event
	case <-time.After(time.Second):
		t.Fatal("target Handler event did not arrive")
		return callTargetEvent{}
	}
}

func newCallTestRuntime(t *testing.T, config Config) *Runtime {
	t.Helper()
	runtime, err := NewRuntime(config)
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := runtime.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("cleanup Close() error = %v", err)
		}
	})
	return runtime
}
