package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证没有本地 NodeID 或监听的 TCP 客户端可 Send/Call，并沿原连接收到结果和领域错误。
func TestTCPClusterRoundTrip(t *testing.T) {
	link := newTCPClusterTestLink(t)
	target := ServiceTarget{ServiceID: link.target}
	if err := link.client.Send("node-b", target, clusterTestCommand, "send"); err != nil {
		t.Fatal(err)
	}
	value, err := link.client.Call(context.Background(), "node-b", target, clusterTestCommand, "hello", int32(3))
	if err != nil || !reflect.DeepEqual(value, []any{"hello", int64(3)}) {
		t.Fatalf("TCP Call = %#v, %v", value, err)
	}
	domainErr := runtime.NewStatusError(runtime.ErrorClassDomain, "room.full", "room is full")
	id, err := link.adapter.runtime.CreateService(context.Background(), &tcpErrorService{err: domainErr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := link.client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand); !errors.Is(err, domainErr) {
		t.Fatalf("TCP domain error = %v", err)
	}
	if link.calls.PendingCount() != 0 {
		t.Fatalf("pending count = %d", link.calls.PendingCount())
	}
	select {
	case failure := <-link.diagnostics:
		t.Fatalf("unexpected response diagnostic: %+v", failure)
	default:
	}
}

// 验证真实 TCP 对端关闭直接恢复等待中的 Call，不必等到 MaxCallWait。
func TestTCPClusterDisconnectCompletesCall(t *testing.T) {
	link := newTCPClusterTestLink(t)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	defer close(blocked.release)
	id, err := link.adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	outcome := make(chan error, 1)
	go func() {
		defer close(done)
		_, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "block")
		outcome <- err
	}()
	defer func() { cancel(); <-done }()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("remote Handler did not start")
	}
	if err := link.connection(t).close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-outcome:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("disconnected Call = %v, want EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Call did not resume after peer close")
	}
	if link.calls.PendingCount() != 0 {
		t.Fatalf("pending count = %d", link.calls.PendingCount())
	}
}

// 验证慢 Call 断线失败后，新 Call 通过按需重连执行，旧请求不会重放。
func TestTCPClusterReconnectAfterInterruptedCall(t *testing.T) {
	link := newTCPClusterTestLink(t)
	blocked := &tcpCountingService{inboundRecordingService: &inboundRecordingService{
		entered: make(chan struct{}), release: make(chan struct{}),
	}}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blocked.release) }) }
	defer release()
	id, err := link.adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	outcome := make(chan error, 1)
	go func() {
		_, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "block")
		outcome <- err
	}()
	select {
	case <-blocked.entered:
	case <-ctx.Done():
		t.Fatal("remote Handler did not start")
	}
	old := link.connection(t)
	if err := old.close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-outcome:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("interrupted Call = %v", err)
		}
	case <-ctx.Done():
		t.Fatal("interrupted Call did not finish")
	}
	value, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "reconnected")
	if err != nil || !reflect.DeepEqual(value, []any{"reconnected"}) {
		t.Fatalf("new Call = %#v, %v", value, err)
	}
	if link.connection(t) == old {
		t.Fatal("new request reused closed connection")
	}
	release()
	// 用同一 Service 的后续消息作为串行屏障，确认原 Handler 已完成且没有重复执行。
	if _, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "check"); err != nil {
		t.Fatal(err)
	}
	if blocked.invocations.Load() != 2 {
		t.Fatalf("Handler executions = %d, want original plus check", blocked.invocations.Load())
	}
	if link.calls.PendingCount() != 0 {
		t.Fatal("pending Call leaked after reconnect")
	}
}

// 验证真实 TCP 上同连接的慢 Call 不妨碍其它目标先完成，回复按各自 Session 匹配。
func TestTCPClusterSlowCallDoesNotBlockOtherService(t *testing.T) {
	link := newTCPClusterTestLink(t)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blocked.release) }) }
	defer release()
	id, err := link.adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	outcome := make(chan error, 1)
	go func() {
		defer close(done)
		_, err := link.client.Call(ctx, "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "block")
		outcome <- err
	}()
	defer func() { cancel(); release(); <-done }()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("slow Handler did not start")
	}
	fastCtx, fastCancel := context.WithTimeout(context.Background(), time.Second)
	defer fastCancel()
	value, err := link.client.Call(fastCtx, "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "fast")
	if err != nil || !reflect.DeepEqual(value, []any{"fast"}) {
		t.Fatalf("fast Call = %#v, %v", value, err)
	}
	release()
	select {
	case err := <-outcome:
		if err != nil {
			t.Fatalf("slow Call = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("slow Call did not finish after release")
	}
}

// 验证两个方向各自拥有 TCP 链路；关闭 A 到 B 不会关闭 B 到 A 的主动调用路径。
func TestTCPClusterDirectionalConnectionsAreIndependent(t *testing.T) {
	toB := newTCPClusterTestLinkTo(t, "node-b")
	toA := newTCPClusterTestLinkTo(t, "node-a")
	for _, call := range []struct {
		link *tcpClusterTestLink
		node NodeID
	}{
		{link: toB, node: "node-b"},
		{link: toA, node: "node-a"},
	} {
		value, err := call.link.client.Call(context.Background(), call.node, ServiceTarget{ServiceID: call.link.target}, clusterTestCommand, string(call.node))
		if err != nil || !reflect.DeepEqual(value, []any{string(call.node)}) {
			t.Fatalf("Call(%s) = %#v, %v", call.node, value, err)
		}
	}
	if err := toB.connection(t).close(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := toA.client.Call(ctx, "node-a", ServiceTarget{ServiceID: toA.target}, clusterTestCommand, "still open")
	if err != nil || !reflect.DeepEqual(value, []any{"still open"}) {
		t.Fatalf("opposite direction after close = %#v, %v", value, err)
	}
}

type tcpClusterTestLink struct {
	client      *ClusterClient
	calls       *ClusterCallRegistry
	adapter     *localRequestAdapter
	target      runtime.ServiceID
	listener    *tcpListener
	diagnostics <-chan responseReadFailure
}

// connection 只用于故障注入，业务验证仍通过 ClusterClient 进行。
func (link *tcpClusterTestLink) connection(t *testing.T) *streamConnection {
	t.Helper()
	link.listener.mu.Lock()
	defer link.listener.mu.Unlock()
	for connection := range link.listener.connections {
		if !connection.closed.Load() {
			return connection
		}
	}
	t.Fatal("no live inbound connection")
	return nil
}

func newTCPClusterTestLink(t *testing.T) *tcpClusterTestLink {
	t.Helper()
	return newTCPClusterTestLinkTo(t, "node-b")
}

// 测试仅提供静态目标和生命周期组装，拨号、接纳、回复与恢复使用真实内部 owner。
func newTCPClusterTestLinkTo(t *testing.T, remoteNode NodeID) *tcpClusterTestLink {
	t.Helper()
	adapter, target, _ := newLocalAdapterTest(t)
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	frames, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	inbound, err := newTCPListener(tcpListenerConfig{
		listener: listener, codec: frames, adapter: adapter,
		maxConnections: 4, maxCalls: 2, writeTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := NewClusterCallRegistry()
	t.Cleanup(func() { calls.Close() })
	outbound, err := newOutboundPeer(outboundPeerConfig{
		node: remoteNode, address: listener.Addr().String(), codec: frames, calls: calls,
		dialTimeout: time.Second, writeTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { outbound.close() })
	ctx, cancel := context.WithCancel(context.Background())
	inboundDone := make(chan error, 1)
	go func() { inboundDone <- inbound.serve(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-inboundDone:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("listener exit: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("listener did not stop")
		}
	})
	sender, err := NewNodeSender(NodeSenderConfig{RemoteNode: remoteNode, Transport: outbound})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		outbound.close()
		closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
		defer closeCancel()
		if err := sender.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})
	client, err := NewClusterClient(ClusterClientConfig{
		Source: 11, Senders: clientSenderResolver{sender: sender}, Codec: adapter.codec,
		PendingCalls: calls, MaxCallWait: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &tcpClusterTestLink{
		client: client, calls: calls, adapter: adapter, target: target,
		listener: inbound, diagnostics: outbound.diagnostics,
	}
}

type tcpErrorService struct {
	err error
}

type tcpCountingService struct {
	*inboundRecordingService
	invocations atomic.Int32
}

func (service *tcpCountingService) Handle(ctx runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	service.invocations.Add(1)
	return service.inboundRecordingService.Handle(ctx, command, args...)
}

func (*tcpErrorService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *tcpErrorService) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, service.err
}

func (*tcpErrorService) OnStop(context.Context) error {
	return nil
}
