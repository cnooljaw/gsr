package cluster_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/cluster"
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
			link := newRoundTripLink(t, func(_ context.Context, from cluster.NodeID, request cluster.ClusterEnvelope) ([]byte, error) {
				if from != "node-a" || request.Target.ServiceName != "room" || request.Command != roundTripCommand {
					t.Errorf("unexpected request route: from=%s request=%+v", from, request)
				}
				if testCase.panics {
					panic("private panic detail")
				}
				return request.Payload, testCase.err
			})
			result, err := link.client.Call(context.Background(), "node-b", cluster.ServiceTarget{ServiceName: "room"}, roundTripCommand, "hello", int32(3))
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
	link := newRoundTripLink(t, func(_ context.Context, _ cluster.NodeID, request cluster.ClusterEnvelope) ([]byte, error) {
		if request.Session == 0 {
			remoteErrors <- remoteErr
			return nil, remoteErr
		}
		return request.Payload, nil
	})
	target := cluster.ServiceTarget{ServiceName: "room"}
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
	link := newRoundTripLink(t, func(_ context.Context, _ cluster.NodeID, request cluster.ClusterEnvelope) ([]byte, error) {
		return request.Payload, nil
	})
	if err := link.transport.Unregister("node-b"); err != nil {
		t.Fatal(err)
	}
	target := cluster.ServiceTarget{ServiceName: "room"}
	if err := link.client.Send("node-b", target, roundTripCommand); err != nil {
		t.Fatalf("Send admission: %v", err)
	}
	select {
	case failure := <-link.sender.Failures():
		if failure.LocalNode != "node-a" || failure.RemoteNode != "node-b" || failure.Target != target ||
			failure.Command != roundTripCommand || failure.Session != 0 || !errors.Is(failure.Err, cluster.ErrNodeNotFound) {
			t.Fatalf("unexpected transport failure: %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("missing node-a transport failure")
	}
}

type roundTripLink struct {
	client    *cluster.ClusterClient
	calls     *cluster.ClusterCallRegistry
	sender    *cluster.NodeSender
	transport *cluster.InMemoryTransport
}

type roundTripResolver struct {
	sender *cluster.NodeSender
}

func (resolver roundTripResolver) ResolveNodeSender(node cluster.NodeID) (cluster.OutboundSender, error) {
	if node != "node-b" {
		return nil, cluster.ErrNodeNotFound
	}
	return resolver.sender, nil
}

// 仅组装公开 API；完整帧往返模拟连接边界，不模拟 TCP 调度或 Core Service 执行。
func newRoundTripLink(t *testing.T, handler cluster.RequestHandler) *roundTripLink {
	t.Helper()
	transport, err := cluster.NewInMemoryTransport(cluster.InMemoryTransportConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := transport.Close(); err != nil {
			t.Error(err)
		}
	})
	frames, err := cluster.NewFrameCodec(cluster.FrameCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	wireCopy := func(envelope cluster.ClusterEnvelope) (cluster.ClusterEnvelope, error) {
		frame, err := frames.Encode(envelope)
		if err != nil {
			return cluster.ClusterEnvelope{}, err
		}
		return frames.Decode(frame)
	}
	calls := cluster.NewClusterCallRegistry()
	t.Cleanup(func() {
		if err := calls.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := transport.Register("node-a", func(from cluster.NodeID, response cluster.ClusterEnvelope) {
		decoded, err := wireCopy(response)
		if err == nil {
			err = calls.Complete(from, decoded)
		}
		if err != nil {
			t.Errorf("node-a receive: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	agent, err := cluster.NewConnectionAgent(cluster.ConnectionAgentConfig{
		Handler: handler,
		Write: func(response cluster.ClusterEnvelope) error {
			return transport.Send("node-b", "node-a", response)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := transport.Register("node-b", func(from cluster.NodeID, request cluster.ClusterEnvelope) {
		decoded, err := wireCopy(request)
		if err == nil {
			err = agent.Handle(context.Background(), from, decoded)
		}
		if err != nil {
			t.Errorf("node-b receive: %v", err)
		}
	}); err != nil {
		t.Fatal(err)
	}
	sender, err := cluster.NewNodeSender(cluster.NodeSenderConfig{LocalNode: "node-a", RemoteNode: "node-b", Transport: transport})
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
	values, err := cluster.NewValueCodec(cluster.ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	client, err := cluster.NewClusterClient(cluster.ClusterClientConfig{
		Source:       11,
		Senders:      roundTripResolver{sender: sender},
		Codecs:       cluster.MustCommandCodecRegistry([]cluster.CommandCodecEntry{{CommandID: roundTripCommand, Codec: values}}),
		PendingCalls: calls,
		MaxCallWait:  time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &roundTripLink{client: client, calls: calls, sender: sender, transport: transport}
}
