package cluster

import (
	"context"
	"errors"
	"testing"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// 验证 Cluster PendingCall 绑定 Source 并为每次调用分配不复用的单调 Session。
func TestClusterPendingCallsRegisterSourceAndSession(t *testing.T) {
	registry := newClusterPendingCalls()
	first, err := registry.begin(runtime.ServiceID(11), "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin(first) error = %v, want nil", err)
	}
	second, err := registry.begin(runtime.ServiceID(12), "node-b", ServiceTarget{ServiceID: 8}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin(second) error = %v, want nil", err)
	}
	if first.session != 1 || second.session != 2 {
		t.Fatalf("sessions = %d, %d, want 1, 2", first.session, second.session)
	}
	if first.source != 11 || second.source != 12 {
		t.Fatalf("sources = %d, %d, want 11, 12", first.source, second.source)
	}
	if got := registry.pendingCount(); got != 2 {
		t.Fatalf("pendingCount() = %d, want 2", got)
	}
	registry.cancel(first.session, context.Canceled)
	registry.cancel(second.session, context.Canceled)
}

// 验证成功 Response 只完成匹配 PendingCall 一次，并隔离响应载荷。
func TestClusterPendingCallsCompleteResponseOnce(t *testing.T) {
	registry := newClusterPendingCalls()
	pending, err := registry.begin(runtime.ServiceID(11), "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin() error = %v, want nil", err)
	}
	envelope := ClusterEnvelope{
		Kind:    FrameResponse,
		Session: pending.session,
		Target:  pending.target,
		Command: pending.command,
		Payload: []byte("result"),
	}
	if err := registry.complete("node-b", envelope); err != nil {
		t.Fatalf("complete() error = %v, want nil", err)
	}
	envelope.Payload[0] = 'X'
	result := receiveClusterCallResult(t, pending)
	if result.err != nil {
		t.Fatalf("result.err = %v, want nil", result.err)
	}
	if string(result.payload) != "result" {
		t.Fatalf("result.payload = %q, want result", result.payload)
	}
	if got := registry.pendingCount(); got != 0 {
		t.Fatalf("pendingCount() = %d, want 0", got)
	}
	if err := registry.complete("node-b", envelope); !errors.Is(err, ErrClusterSessionNotFound) {
		t.Fatalf("duplicate complete() error = %v, want ErrClusterSessionNotFound", err)
	}
}

// 验证 Error 帧只把结构化远端错误交给原 PendingCall，不产生业务载荷。
func TestClusterPendingCallsCompleteRemoteError(t *testing.T) {
	registry := newClusterPendingCalls()
	pending, err := registry.begin(runtime.ServiceID(11), "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin() error = %v, want nil", err)
	}
	wantErr := runtime.NewStatusError(runtime.ErrorClassDomain, runtime.ErrorCode("room.full"), "room is full")
	if err := registry.complete("node-b", ClusterEnvelope{
		Kind:    FrameError,
		Session: pending.session,
		Target:  pending.target,
		Command: pending.command,
		Error:   wantErr,
	}); err != nil {
		t.Fatalf("complete(error) error = %v, want nil", err)
	}
	result := receiveClusterCallResult(t, pending)
	if result.payload != nil {
		t.Fatalf("result.payload = %v, want nil", result.payload)
	}
	if !errors.Is(result.err, wantErr) {
		t.Fatalf("result.err = %v, want %v", result.err, wantErr)
	}
}

// 验证错误来源、目标或 Command 不匹配时保留 PendingCall，未知 Session 不影响其它调用。
func TestClusterPendingCallsRejectMismatchedResponses(t *testing.T) {
	registry := newClusterPendingCalls()
	pending, err := registry.begin(runtime.ServiceID(11), "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin() error = %v, want nil", err)
	}
	base := ClusterEnvelope{
		Kind:    FrameResponse,
		Session: pending.session,
		Target:  pending.target,
		Command: pending.command,
		Payload: []byte("result"),
	}
	cases := []struct {
		name string
		from NodeID
		env  ClusterEnvelope
	}{
		{name: "wrong node", from: "node-c", env: base},
		{name: "wrong target", from: "node-b", env: ClusterEnvelope{Kind: FrameResponse, Session: pending.session, Target: ServiceTarget{ServiceID: 8}, Command: pending.command}},
		{name: "wrong command", from: "node-b", env: ClusterEnvelope{Kind: FrameResponse, Session: pending.session, Target: pending.target, Command: clusterTestCommand + 1}},
	}
	for _, testCase := range cases {
		if err := registry.complete(testCase.from, testCase.env); !errors.Is(err, ErrClusterResponseMismatch) {
			t.Errorf("%s complete() error = %v, want ErrClusterResponseMismatch", testCase.name, err)
		}
	}
	if got := registry.pendingCount(); got != 1 {
		t.Fatalf("pendingCount() after mismatches = %d, want 1", got)
	}
	if err := registry.complete("node-b", base); err != nil {
		t.Fatalf("matching complete() error = %v, want nil", err)
	}
	receiveClusterCallResult(t, pending)
	if err := registry.complete("node-b", base); !errors.Is(err, ErrClusterSessionNotFound) {
		t.Fatalf("unknown complete() error = %v, want ErrClusterSessionNotFound", err)
	}
}

// 验证取消会完成原 PendingCall，迟到响应不会误完成后续 Session。
func TestClusterPendingCallsCancelRejectsLateResponse(t *testing.T) {
	registry := newClusterPendingCalls()
	pending, err := registry.begin(runtime.ServiceID(11), "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin() error = %v, want nil", err)
	}
	if canceled := registry.cancel(pending.session, context.DeadlineExceeded); !canceled {
		t.Fatal("cancel() = false, want true")
	}
	result := receiveClusterCallResult(t, pending)
	if !errors.Is(result.err, context.DeadlineExceeded) {
		t.Fatalf("result.err = %v, want context.DeadlineExceeded", result.err)
	}
	if err := registry.complete("node-b", ClusterEnvelope{
		Kind:    FrameResponse,
		Session: pending.session,
		Target:  pending.target,
		Command: pending.command,
	}); !errors.Is(err, ErrClusterSessionNotFound) {
		t.Fatalf("late complete() error = %v, want ErrClusterSessionNotFound", err)
	}
	if got := registry.pendingCount(); got != 0 {
		t.Fatalf("pendingCount() = %d, want 0", got)
	}
}

// 验证关闭会一次性完成全部 PendingCall，关闭后不再接受新 Session。
func TestClusterPendingCallsCloseCompletesAll(t *testing.T) {
	registry := newClusterPendingCalls()
	first, err := registry.begin(runtime.ServiceID(11), "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin(first) error = %v, want nil", err)
	}
	second, err := registry.begin(runtime.ServiceID(12), "node-c", ServiceTarget{ServiceID: 8}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin(second) error = %v, want nil", err)
	}
	if err := registry.close(ErrClusterCallClosed); err != nil {
		t.Fatalf("close() error = %v, want nil", err)
	}
	for _, pending := range []*clusterPendingCall{first, second} {
		result := receiveClusterCallResult(t, pending)
		if !errors.Is(result.err, ErrClusterCallClosed) {
			t.Fatalf("close result error = %v, want ErrClusterCallClosed", result.err)
		}
	}
	if _, err := registry.begin(runtime.ServiceID(13), "node-b", ServiceTarget{ServiceID: 9}, clusterTestCommand); !errors.Is(err, ErrClusterCallClosed) {
		t.Fatalf("begin(after close) error = %v, want ErrClusterCallClosed", err)
	}
	if err := registry.close(nil); err != nil {
		t.Fatalf("second close() error = %v, want nil", err)
	}
}

// 验证 ClusterSessionID 到达最大值后拒绝后续分配而不回绕到已发放 ID。
func TestClusterPendingCallsExhaustsSessionIDs(t *testing.T) {
	registry := newClusterPendingCalls()
	registry.next = ^ClusterSessionID(0)
	pending, err := registry.begin(runtime.ServiceID(11), "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatalf("begin(max) error = %v, want nil", err)
	}
	if pending.session != ^ClusterSessionID(0) {
		t.Fatalf("session = %d, want max uint64", pending.session)
	}
	if _, err := registry.begin(runtime.ServiceID(12), "node-b", ServiceTarget{ServiceID: 8}, clusterTestCommand); !errors.Is(err, runtime.ErrIDExhausted) {
		t.Fatalf("begin(after max) error = %v, want runtime.ErrIDExhausted", err)
	}
	registry.cancel(pending.session, context.Canceled)
}

func receiveClusterCallResult(t *testing.T, pending *clusterPendingCall) clusterCallResult {
	t.Helper()
	result, open := <-pending.done
	if open {
		if _, stillOpen := <-pending.done; stillOpen {
			t.Fatal("pending result channel remains open after completion")
		}
	}
	return result
}
