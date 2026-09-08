package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"sync"
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
	if err := link.server.close(); err != nil {
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
	if toB.server.socket == toA.server.socket {
		t.Fatal("opposite directions share a socket")
	}
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
	if err := toB.server.close(); err != nil {
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
	server      *streamConnection
	diagnostics chan responseReadFailure
}

func newTCPClusterTestLink(t *testing.T) *tcpClusterTestLink {
	t.Helper()
	return newTCPClusterTestLinkTo(t, "node-b")
}

// 每条测试链路只配置出站目标，不注册调用方；接收端 writer 绑定同一 socket。
func newTCPClusterTestLinkTo(t *testing.T, remoteNode NodeID) *tcpClusterTestLink {
	t.Helper()
	adapter, target, _ := newLocalAdapterTest(t)
	listener, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	if err := listener.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	socket, err := net.DialTimeout("tcp4", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { socket.Close() })
	accepted, err := listener.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { accepted.Close() })
	frames, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	outbound, err := newStreamConnection(socket, frames, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	inbound, err := newStreamConnection(accepted, frames, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := newInboundConnection(inbound, adapter, 2, newInboundDiagnostics())
	if err != nil {
		t.Fatal(err)
	}
	calls := NewClusterCallRegistry()
	sending, err := newOutboundConnection(outbound, calls, remoteNode)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics := make(chan responseReadFailure, 16)
	ctx, cancel := context.WithCancel(context.Background())
	inboundDone := make(chan error, 1)
	responseDone := make(chan error, 1)
	go func() {
		err := owner.serve(ctx)
		close(owner.diagnostics.failures)
		inboundDone <- err
	}()
	go func() {
		_, err := outbound.receiveResponses(sending.calls, diagnostics)
		responseDone <- err
	}()
	t.Cleanup(func() {
		cancel()
		outbound.close()
		inbound.close()
		for _, done := range []<-chan error{inboundDone, responseDone} {
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
					t.Errorf("TCP loop exit: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Error("TCP loop did not exit")
			}
		}
		if err := calls.Close(); err != nil {
			t.Error(err)
		}
	})
	sender, err := NewNodeSender(NodeSenderConfig{RemoteNode: remoteNode, Transport: &tcpTestTransport{connection: sending, target: remoteNode}})
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
	client, err := NewClusterClient(ClusterClientConfig{Source: 11, Senders: clientSenderResolver{sender: sender}, Codecs: adapter.codecs, PendingCalls: calls, MaxCallWait: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	return &tcpClusterTestLink{client: client, calls: calls, adapter: adapter, target: target, server: inbound, diagnostics: diagnostics}
}

// tcpTestTransport 只连接测试已建立的单条 TCP 连接，不提供生产建连、认证或重连能力。
type tcpTestTransport struct {
	connection *outboundConnection
	target     NodeID
}

func (transport *tcpTestTransport) Send(to NodeID, request ClusterEnvelope) error {
	if to != transport.target {
		return ErrInvalidNode
	}
	return transport.connection.write(request)
}

type tcpErrorService struct {
	err error
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
