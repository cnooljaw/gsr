package cluster

import (
	"errors"
	"io"
	"net"
	"testing"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证响应读循环完成匹配 Call，断线只清理该节点剩余等待。
func TestResponseReaderCompletesAndFailsPendingCalls(t *testing.T) {
	connection, peer, codec := newTestStreamConnection(t)
	calls := NewClusterCallRegistry()
	t.Cleanup(func() { calls.Close() })
	first := beginResponseReaderCall(t, calls, "node-b")
	rejected := beginResponseReaderCall(t, calls, "node-b")
	remaining := beginResponseReaderCall(t, calls, "node-b")
	other := beginResponseReaderCall(t, calls, "node-c")
	run := startResponseReader(t, connection, calls, nil)
	response := ClusterEnvelope{Kind: FrameResponse, Session: first.session, Target: first.target, Command: first.command, Payload: []byte("result")}
	if err := codec.writeFrame(peer, response); err != nil {
		t.Fatal(err)
	}
	result := receiveClusterCallResult(t, first)
	if result.err != nil || string(result.payload) != "result" {
		t.Fatalf("response = %+v", result)
	}
	domainErr := runtime.NewStatusError(runtime.ErrorClassDomain, "room.full", "room is full")
	if err := codec.writeFrame(peer, ClusterEnvelope{Kind: FrameError, Session: rejected.session, Target: rejected.target, Command: rejected.command, Error: domainErr}); err != nil {
		t.Fatal(err)
	}
	if result := receiveClusterCallResult(t, rejected); !errors.Is(result.err, domainErr) {
		t.Fatalf("domain error = %v", result.err)
	}
	if err := peer.Close(); err != nil {
		t.Fatal(err)
	}
	<-run.done
	if !errors.Is(run.err, io.EOF) {
		t.Fatalf("reader stopped with %v", run.err)
	}
	if result := receiveClusterCallResult(t, remaining); !errors.Is(result.err, io.EOF) {
		t.Fatalf("pending failure = %v", result.err)
	}
	select {
	case <-other.done:
		t.Fatal("other node call was completed")
	default:
	}
	if calls.PendingCount() != 1 {
		t.Fatalf("pending count = %d", calls.PendingCount())
	}
}

// 验证错误路由、完整坏帧和重复响应被诊断并丢弃，诊断满载不阻塞有效回复。
func TestResponseReaderDropsInvalidResponsesWithoutBlocking(t *testing.T) {
	connection, peer, codec := newTestStreamConnection(t)
	calls := NewClusterCallRegistry()
	t.Cleanup(func() { calls.Close() })
	call := beginResponseReaderCall(t, calls, "node-b")
	diagnostics := make(chan responseReadFailure, 1)
	run := startResponseReader(t, connection, calls, diagnostics)
	wrong := ClusterEnvelope{Kind: FrameResponse, Session: call.session, Target: ServiceTarget{ServiceID: 99}, Command: call.command}
	for i := 0; i < 3; i++ {
		if err := codec.writeFrame(peer, wrong); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeFrameBytes(peer, []byte{0, 0, 0, 1, 255}); err != nil {
		t.Fatal(err)
	}
	response := ClusterEnvelope{Kind: FrameResponse, Session: call.session, Target: call.target, Command: call.command}
	for i := 0; i < 2; i++ {
		if err := codec.writeFrame(peer, response); err != nil {
			t.Fatal(err)
		}
	}
	if err := peer.Close(); err != nil {
		t.Fatal(err)
	}
	<-run.done
	if !errors.Is(run.err, io.EOF) || run.dropped != 4 {
		t.Fatalf("reader error=%v dropped=%d", run.err, run.dropped)
	}
	if result := receiveClusterCallResult(t, call); result.err != nil {
		t.Fatal(result.err)
	}
	select {
	case failure := <-diagnostics:
		if failure.node != "node-b" || failure.session != call.session || !errors.Is(failure.err, ErrClusterResponseMismatch) {
			t.Fatalf("diagnostic = %+v", failure)
		}
	default:
		t.Fatal("missing response diagnostic")
	}
}

// 验证非法帧长终止读循环、关闭连接，并将协议失败交回待完成 Call。
func TestResponseReaderInvalidLengthClosesConnection(t *testing.T) {
	connection, peer, _ := newTestStreamConnection(t)
	calls := NewClusterCallRegistry()
	t.Cleanup(func() { calls.Close() })
	call := beginResponseReaderCall(t, calls, "node-b")
	run := startResponseReader(t, connection, calls, nil)
	if err := writeFrameBytes(peer, []byte{255, 255, 255, 255}); err != nil {
		t.Fatal(err)
	}
	<-run.done
	if !errors.Is(run.err, ErrFrameTooLarge) {
		t.Fatalf("reader error = %v", run.err)
	}
	if result := receiveClusterCallResult(t, call); !errors.Is(result.err, ErrFrameTooLarge) {
		t.Fatalf("call error = %v", result.err)
	}
	if err := connection.write(ClusterEnvelope{Kind: FrameRequest, Target: call.target, Command: call.command}); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write after failure = %v", err)
	}
}

type responseReaderRun struct {
	done    chan struct{}
	dropped uint64
	err     error
}

// 验证连接管理组件主动关闭时，读循环退出并完成仍在等待的 Call。
func TestResponseReaderLocalCloseCompletesCalls(t *testing.T) {
	connection, _, _ := newTestStreamConnection(t)
	calls := NewClusterCallRegistry()
	t.Cleanup(func() { calls.Close() })
	call := beginResponseReaderCall(t, calls, "node-b")
	run := startResponseReader(t, connection, calls, nil)
	if err := connection.close(); err != nil {
		t.Fatal(err)
	}
	<-run.done
	if !errors.Is(run.err, io.ErrClosedPipe) {
		t.Fatalf("local close = %v", run.err)
	}
	if result := receiveClusterCallResult(t, call); !errors.Is(result.err, io.ErrClosedPipe) {
		t.Fatalf("pending error = %v", result.err)
	}
	if calls.PendingCount() != 0 {
		t.Fatalf("pending count = %d", calls.PendingCount())
	}
}

func startResponseReader(t *testing.T, connection *streamConnection, calls *ClusterCallRegistry, diagnostics chan<- responseReadFailure) *responseReaderRun {
	t.Helper()
	run := &responseReaderRun{done: make(chan struct{})}
	go func() {
		defer close(run.done)
		run.dropped, run.err = connection.receiveResponses("node-b", calls, diagnostics)
	}()
	t.Cleanup(func() {
		connection.close()
		<-run.done
	})
	return run
}

func beginResponseReaderCall(t *testing.T, calls *ClusterCallRegistry, node NodeID) *clusterPendingCall {
	t.Helper()
	call, err := calls.begin(11, node, ServiceTarget{ServiceID: 7}, clusterTestCommand)
	if err != nil {
		t.Fatal(err)
	}
	return call
}
