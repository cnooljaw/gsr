package cluster

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证入站适配器按 ID 或外层名称解析调用真实 Service，Source 为零且沿原 Session 回复。
func TestLocalAdapterCallsRuntime(t *testing.T) {
	adapter, id, codec := newLocalAdapterTest(t)
	for _, target := range []ServiceTarget{{ServiceID: id}, {ServiceName: "room"}} {
		payload, err := codec.Encode([]any{"hello", int64(3)})
		if err != nil {
			t.Fatal(err)
		}
		var replies []ClusterEnvelope
		err = adapter.handle(context.Background(), ClusterEnvelope{Kind: FrameRequest, Session: 99, Target: target, Command: clusterTestCommand, Payload: payload}, func(reply ClusterEnvelope) error {
			replies = append(replies, reply)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(replies) != 1 {
			t.Fatalf("reply count = %d", len(replies))
		}
		reply := replies[0]
		if reply.Kind != FrameResponse || reply.Session != 99 || reply.Target != target || reply.Command != clusterTestCommand {
			t.Fatalf("reply = %+v", reply)
		}
		value, err := codec.Decode(reply.Payload)
		if err != nil || !reflect.DeepEqual(value, []any{"hello", int64(3)}) {
			t.Fatalf("result = %#v, %v", value, err)
		}
	}
}

// 验证未知命令、坏载荷和非参数列表在调用 Service 前被拒绝，不制造响应帧。
func TestLocalAdapterDropsInvalidPayload(t *testing.T) {
	adapter, id, codec := newLocalAdapterTest(t)
	scalar, err := codec.Encode("not an argument list")
	if err != nil {
		t.Fatal(err)
	}
	for _, testCase := range []struct {
		command runtime.CommandID
		payload []byte
		want    error
	}{
		{command: clusterTestCommand + 1, want: ErrCommandCodecNotFound},
		{command: clusterTestCommand, payload: []byte{255}, want: ErrMalformedPayload},
		{command: clusterTestCommand, payload: scalar, want: runtime.ErrInvalidCommandArguments},
	} {
		writes := 0
		err := adapter.handle(context.Background(), ClusterEnvelope{Kind: FrameRequest, Session: 1, Target: ServiceTarget{ServiceID: id}, Command: testCase.command, Payload: testCase.payload}, func(ClusterEnvelope) error { writes++; return nil })
		if !errors.Is(err, testCase.want) || writes != 0 {
			t.Fatalf("error = %v, writes = %d", err, writes)
		}
	}
}

// 验证目标解析失败时 Call 返回结构化错误，Send 只返回本地诊断错误且不回包。
func TestLocalAdapterTargetFailureOwnership(t *testing.T) {
	adapter, _, codec := newLocalAdapterTest(t)
	payload, err := codec.Encode([]any{})
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []ClusterSessionID{0, 1} {
		var replies []ClusterEnvelope
		err := adapter.handle(context.Background(), ClusterEnvelope{Kind: FrameRequest, Session: session, Target: ServiceTarget{ServiceName: "missing"}, Command: clusterTestCommand, Payload: payload}, func(reply ClusterEnvelope) error { replies = append(replies, reply); return nil })
		if session == 0 {
			if !errors.Is(err, runtime.ErrServiceNotFound) || len(replies) != 0 {
				t.Fatalf("Send error=%v replies=%v", err, replies)
			}
			continue
		}
		if err != nil || len(replies) != 1 || replies[0].Kind != FrameError || !errors.Is(replies[0].Error, runtime.ErrServiceNotFound) {
			t.Fatalf("Call error=%v replies=%v", err, replies)
		}
	}
}

// 验证入站 Send 只做本地接纳、不回包，随后同一 Service 仍可完成 Call。
func TestLocalAdapterSendDoesNotReply(t *testing.T) {
	adapter, id, codec := newLocalAdapterTest(t)
	payload, err := codec.Encode([]any{"send"})
	if err != nil {
		t.Fatal(err)
	}
	writes := 0
	err = adapter.handle(context.Background(), ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload}, func(ClusterEnvelope) error {
		writes++
		return nil
	})
	if err != nil || writes != 0 {
		t.Fatalf("Send error=%v writes=%d", err, writes)
	}
	if _, err := adapter.runtime.Call(context.Background(), id, clusterTestCommand, "barrier"); err != nil {
		t.Fatalf("Call after Send: %v", err)
	}
}

// 验证写回失败保留原始 I/O 原因，已取消的请求不会写回响应。
func TestLocalAdapterReportsWriteFailureAndCancellation(t *testing.T) {
	adapter, id, codec := newLocalAdapterTest(t)
	payload, err := codec.Encode([]any{})
	if err != nil {
		t.Fatal(err)
	}
	request := ClusterEnvelope{Kind: FrameRequest, Session: 1, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload}
	writeErr := errors.New("connection closed")
	err = adapter.handle(context.Background(), request, func(ClusterEnvelope) error { return writeErr })
	if !errors.Is(err, ErrConnectionAgentWrite) || !errors.Is(err, writeErr) {
		t.Fatalf("write error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	writes := 0
	err = adapter.handle(ctx, request, func(ClusterEnvelope) error { writes++; return nil })
	if !errors.Is(err, context.Canceled) || writes != 0 {
		t.Fatalf("cancelled request error=%v writes=%d", err, writes)
	}
}

func newLocalAdapterTest(t *testing.T) (*localRequestAdapter, runtime.ServiceID, *ValueCodec) {
	t.Helper()
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := local.CreateService(context.Background(), &localAdapterService{})
	if err != nil {
		t.Fatal(err)
	}
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &localRequestAdapter{
		runtime: local,
		codecs:  MustCommandCodecRegistry([]CommandCodecEntry{{CommandID: clusterTestCommand, Codec: codec}}),
		resolve: func(name ServiceName) (runtime.ServiceID, error) {
			if name == "room" {
				return id, nil
			}
			return 0, runtime.ErrServiceNotFound
		},
	}
	return adapter, id, codec
}

type localAdapterService struct{}

func (*localAdapterService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (*localAdapterService) Handle(ctx runtime.CommandContext, _ runtime.CommandID, args ...any) (any, error) {
	if ctx.Source() != 0 {
		return nil, errors.New("unexpected source")
	}
	return args, nil
}

func (*localAdapterService) OnStop(context.Context) error {
	return nil
}
