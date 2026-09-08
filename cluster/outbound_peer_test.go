package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

func newTestOutboundPeer(t *testing.T, dial func(context.Context, string, string) (net.Conn, error)) (*outboundPeer, *ClusterCallRegistry) {
	t.Helper()
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewClusterCallRegistry()
	peer, err := newOutboundPeer(outboundPeerConfig{
		node: "node-b", address: "localhost:1234", codec: codec, calls: registry,
		dialTimeout: 30 * time.Millisecond, writeTimeout: time.Second, dial: dial,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		peer.close()
		registry.Close()
	})
	return peer, registry
}

// 验证建连失败只结束当前 Call，下一请求可连接并复用，关闭等待响应读循环退出。
func TestOutboundPeerConnectFailureAndReuse(t *testing.T) {
	var attempts atomic.Int32
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close() })
	peer, registry := newTestOutboundPeer(t, func(context.Context, string, string) (net.Conn, error) {
		if attempts.Add(1) == 1 {
			return nil, io.ErrUnexpectedEOF
		}
		return client, nil
	})
	first := beginResponseReaderCall(t, registry, "node-b")
	next := beginResponseReaderCall(t, registry, "node-b")
	if err := peer.Send("node-b", connectionCallRequest(first)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	if result := receiveClusterCallResult(t, first); !errors.Is(result.err, io.ErrUnexpectedEOF) {
		t.Fatal(result.err)
	}
	if registry.PendingCount() != 1 {
		t.Fatal("dial failure removed queued call")
	}
	done := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			request, err := peer.config.codec.readFrame(server)
			if err != nil {
				done <- err
				return
			}
			request.Kind = FrameResponse
			if err := peer.config.codec.writeFrame(server, request); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	for i := 0; i < 2; i++ {
		if i == 1 {
			next = beginResponseReaderCall(t, registry, "node-b")
		}
		if err := peer.Send("node-b", connectionCallRequest(next)); err != nil {
			t.Fatal(err)
		}
		if result := awaitPeerCall(t, next); result.err != nil {
			t.Fatal(result.err)
		}
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatalf("dial attempts = %d", attempts.Load())
	}
}

func awaitPeerCall(t *testing.T, call *clusterPendingCall) clusterCallResult {
	t.Helper()
	select {
	case result := <-call.done:
		return result
	case <-time.After(time.Second):
		t.Fatal("call did not finish")
		return clusterCallResult{}
	}
}

// 验证有限 Dial 超时保留原因，关闭可取消正在进行的 Dial 且阻止再次拨号。
func TestOutboundPeerDialTimeoutAndClose(t *testing.T) {
	for _, closing := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "close"}[closing], func(t *testing.T) {
			entered := make(chan struct{})
			peer, registry := newTestOutboundPeer(t, func(ctx context.Context, _, _ string) (net.Conn, error) {
				close(entered)
				<-ctx.Done()
				return nil, ctx.Err()
			})
			call := beginResponseReaderCall(t, registry, "node-b")
			done := make(chan error, 1)
			go func() { done <- peer.Send("node-b", connectionCallRequest(call)) }()
			<-entered
			want := context.DeadlineExceeded
			if closing {
				peer.close()
				want = context.Canceled
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("dial did not stop")
			}
			if result := receiveClusterCallResult(t, call); !errors.Is(result.err, want) {
				t.Fatal(result.err)
			}
		})
	}
}

// 验证建连前已取消的 Call 不拨号，建连期间取消的 Call 也不会写到新连接。
func TestOutboundPeerCancelledCall(t *testing.T) {
	var attempts atomic.Int32
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close() })
	var cancelCall func()
	peer, registry := newTestOutboundPeer(t, func(context.Context, string, string) (net.Conn, error) {
		attempts.Add(1)
		cancelCall()
		return client, nil
	})
	call := beginResponseReaderCall(t, registry, "node-b")
	registry.cancel(call.session, context.Canceled)
	if err := peer.Send("node-b", connectionCallRequest(call)); err != nil || attempts.Load() != 0 {
		t.Fatalf("cancelled call dialed: %v", err)
	}
	call = beginResponseReaderCall(t, registry, "node-b")
	cancelCall = func() { registry.cancel(call.session, context.Canceled) }
	if err := peer.Send("node-b", connectionCallRequest(call)); err != nil {
		t.Fatal(err)
	}
	if err := server.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var data [1]byte
	if n, err := server.Read(data[:]); n != 0 || err == nil {
		t.Fatalf("cancelled call wrote bytes: %d, %v", n, err)
	}
}

// 验证真实 TCP 断线后无后台拨号，下一请求触发新连接且收到原连接上的回复。
func TestOutboundPeerTCPReconnect(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	peer, registry := newTestOutboundPeer(t, nil)
	peer.config.address = listener.Addr().String()
	peer.config.dialTimeout = time.Second
	serverDone := make(chan error, 1)
	var accepts atomic.Int32
	go func() {
		for i := 0; i < 2; i++ {
			socket, err := listener.Accept()
			if err != nil {
				serverDone <- err
				return
			}
			accepts.Add(1)
			request, err := peer.config.codec.readFrame(socket)
			if err == nil {
				request.Kind = FrameResponse
				err = peer.config.codec.writeFrame(socket, request)
			}
			socket.Close()
			if err != nil {
				serverDone <- err
				return
			}
		}
		serverDone <- nil
	}()
	for i := 0; i < 2; i++ {
		call := beginResponseReaderCall(t, registry, "node-b")
		if err := peer.Send("node-b", connectionCallRequest(call)); err != nil {
			t.Fatal(err)
		}
		if result := awaitPeerCall(t, call); result.err != nil {
			t.Fatal(result.err)
		}
		select {
		case failure := <-peer.diagnostics:
			if !errors.Is(failure.err, io.EOF) {
				t.Fatal(failure.err)
			}
		case <-time.After(time.Second):
			t.Fatal("disconnect was not observed")
		}
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if accepts.Load() != 2 {
		t.Fatalf("accepted %d connections", accepts.Load())
	}
}

// 验证半写失败不重拨或重放当前请求，下一条请求才允许发起新的 Dial。
func TestOutboundPeerPartialWriteNoReplay(t *testing.T) {
	var attempts atomic.Int32
	var writes atomic.Int32
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close() })
	peer, registry := newTestOutboundPeer(t, func(context.Context, string, string) (net.Conn, error) {
		if attempts.Add(1) == 1 {
			return &partialPeerConn{Conn: client, writes: &writes}, nil
		}
		return nil, io.EOF
	})
	call := beginResponseReaderCall(t, registry, "node-b")
	if err := peer.Send("node-b", connectionCallRequest(call)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatal(err)
	}
	awaitPeerCall(t, call)
	// 等待一个短暂空闲窗口，确认没有后台拨号任务。
	select {
	case <-time.After(20 * time.Millisecond):
	}
	if attempts.Load() != 1 || writes.Load() != 1 {
		t.Fatal("uncertain request was retried")
	}
	call = beginResponseReaderCall(t, registry, "node-b")
	if err := peer.Send("node-b", connectionCallRequest(call)); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	if attempts.Load() != 2 {
		t.Fatal("next request did not try to reconnect")
	}
}

// 验证拨号器 panic 转换为基础设施错误，当前 Call 不会遗留到超时。
func TestOutboundPeerDialPanic(t *testing.T) {
	peer, registry := newTestOutboundPeer(t, func(context.Context, string, string) (net.Conn, error) {
		panic("dial test")
	})
	call := beginResponseReaderCall(t, registry, "node-b")
	if err := peer.Send("node-b", connectionCallRequest(call)); !errors.Is(err, ErrTransportPanicked) {
		t.Fatal(err)
	}
	if result := awaitPeerCall(t, call); !errors.Is(result.err, ErrTransportPanicked) {
		t.Fatal(result.err)
	}
}

// 验证关闭解除正在阻塞的 socket 写入与响应读取，后续发送不会重启连接。
func TestOutboundPeerCloseDuringWrite(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close() })
	entered := make(chan struct{})
	peer, registry := newTestOutboundPeer(t, func(context.Context, string, string) (net.Conn, error) {
		return &signaledPeerConn{Conn: client, entered: entered}, nil
	})
	call := beginResponseReaderCall(t, registry, "node-b")
	done := make(chan error, 1)
	go func() { done <- peer.Send("node-b", connectionCallRequest(call)) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("write did not start")
	}
	if err := peer.close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("blocked write unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("write did not stop")
	}
	if result := awaitPeerCall(t, call); result.err == nil {
		t.Fatal("closed connection returned success")
	}
	if err := peer.Send("node-b", connectionCallRequest(call)); !errors.Is(err, ErrTransportClosed) {
		t.Fatal(err)
	}
}

type signaledPeerConn struct {
	net.Conn
	entered chan struct{}
}

// 验证响应读取 panic 被连接 owner 捕获、诊断并收尾，不使进程崩溃。
func TestOutboundPeerReaderPanic(t *testing.T) {
	server, client := net.Pipe()
	t.Cleanup(func() { server.Close() })
	peer, registry := newTestOutboundPeer(t, func(context.Context, string, string) (net.Conn, error) {
		return &panicReadPeerConn{Conn: client}, nil
	})
	call := beginResponseReaderCall(t, registry, "node-b")
	if err := peer.Send("node-b", connectionCallRequest(call)); err == nil {
		t.Fatal("write to failed reader unexpectedly succeeded")
	}
	if result := awaitPeerCall(t, call); result.err == nil {
		t.Fatal("reader panic did not finish call")
	}
	select {
	case failure := <-peer.diagnostics:
		if !errors.Is(failure.err, ErrTransportPanicked) {
			t.Fatal(failure.err)
		}
	case <-time.After(time.Second):
		t.Fatal("reader panic was not reported")
	}
}

type panicReadPeerConn struct {
	net.Conn
}

func (socket *panicReadPeerConn) Read(_ []byte) (int, error) {
	panic("reader test")
}

func (socket *signaledPeerConn) Write(frame []byte) (int, error) {
	close(socket.entered)
	return socket.Conn.Write(frame)
}

// 验证缺少目标、编解码器、等待表或有限超时的配置在启动前被拒绝。
func TestOutboundPeerInvalidConfig(t *testing.T) {
	codec, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	registry := NewClusterCallRegistry()
	defer registry.Close()
	valid := outboundPeerConfig{node: "node-b", address: "localhost:1234", codec: codec,
		calls: registry, dialTimeout: time.Second, writeTimeout: time.Second}
	for _, change := range []func(*outboundPeerConfig){
		func(c *outboundPeerConfig) { c.node = "" },
		func(c *outboundPeerConfig) { c.address = "" },
		func(c *outboundPeerConfig) { c.codec = nil },
		func(c *outboundPeerConfig) { c.calls = nil },
		func(c *outboundPeerConfig) { c.dialTimeout = 0 },
		func(c *outboundPeerConfig) { c.writeTimeout = -1 },
	} {
		config := valid
		change(&config)
		if _, err := newOutboundPeer(config); !errors.Is(err, ErrInvalidTransportConfig) {
			t.Fatal(err)
		}
	}
}

type partialPeerConn struct {
	net.Conn
	writes *atomic.Int32
}

func (socket *partialPeerConn) Write(frame []byte) (int, error) {
	socket.writes.Add(1)
	return len(frame) / 2, io.ErrUnexpectedEOF
}

// 验证 Send 入队成功后拨号失败只进入 A 侧诊断，sender 可以继续接纳消息。
func TestOutboundPeerSendFailureDiagnostic(t *testing.T) {
	peer, registry := newTestOutboundPeer(t, func(context.Context, string, string) (net.Conn, error) {
		return nil, io.ErrUnexpectedEOF
	})
	sender, err := NewNodeSender(NodeSenderConfig{RemoteNode: "node-b", Transport: peer})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := sender.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	call := beginResponseReaderCall(t, registry, "node-b")
	request := connectionCallRequest(call)
	request.Session = 0
	registry.cancel(call.session, context.Canceled)
	for i := 0; i < 2; i++ {
		if err := sender.Send(request); err != nil {
			t.Fatal(err)
		}
		select {
		case failure := <-sender.Failures():
			if failure.RemoteNode != "node-b" || !errors.Is(failure.Err, io.ErrUnexpectedEOF) {
				t.Fatalf("failure = %+v", failure)
			}
		case <-time.After(time.Second):
			t.Fatal("missing delivery failure")
		}
	}
}
