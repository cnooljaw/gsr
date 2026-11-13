package protocol

import (
	"errors"
	"testing"

	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

type closingSender struct {
	recordedSender
	last   int
	closed int
}

func (sender *closingSender) SendAndClose(frame []byte) error {
	sender.last++
	return sender.Send(frame)
}

func (sender *closingSender) Close() error {
	sender.closed++
	return nil
}

// 验证策略在自动回错前得到当前上下文与原始错误，只生成唯一安全末帧并保留编号。
func TestErrorPolicyTerminalResponse(t *testing.T) {
	sender := &closingSender{}
	cause := errors.New("private SQL token")
	calls := 0
	dispatcher, err := NewDispatcher[string](testRegistry(t), sender, map[uint32]any{1001: func(string, Context, *echopb.EchoRequest) error { return cause }}, WithErrorPolicy(func(scope string, id uint32, err error) ErrorAction {
		calls++
		if scope != "owner" || id != 1001 || !errors.Is(err, cause) {
			t.Fatal(scope, id, err)
		}
		return ReplyErrorAndClose
	}))
	if err != nil {
		t.Fatal(err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 1001, RequestId: 77})
	if err := dispatcher.HandleFrame("owner", frame); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	if calls != 1 || sender.last != 1 || len(sender.envelopes) != 1 || sender.envelopes[0].GetRequestId() != 77 {
		t.Fatal(calls, sender)
	}
	var failure commonv1.ErrorResponse
	if err := proto.Unmarshal(sender.envelopes[0].Payload, &failure); err != nil {
		t.Fatal(err)
	}
	if failure.Message != "request could not be processed" {
		t.Fatal(failure.Message)
	}
	sender.envelopes = nil
	sender.sendErr = errors.New("queue full")
	err = dispatcher.HandleFrame("owner", frame)
	if !errors.Is(err, cause) || !errors.Is(err, sender.sendErr) || len(sender.envelopes) != 1 || sender.last != 2 {
		t.Fatal(err, sender)
	}
}

// 验证无法关联的错误只关闭、已有响应不追加策略，配置策略但缺少关闭能力在构造时拒绝。
func TestErrorPolicyBoundaries(t *testing.T) {
	policy := WithErrorPolicy(func(_ string, _ uint32, _ error) ErrorAction { return ReplyErrorAndClose })
	if _, err := NewDispatcher[string](testRegistry(t), &recordedSender{}, nil, policy); !errors.Is(err, ErrCloseUnsupported) {
		t.Fatal(err)
	}
	for _, frame := range [][]byte{{0xff}, marshalEnvelope(t, &commonv1.Envelope{MsgId: 7777}), marshalEnvelope(t, &commonv1.Envelope{MsgId: 1003}), marshalEnvelope(t, &commonv1.Envelope{MsgId: 1003, RequestId: 99})} {
		sender := &closingSender{}
		dispatcher, err := NewDispatcher[string](testRegistry(t), sender, nil, policy)
		if err != nil {
			t.Fatal(err)
		}
		if err := dispatcher.HandleFrame("", frame); err == nil {
			t.Fatal("missing error")
		}
		if sender.closed != 1 || sender.last != 0 || len(sender.envelopes) != 0 {
			t.Fatal(sender)
		}
	}
	sender := &closingSender{}
	calls := 0
	dispatcher, err := NewDispatcher[string](testRegistry(t), sender, map[uint32]any{1001: func(_ string, ctx Context, _ *echopb.EchoRequest) error {
		if err := ctx.Response(1002, &echopb.EchoResponse{}); err != nil {
			return err
		}
		return errors.New("after response")
	}}, WithErrorPolicy(func(_ string, _ uint32, _ error) ErrorAction { calls++; return ReplyErrorAndClose }))
	if err != nil {
		t.Fatal(err)
	}
	dispatcher.HandleFrame("", marshalEnvelope(t, &commonv1.Envelope{MsgId: 1001, RequestId: 9}))
	if calls != 0 || sender.last != 0 || len(sender.envelopes) != 1 {
		t.Fatal(calls, sender)
	}
}
