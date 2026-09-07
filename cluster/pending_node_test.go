package cluster

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 验证节点失败只完成该节点的等待、保留原始原因，且不永久禁止后续调用。
func TestClusterPendingNodeFailureIsIsolated(t *testing.T) {
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	var affected []*clusterPendingCall
	for i := 0; i < 3; i++ {
		call, err := registry.begin(11, "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
		if err != nil {
			t.Fatal(err)
		}
		affected = append(affected, call)
	}
	other, err := registry.begin(12, "node-c", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatal(err)
	}
	count, err := registry.failNode("node-b", io.ErrUnexpectedEOF)
	if err != nil || count != 3 {
		t.Fatalf("failNode = %d, %v", count, err)
	}
	for _, call := range affected {
		result := receiveClusterCallResult(t, call)
		if !errors.Is(result.err, io.ErrUnexpectedEOF) {
			t.Fatalf("call error = %v", result.err)
		}
		response := ClusterEnvelope{Kind: FrameResponse, Session: call.session, Target: call.target, Command: call.command}
		if err := registry.Complete("node-b", response); !errors.Is(err, ErrClusterSessionNotFound) {
			t.Fatalf("late reply = %v", err)
		}
	}
	if registry.PendingCount() != 1 {
		t.Fatalf("PendingCount = %d", registry.PendingCount())
	}
	select {
	case <-other.done:
		t.Fatal("unrelated node call completed")
	default:
	}
	if count, err := registry.failNode("node-b", io.EOF); err != nil || count != 0 {
		t.Fatalf("repeated failure = %d, %v", count, err)
	}
	next, err := registry.begin(11, "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil || next.session <= other.session {
		t.Fatalf("new call = %+v, %v", next, err)
	}
	if err := registry.Complete("node-c", ClusterEnvelope{Kind: FrameResponse, Session: other.session, Target: other.target, Command: other.command}); err != nil {
		t.Fatal(err)
	}
	if result := receiveClusterCallResult(t, other); result.err != nil {
		t.Fatal(result.err)
	}
}

// 验证断线、回复和调用方取消竞争时，恰好一个路径获得完成权。
func TestClusterPendingNodeFailureRacesCompletion(t *testing.T) {
	for iteration := 0; iteration < 64; iteration++ {
		registry := NewClusterCallRegistry()
		call, err := registry.begin(11, "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		var workers sync.WaitGroup
		var winners atomic.Int32
		workers.Add(3)
		go func() {
			defer workers.Done()
			<-start
			count, err := registry.failNode("node-b", io.EOF)
			if err != nil {
				t.Error(err)
			}
			winners.Add(int32(count))
		}()
		go func() {
			defer workers.Done()
			<-start
			err := registry.Complete("node-b", ClusterEnvelope{Kind: FrameResponse, Session: call.session, Target: call.target, Command: call.command})
			if err == nil {
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
		result := receiveClusterCallResult(t, call)
		if result.err != nil && !errors.Is(result.err, io.EOF) && !errors.Is(result.err, context.Canceled) {
			t.Fatal(result.err)
		}
		if err := registry.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// 验证缺失节点或失败原因时不清理等待，未初始化注册表返回明确错误。
func TestClusterPendingNodeFailureRejectsInvalidInput(t *testing.T) {
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	if _, err := registry.begin(11, "node-b", ServiceTarget{ServiceID: 7}, clusterTestCommand); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.failNode("", io.EOF); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("empty node = %v", err)
	}
	if _, err := registry.failNode("node-b", nil); !errors.Is(err, ErrInvalidClusterCall) {
		t.Fatalf("nil cause = %v", err)
	}
	if registry.PendingCount() != 1 {
		t.Fatal("invalid failure removed a call")
	}
	var uninitialized ClusterCallRegistry
	if _, err := uninitialized.failNode("node-b", io.EOF); !errors.Is(err, ErrInvalidClusterCallRegistry) {
		t.Fatalf("uninitialized registry = %v", err)
	}
}

// 验证节点失败直接恢复等待中的 ClusterClient.Call，并将底层原因交回调用方。
func TestClusterPendingNodeFailureWakesClient(t *testing.T) {
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	admitted := make(chan ClusterEnvelope, 1)
	client, transport, sender := newClientCallTestClient(t, registry, func(_ NodeID, request ClusterEnvelope) {
		admitted <- request
	}, time.Second)
	defer closeCallTestResources(t, transport, sender)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		_, err := client.Call(ctx, "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand)
		done <- err
	}()
	defer func() {
		cancel()
		<-exited
	}()
	select {
	case <-admitted:
	case <-time.After(time.Second):
		t.Fatal("request was not admitted")
	}
	if count, err := registry.failNode("node-b", io.ErrUnexpectedEOF); err != nil || count != 1 {
		t.Fatalf("failNode = %d, %v", count, err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("Call error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Call did not resume after node failure")
	}
}
