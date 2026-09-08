package cluster

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
)

// 验证旧连接只清理自己的等待，不影响同节点排队和新连接上的 Call。
func TestConnectionCallsFailureIsolation(t *testing.T) {
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	old := newConnectionCallsTest(t, registry, "node-b")
	next := newConnectionCallsTest(t, registry, "node-b")
	sent := beginResponseReaderCall(t, registry, "node-b")
	queued := beginResponseReaderCall(t, registry, "node-b")
	newCall := beginResponseReaderCall(t, registry, "node-b")
	bindConnectionCallTest(t, old, sent)
	bindConnectionCallTest(t, next, newCall)
	if count := old.fail(io.EOF); count != 1 {
		t.Fatalf("failed = %d", count)
	}
	if result := receiveClusterCallResult(t, sent); !errors.Is(result.err, io.EOF) {
		t.Fatal(result.err)
	}
	if registry.PendingCount() != 2 {
		t.Fatalf("pending = %d", registry.PendingCount())
	}
	if ready, err := old.bind(connectionCallRequest(queued)); ready || !errors.Is(err, io.EOF) {
		t.Fatalf("closed bind = %v, %v", ready, err)
	}
	bindConnectionCallTest(t, next, queued)
	for _, call := range []*clusterPendingCall{queued, newCall} {
		response := connectionCallRequest(call)
		response.Kind = FrameResponse
		if err := old.complete(response); !errors.Is(err, ErrClusterResponseMismatch) {
			t.Fatalf("old response = %v", err)
		}
		if err := registry.Complete("node-b", response); !errors.Is(err, ErrClusterResponseMismatch) {
			t.Fatalf("unbound completion bypass = %v", err)
		}
		if err := next.complete(response); err != nil {
			t.Fatal(err)
		}
	}
}

// 验证取消过的排队请求不再绑定，已绑定请求不能重复绑定或转投其它连接。
func TestConnectionCallsBindingIsSingleAttempt(t *testing.T) {
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	owner := newConnectionCallsTest(t, registry, "node-b")
	call := beginResponseReaderCall(t, registry, "node-b")
	registry.cancel(call.session, context.Canceled)
	if ready, err := owner.bind(connectionCallRequest(call)); ready || err != nil {
		t.Fatalf("cancelled bind = %v, %v", ready, err)
	}
	call = beginResponseReaderCall(t, registry, "node-b")
	bindConnectionCallTest(t, owner, call)
	for _, candidate := range []*connectionCalls{owner, newConnectionCallsTest(t, registry, "node-b")} {
		if ready, err := candidate.bind(connectionCallRequest(call)); ready || !errors.Is(err, ErrClusterResponseMismatch) {
			t.Fatalf("duplicate bind = %v, %v", ready, err)
		}
	}
}

// 验证连接失败、回复和取消并发时只完成一次并清除等待记录。
func TestConnectionCallsCompletionRace(t *testing.T) {
	for iteration := 0; iteration < 64; iteration++ {
		registry := NewClusterCallRegistry()
		owner := newConnectionCallsTest(t, registry, "node-b")
		call := beginResponseReaderCall(t, registry, "node-b")
		bindConnectionCallTest(t, owner, call)
		var workers sync.WaitGroup
		var winners atomic.Int32
		start := make(chan struct{})
		workers.Add(3)
		go func() {
			defer workers.Done()
			<-start
			winners.Add(int32(owner.fail(io.EOF)))
		}()
		go func() {
			defer workers.Done()
			<-start
			response := connectionCallRequest(call)
			response.Kind = FrameResponse
			if err := owner.complete(response); err == nil {
				winners.Add(1)
			} else if !errors.Is(err, ErrClusterSessionNotFound) {
				t.Error(err)
			}
		}()
		go func() {
			defer workers.Done()
			<-start
			if registry.cancel(call.session, context.Canceled) {
				winners.Add(1)
			}
		}()
		close(start)
		workers.Wait()
		if winners.Load() != 1 || registry.PendingCount() != 0 {
			t.Fatalf("winners=%d pending=%d", winners.Load(), registry.PendingCount())
		}
		receiveClusterCallResult(t, call)
		registry.Close()
	}
}

func newConnectionCallsTest(t *testing.T, registry *ClusterCallRegistry, node NodeID) *connectionCalls {
	t.Helper()
	owner, err := newConnectionCalls(registry, node)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}

func bindConnectionCallTest(t *testing.T, owner *connectionCalls, call *clusterPendingCall) {
	t.Helper()
	if ready, err := owner.bind(connectionCallRequest(call)); !ready || err != nil {
		t.Fatalf("bind = %v, %v", ready, err)
	}
}

func connectionCallRequest(call *clusterPendingCall) ClusterEnvelope {
	return ClusterEnvelope{Kind: FrameRequest, Session: call.session, Target: call.target, Command: call.command}
}

// 验证连接 owner 拒绝缺失注册表、空目标和错路由绑定，不消费有效等待。
func TestConnectionCallsRejectInvalidInput(t *testing.T) {
	if _, err := newConnectionCalls(nil, "node-b"); !errors.Is(err, ErrInvalidClusterCallRegistry) {
		t.Fatalf("nil registry = %v", err)
	}
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	if _, err := newConnectionCalls(registry, ""); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("empty node = %v", err)
	}
	owner := newConnectionCallsTest(t, registry, "node-b")
	call := beginResponseReaderCall(t, registry, "node-c")
	if ready, err := owner.bind(connectionCallRequest(call)); ready || !errors.Is(err, ErrClusterResponseMismatch) {
		t.Fatalf("wrong node bind = %v, %v", ready, err)
	}
	if registry.PendingCount() != 1 {
		t.Fatal("wrong route consumed pending call")
	}
	registry.Close()
	if ready, err := owner.bind(connectionCallRequest(call)); ready || !errors.Is(err, ErrClusterCallClosed) {
		t.Fatalf("closed registry bind = %v, %v", ready, err)
	}
}
