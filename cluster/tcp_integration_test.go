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

// 验证真实 TCP 连接上的 Send/Call 经本地 Runtime 执行，并保留结构化业务错误身份。
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
	owner, err := newInboundConnection(inbound, "node-a", adapter, 2)
	if err != nil {
		t.Fatal(err)
	}
	calls := NewClusterCallRegistry()
	diagnostics := make(chan responseReadFailure, 16)
	ctx, cancel := context.WithCancel(context.Background())
	inboundDone := make(chan error, 1)
	responseDone := make(chan error, 1)
	go func() { inboundDone <- owner.serve(ctx) }()
	go func() {
		_, err := outbound.receiveResponses("node-b", calls, diagnostics)
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
	sender, err := NewNodeSender(NodeSenderConfig{LocalNode: "node-a", RemoteNode: "node-b", Transport: &tcpTestTransport{connection: outbound}})
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
	connection *streamConnection
}

func (*tcpTestTransport) Register(NodeID, Receiver) error {
	return ErrInvalidNode
}

func (*tcpTestTransport) Unregister(NodeID) error {
	return ErrInvalidNode
}

func (transport *tcpTestTransport) Send(from, to NodeID, request ClusterEnvelope) error {
	if from != "node-a" || to != "node-b" {
		return ErrInvalidNode
	}
	return transport.connection.write(request)
}

func (transport *tcpTestTransport) Close() error {
	return transport.connection.close()
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
