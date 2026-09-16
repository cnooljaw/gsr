package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证合法载荷的未知 Command 由目标判断，Send 无回包，Call 及时返回未知命令而非被 Cluster 丢弃。
func TestLocalAdapterUnknownCommandBelongsToService(t *testing.T) {
	adapter, _, codec := newLocalAdapterTest(t)
	id, err := adapter.runtime.CreateService(context.Background(), &scopedCommandService{value: "one"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := codec.Encode([]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []ClusterSessionID{0, 1} {
		var replies []ClusterEnvelope
		err := adapter.handle(context.Background(), ClusterEnvelope{Kind: FrameRequest, Session: session,
			Target: ServiceTarget{ServiceID: id}, Command: 999, Payload: payload}, func(reply ClusterEnvelope) error {
			replies = append(replies, reply)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if session == 0 {
			if len(replies) != 0 {
				t.Fatal("Send received reply")
			}
		} else if len(replies) != 1 || !errors.Is(replies[0].Error, runtime.ErrUnknownCommand) {
			t.Fatalf("unknown Call replies: %+v", replies)
		}
	}
}

// 验证两个目标可复用同一未在 Cluster 注册的编号，分别执行各自的业务语义。
func TestLocalAdapterCommandIDIsServiceLocal(t *testing.T) {
	adapter, _, codec := newLocalAdapterTest(t)
	payload, err := codec.Encode([]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"one", "two"} {
		id, err := adapter.runtime.CreateService(context.Background(), &scopedCommandService{value: expected})
		if err != nil {
			t.Fatal(err)
		}
		var result any
		err = adapter.handle(context.Background(), ClusterEnvelope{Kind: FrameRequest, Session: 2,
			Target: ServiceTarget{ServiceID: id}, Command: 1, Payload: payload}, func(reply ClusterEnvelope) error {
			if reply.Kind != FrameResponse {
				t.Fatalf("reply: %+v", reply)
			}
			var err error
			result, err = codec.Decode(reply.Payload)
			return err
		})
		if err != nil || result != expected {
			t.Fatalf("result=%v error=%v", result, err)
		}
	}
}

type scopedCommandService struct{ value string }

// 验证默认编码无需命令清单，真实 TCP 上未知请求不会破坏后续已知命令调用。
func TestClusterDefaultCodecCommandScopeOverTCP(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	server, err := NewCluster(ClusterConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestCluster(t, server) })
	address, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := NewCluster(ClusterConfig{Nodes: map[NodeID]string{"target": address}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestCluster(t, caller) })
	client, err := caller.Client(7)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, expected := range []string{"one", "two"} {
		id, err := adapter.runtime.CreateService(ctx, &scopedCommandService{value: expected})
		if err != nil {
			t.Fatal(err)
		}
		target := ServiceTarget{ServiceID: id}
		if err := client.Send("target", target, 999); err != nil {
			t.Fatal(err)
		}
		if _, err := client.Call(ctx, "target", target, 999); !errors.Is(err, runtime.ErrUnknownCommand) {
			t.Fatalf("unknown Call: %v", err)
		}
		value, err := client.Call(ctx, "target", target, 1)
		if err != nil || value != expected {
			t.Fatalf("value=%v error=%v", value, err)
		}
	}
}

func (service *scopedCommandService) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	return nil
}

func (service *scopedCommandService) Handle(_ runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != 1 {
		return nil, runtime.ErrUnknownCommand
	}
	return service.value, nil
}

func (service *scopedCommandService) OnStop(_ context.Context) error {
	return nil
}
