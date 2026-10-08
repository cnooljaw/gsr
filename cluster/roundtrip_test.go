package cluster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

const roundTripCommand runtime.CommandID = 0x04100306

// 验证双节点 Call 经完整帧编码往返后保留返回值、业务错误身份，并隐藏未知错误和 panic 细节。
func TestClusterRoundTripCall(t *testing.T) {
	domainErr := runtime.NewStatusError(runtime.ErrorClassDomain, "room.full", "room is full")
	cases := []struct {
		name    string
		err     error
		panics  bool
		wantErr error
	}{
		{name: "success"},
		{name: "domain", err: fmt.Errorf("local detail: %w", domainErr), wantErr: domainErr},
		{name: "unknown", err: errors.New("private database detail"), wantErr: runtime.ErrHandlerFailed},
		{name: "panic", panics: true, wantErr: runtime.ErrServicePanicked},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			link := newRoundTripLink(t, func(_ runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
				if command != roundTripCommand {
					t.Errorf("unexpected command: %d", command)
				}
				if testCase.panics {
					panic("private panic detail")
				}
				return args, testCase.err
			})
			result, err := link.client.Call(context.Background(), "node-b", ServiceTarget{ServiceName: "room"}, roundTripCommand, "hello", int32(3))
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Call error = %v, want %v", err, testCase.wantErr)
			}
			if testCase.wantErr == nil {
				if !reflect.DeepEqual(result, []any{"hello", int64(3)}) {
					t.Fatalf("Call result = %#v", result)
				}
			} else {
				var status *runtime.StatusError
				var expected *runtime.StatusError
				if !errors.As(err, &status) || !errors.As(testCase.wantErr, &expected) || status.Message != expected.Message {
					t.Fatalf("wire error does not preserve safe status: %v", err)
				}
			}
			if count := link.calls.PendingCount(); count != 0 {
				t.Fatalf("PendingCount = %d, want 0", count)
			}
		})
	}
}

// 验证 Send 的远端业务失败由 B 记录且不回包，随后同一链路仍能完成 Call。
func TestClusterRoundTripSendFailureStaysRemote(t *testing.T) {
	remoteErrors := make(chan error, 1)
	remoteErr := errors.New("node-b handler failed")
	link := newRoundTripLink(t, func(_ runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
		if len(args) > 0 && args[0] == "send" {
			remoteErrors <- remoteErr
			return nil, remoteErr
		}
		return args, nil
	})
	target := ServiceTarget{ServiceName: "room"}
	if err := link.client.Send("node-b", target, roundTripCommand, "send"); err != nil {
		t.Fatalf("Send admission: %v", err)
	}
	// 同一 NodeSender 串行投递；Call 完成也确认前面的 Send 已处理结束。
	if _, err := link.client.Call(context.Background(), "node-b", target, roundTripCommand, "call"); err != nil {
		t.Fatalf("Call after Send: %v", err)
	}
	select {
	case err := <-remoteErrors:
		if !errors.Is(err, remoteErr) {
			t.Fatalf("remote record = %v", err)
		}
	default:
		t.Fatal("missing node-b error record")
	}
	select {
	case failure := <-link.sender.Failures():
		t.Fatalf("remote handler error reached node-a transport diagnostics: %+v", failure)
	default:
	}
}

// 验证目标节点未注册时 Send 仍只表示入队成功，后续失败记录包含节点、命令和底层原因。
func TestClusterRoundTripSendUnavailableNode(t *testing.T) {
	link := newRoundTripLink(t, func(_ runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
		return args, nil
	})
	if err := link.transport.Unregister("node-b"); err != nil {
		t.Fatal(err)
	}
	target := ServiceTarget{ServiceName: "room"}
	if err := link.client.Send("node-b", target, roundTripCommand); err != nil {
		t.Fatalf("Send admission: %v", err)
	}
	select {
	case failure := <-link.sender.Failures():
		if failure.RemoteNode != "node-b" || failure.Target != target ||
			failure.Command != roundTripCommand || failure.Session != 0 || !errors.Is(failure.Err, ErrNodeNotFound) {
			t.Fatalf("unexpected transport failure: %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("missing node-a transport failure")
	}
}

type roundTripLink struct {
	local     *runtime.Runtime
	serviceID runtime.ServiceID
	client    *ClusterClient
	calls     *ClusterCallRegistry
	sender    *NodeSender
	transport *InMemoryTransport
}

type roundTripResolver struct {
	sender *NodeSender
}

func (resolver roundTripResolver) ResolveNodeSender(node NodeID) (OutboundSender, error) {
	if node != "node-b" {
		return nil, ErrNodeNotFound
	}
	return resolver.sender, nil
}

// 帧往返模拟连接边界；入站复用生产 localRequestAdapter 和真实 Core Service。
func newRoundTripLink(t *testing.T, handler func(runtime.CommandContext, runtime.CommandID, ...any) (any, error)) *roundTripLink {
	t.Helper()
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := transport.Close(); err != nil {
			t.Error(err)
		}
	})
	frames, err := NewFrameCodec(FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	wireCopy := func(envelope ClusterEnvelope) (ClusterEnvelope, error) {
		frame, err := frames.Encode(envelope)
		if err != nil {
			return ClusterEnvelope{}, err
		}
		return frames.Decode(frame)
	}
	calls := NewClusterCallRegistry()
	t.Cleanup(func() {
		if err := calls.Close(); err != nil {
			t.Error(err)
		}
	})
	values, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { local.Close() })
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := local.CreateService(context.Background(), &roundTripService{handler: handler})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &localRequestAdapter{
		runtime: local,
		codec:   values,
		resolve: func(name ServiceName) (runtime.ServiceID, error) {
			if name == "room" {
				return id, nil
			}
			return 0, runtime.ErrServiceNotFound
		},
	}
	// 回复绑定当前链路，不注册调用方，也不通过反向 Send 路由。
	write := func(response ClusterEnvelope) error {
		decoded, err := wireCopy(response)
		if err != nil {
			return err
		}
		return calls.Complete("node-b", decoded)
	}
	if err := transport.Register("node-b", func(request ClusterEnvelope) {
		decoded, err := wireCopy(request)
		if err == nil {
			err = adapter.handle(context.Background(), decoded, write)
		}
		if err != nil {
			t.Errorf("node-b receive: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{RemoteNode: "node-b", Transport: transport})
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
	client, err := NewClusterClient(ClusterClientConfig{
		Source:       11,
		Senders:      roundTripResolver{sender: sender},
		Codec:        values,
		PendingCalls: calls,
		MaxCallWait:  time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &roundTripLink{local: local, serviceID: id, client: client, calls: calls, sender: sender, transport: transport}
}

// roundTripService 在真实 runner 上执行测试业务，panic containment 归 Runtime。
type roundTripService struct {
	handler func(runtime.CommandContext, runtime.CommandID, ...any) (any, error)
}

func (*roundTripService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *roundTripService) Handle(self runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if self.Source() != 0 {
		return nil, errors.New("unexpected inbound source")
	}
	return service.handler(self, command, args...)
}

func (*roundTripService) OnStop(context.Context) error {
	return nil
}
