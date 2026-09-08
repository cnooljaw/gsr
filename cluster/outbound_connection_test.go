package cluster

import (
	"context"
	"errors"
	"io"
	"os"
	"testing"
	"time"
)

// 验证排队期间已取消的 Call 不写任何字节，也不重新创建等待记录。
func TestOutboundConnectionSkipsCancelledCall(t *testing.T) {
	stream, peer, _ := newTestStreamConnection(t)
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	connection, err := newOutboundConnection(stream, registry, "node-b")
	if err != nil {
		t.Fatal(err)
	}
	call := beginResponseReaderCall(t, registry, "node-b")
	registry.cancel(call.session, context.Canceled)
	if err := connection.write(connectionCallRequest(call)); err != nil {
		t.Fatal(err)
	}
	if err := peer.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var header [4]byte
	if n, err := peer.Read(header[:]); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("cancelled request wrote %d bytes: %v", n, err)
	}
}

// 验证对端一读到请求就回复时，绑定已完成，结果不会因读写并发而丢失。
func TestOutboundConnectionAcceptsImmediateResponse(t *testing.T) {
	stream, peer, codec := newTestStreamConnection(t)
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	connection, err := newOutboundConnection(stream, registry, "node-b")
	if err != nil {
		t.Fatal(err)
	}
	call := beginResponseReaderCall(t, registry, "node-b")
	readDone := make(chan error, 1)
	go func() {
		_, err := stream.receiveResponses(connection.calls, nil)
		readDone <- err
	}()
	peerDone := make(chan error, 1)
	go func() {
		request, err := codec.readFrame(peer)
		if err == nil {
			request.Kind = FrameResponse
			request.Payload = []byte("ready")
			err = codec.writeFrame(peer, request)
		}
		peerDone <- err
	}()
	defer func() {
		stream.close()
		peer.Close()
		<-readDone
		<-peerDone
	}()
	if err := connection.write(connectionCallRequest(call)); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-call.done:
		if result.err != nil || string(result.payload) != "ready" {
			t.Fatalf("result = %+v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("immediate response was lost")
	}
}

// 验证写入失败只完成当前连接等待，不完成同节点尚未绑定的排队 Call。
func TestOutboundConnectionWriteFailureIsolation(t *testing.T) {
	stream, peer, _ := newTestStreamConnection(t)
	registry := NewClusterCallRegistry()
	t.Cleanup(func() { registry.Close() })
	connection, err := newOutboundConnection(stream, registry, "node-b")
	if err != nil {
		t.Fatal(err)
	}
	call := beginResponseReaderCall(t, registry, "node-b")
	queued := beginResponseReaderCall(t, registry, "node-b")
	peer.Close()
	if err := connection.write(connectionCallRequest(call)); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("write = %v", err)
	}
	if result := receiveClusterCallResult(t, call); !errors.Is(result.err, io.ErrClosedPipe) {
		t.Fatal(result.err)
	}
	select {
	case <-queued.done:
		t.Fatal("queued call was failed by old connection")
	default:
	}
}
