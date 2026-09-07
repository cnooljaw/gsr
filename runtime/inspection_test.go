package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 验证 Inspect 返回按 ServiceID 排序的只读 Service 快照和 Mailbox 深度。
func TestRuntimeInspectReportsServiceSnapshot(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{MailboxCapacity: 2})
	first := newBlockingCallTargetService()
	firstID, err := runtime.CreateService(context.Background(), first)
	if err != nil {
		t.Fatalf("CreateService(first) error = %v", err)
	}
	second := &callTargetService{}
	secondID, err := runtime.CreateService(context.Background(), second)
	if err != nil {
		t.Fatalf("CreateService(second) error = %v", err)
	}

	inspection := runtime.Inspect()
	if inspection.State != RuntimeRunning {
		t.Fatalf("Runtime state = %v, want Running", inspection.State)
	}
	if len(inspection.Services) != 2 || inspection.Services[0].ID != firstID || inspection.Services[1].ID != secondID {
		t.Fatalf("Services = %#v, want IDs [%d %d] in order", inspection.Services, firstID, secondID)
	}
	firstSnapshot := inspection.Services[0]
	if firstSnapshot.State != ServiceStateRunning || firstSnapshot.MailboxDepth != 0 || firstSnapshot.MailboxCapacity != 2 {
		t.Fatalf("first Service snapshot = %#v, want running empty mailbox with capacity 2", firstSnapshot)
	}
	if !firstSnapshot.RunnerActive || firstSnapshot.RunnerCompleted || firstSnapshot.ShutdownDeadlineExceeded {
		t.Fatalf("first runner flags = %#v, want active and incomplete", firstSnapshot)
	}

	if err := runtime.Send(firstID, callTestCommand, "running"); err != nil {
		t.Fatalf("Send(running) error = %v", err)
	}
	waitSignal(t, first.entered)
	if err := runtime.Send(firstID, callTestCommand, "queued"); err != nil {
		t.Fatalf("Send(queued) error = %v", err)
	}
	if err := runtime.Send(firstID, callTestCommand, "queued-2"); err != nil {
		t.Fatalf("Send(queued-2) error = %v", err)
	}
	if err := runtime.Send(firstID, callTestCommand, "rejected"); !errors.Is(err, ErrMailboxFull) {
		t.Fatalf("Send(rejected) error = %v, want ErrMailboxFull", err)
	}

	inspection = runtime.Inspect()
	firstSnapshot = inspection.Services[0]
	if firstSnapshot.MailboxDepth != 2 {
		t.Fatalf("queued Mailbox depth = %d, want 2", firstSnapshot.MailboxDepth)
	}
	if inspection.Metrics.AdmissionRejected == 0 {
		t.Fatalf("AdmissionRejected = %d, want a rejected Send", inspection.Metrics.AdmissionRejected)
	}
	first.releaseNow()
}

// 验证 Inspect 能观察 PendingCall、Call 超时和 Timer 的活动计数。
func TestRuntimeInspectReportsPendingCallAndTimerMetrics(t *testing.T) {
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
	inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.PendingCallCount == 1
	})
	if inspection.PendingCallCount != 1 {
		t.Fatalf("PendingCallCount = %d, want 1", inspection.PendingCallCount)
	}
	timerService := &timerRecordingService{events: make(chan timerEvent, 1)}
	if _, err := runtime.CreateService(context.Background(), timerService); err != nil {
		t.Fatalf("CreateService(timer) error = %v", err)
	}
	if _, err := timerService.context.After(time.Second, timerTestCommand); err != nil {
		t.Fatalf("After() error = %v", err)
	}
	clock.Advance(time.Second)
	if outcome := waitCallOutcome(t, caller.outcomes); !errors.Is(outcome.err, ErrCallTimeout) {
		t.Fatalf("Call error = %v, want ErrCallTimeout", outcome.err)
	}
	waitTimerEvent(t, timerService.events)
	inspection = waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.Metrics.CallTimeouts == 1 && value.Metrics.TimersFired == 1 && value.PendingCallCount == 0
	})
	if inspection.Metrics.CallTimeouts != 1 || inspection.Metrics.TimersFired != 1 || inspection.Metrics.TimersAdmitted != 1 || inspection.TimerCount != 0 || inspection.PendingCallCount != 0 {
		t.Fatalf("timeout inspection = %#v, want one timeout, one timer admission and no active work", inspection)
	}
	target.releaseNow()
}

// 验证 Runtime Stop 会先完成 PendingCall，再等待阻塞 Handler 真实返回并反映清理结果。
func TestRuntimeInspectReportsPendingCallCleanupOnStop(t *testing.T) {
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
	if inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.PendingCallCount == 1
	}); inspection.PendingCallCount != 1 {
		t.Fatalf("PendingCallCount before Stop = %d, want 1", inspection.PendingCallCount)
	}
	stopResult := make(chan error, 1)
	go func() {
		stopResult <- runtime.Stop(context.Background())
	}()
	if outcome := waitCallOutcome(t, caller.outcomes); !errors.Is(outcome.err, ErrRuntimeStopping) {
		t.Fatalf("Call during Stop error = %v, want ErrRuntimeStopping", outcome.err)
	}
	inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.PendingCallCount == 0
	})
	if inspection.PendingCallCount != 0 {
		t.Fatalf("PendingCallCount during Stop = %d, want 0", inspection.PendingCallCount)
	}
	target.releaseNow()
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if inspection := runtime.Inspect(); inspection.State != RuntimeStopped || inspection.PendingCallCount != 0 {
		t.Fatalf("post-Stop inspection = %#v, want Stopped with no PendingCall", inspection)
	}
}

// 验证 Timer 投递失败会被公开 Inspection 计数且不会自动重试或残留活动 Timer。
func TestRuntimeInspectReportsTimerAdmissionFailure(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{MailboxCapacity: 1})
	service := newBlockingTimerService()
	serviceID, err := runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	if err := runtime.Send(serviceID, timerTestCommand, "running"); err != nil {
		t.Fatalf("Send(running) error = %v", err)
	}
	waitSignal(t, service.started)
	if err := runtime.Send(serviceID, timerTestCommand, "queued"); err != nil {
		t.Fatalf("Send(queued) error = %v", err)
	}
	if _, err := service.context.After(0, timerTestCommand, "timer"); err != nil {
		t.Fatalf("After() error = %v", err)
	}
	clock.Advance(0)
	inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.Metrics.TimerAdmissionRejected == 1
	})
	if inspection.TimerCount != 0 || inspection.Metrics.TimersFired != 1 || inspection.Metrics.TimersAdmitted != 0 || inspection.Metrics.TimerAdmissionRejected != 1 {
		t.Fatalf("timer failure inspection = %#v, want one rejected fire and no active timer", inspection)
	}
	close(service.release)
}

// 验证调用方修改 RuntimeInspection 返回值不会改变下一次快照中的 Runtime 状态。
func TestRuntimeInspectReturnsIndependentSnapshots(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{})
	service := &callTargetService{}
	serviceID, err := runtime.CreateService(context.Background(), service)
	if err != nil {
		t.Fatalf("CreateService() error = %v", err)
	}
	inspection := runtime.Inspect()
	if len(inspection.Services) != 1 || inspection.Services[0].ID != serviceID {
		t.Fatalf("initial inspection = %#v, want one Service %d", inspection, serviceID)
	}
	inspection.Services[0].ID = 999
	inspection.Services[0].State = ServiceStateStopped
	inspection.Services = nil
	next := runtime.Inspect()
	if len(next.Services) != 1 || next.Services[0].ID != serviceID || next.Services[0].State != ServiceStateRunning {
		t.Fatalf("next inspection = %#v, want an independent running snapshot", next)
	}
}

// 验证包含 Timer 和 PendingCall 的 Runtime 关闭后进入 Closed，并且 Inspection 不再报告 Core 残留资源。
func TestRuntimeInspectReportsClosedResourceQuiescence(t *testing.T) {
	clock := newFakeClock()
	runtime := newTimerTestRuntime(t, clock, Config{})
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
	timerService := &timerRecordingService{events: make(chan timerEvent, 1)}
	if _, err := runtime.CreateService(context.Background(), timerService); err != nil {
		t.Fatalf("CreateService(timer) error = %v", err)
	}
	if _, err := timerService.context.After(time.Hour, timerTestCommand); err != nil {
		t.Fatalf("After() error = %v", err)
	}
	inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.PendingCallCount == 1 && value.TimerCount == 1
	})
	if inspection.PendingCallCount != 1 || inspection.TimerCount != 1 {
		t.Fatalf("active resource inspection = %#v, want one PendingCall and Timer", inspection)
	}
	stopResult := make(chan error, 1)
	go func() {
		stopResult <- runtime.Stop(context.Background())
	}()
	if outcome := waitCallOutcome(t, caller.outcomes); !errors.Is(outcome.err, ErrRuntimeStopping) {
		t.Fatalf("Call during Stop error = %v, want ErrRuntimeStopping", outcome.err)
	}
	target.releaseNow()
	if err := <-stopResult; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := runtime.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	inspection = runtime.Inspect()
	if inspection.State != RuntimeClosed || len(inspection.Services) != 0 || inspection.PendingCallCount != 0 || inspection.TimerCount != 0 || inspection.Metrics.TimersCancelled != 1 {
		t.Fatalf("closed resource inspection = %#v, want Closed with no residual resources", inspection)
	}
}

// 验证 Handler 错误、panic 和停止截止时间到期都进入只读故障计数或 Service 快照。
func TestRuntimeInspectReportsFailureCountersAndDeadline(t *testing.T) {
	runtime := newCallTestRuntime(t, Config{ShutdownTimeout: 10 * time.Millisecond})
	failing := &callTargetService{handlerErr: errors.New("internal")}
	failingID, err := runtime.CreateService(context.Background(), failing)
	if err != nil {
		t.Fatalf("CreateService(failing) error = %v", err)
	}
	if err := runtime.Send(failingID, callTestCommand); err != nil {
		t.Fatalf("Send(failing) error = %v", err)
	}
	inspection := waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.Metrics.HandlerErrors == 1
	})
	if inspection.Metrics.HandlerErrors != 1 || inspection.Metrics.Panics != 0 {
		t.Fatalf("Handler error metrics = %#v, want one error and no panic", inspection.Metrics)
	}
	panicService := &callTargetService{panicValue: "panic"}
	panicID, err := runtime.CreateService(context.Background(), panicService)
	if err != nil {
		t.Fatalf("CreateService(panic) error = %v", err)
	}
	if err := runtime.Send(panicID, callTestCommand); err != nil {
		t.Fatalf("Send(panic) error = %v", err)
	}
	inspection = waitRuntimeInspection(t, runtime, func(value RuntimeInspection) bool {
		return value.Metrics.Panics == 1
	})
	if inspection.Metrics.Panics != 1 || inspection.Metrics.HandlerErrors != 2 {
		t.Fatalf("panic metrics = %#v, want one panic and two total handler errors", inspection.Metrics)
	}

	blocked := newBlockingCallTargetService()
	blockedID, err := runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatalf("CreateService(blocked) error = %v", err)
	}
	if err := runtime.Send(blockedID, callTestCommand); err != nil {
		t.Fatalf("Send(blocked) error = %v", err)
	}
	waitSignal(t, blocked.entered)
	if err := runtime.Stop(context.Background()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Stop() error = %v, want context deadline exceeded", err)
	}
	inspection = runtime.Inspect()
	var blockedSnapshot ServiceInspection
	for _, service := range inspection.Services {
		if service.ID == blockedID {
			blockedSnapshot = service
			break
		}
	}
	if blockedSnapshot.State != ServiceStateStopping || !blockedSnapshot.RunnerActive || !blockedSnapshot.ShutdownDeadlineExceeded {
		t.Fatalf("blocked Service snapshot = %#v, want stopping active runner past deadline", blockedSnapshot)
	}
	if inspection.Metrics.StopTimeouts != 1 {
		t.Fatalf("StopTimeouts = %d, want 1", inspection.Metrics.StopTimeouts)
	}
	blocked.releaseNow()
	if err := runtime.Close(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() after releasing blocked Service error = %v", err)
	}
}

func waitRuntimeInspection(t *testing.T, runtime *Runtime, condition func(RuntimeInspection) bool) RuntimeInspection {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		inspection := runtime.Inspect()
		if condition(inspection) {
			return inspection
		}
		select {
		case <-deadline:
			t.Fatalf("inspection condition not reached: %#v", inspection)
			return RuntimeInspection{}
		case <-time.After(time.Millisecond):
		}
	}
}
