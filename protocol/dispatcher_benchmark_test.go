package protocol

import (
	"testing"

	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

// BenchmarkDispatcherHandleFrame 测量小消息的协议解码、反射分发和请求写回成本。
// NotifyDecode 仅作解码基线，不是保持全部协议语义的手写热点实现。
// 不包含 Gate、MAC、Mailbox、socket I/O 或业务 Service Call 的成本。
func BenchmarkDispatcherHandleFrame(b *testing.B) {
	registry := testRegistry(b)
	payload, err := proto.Marshal(&echopb.EchoNotify{Text: "hello"})
	if err != nil {
		b.Fatal(err)
	}
	notifyFrame, err := proto.Marshal(&commonv1.Envelope{
		MsgId:   uint32(echopb.MsgId_C2S_ECHO_NOTIFY),
		Payload: payload,
	})
	if err != nil {
		b.Fatal(err)
	}
	requestPayload, err := proto.Marshal(&echopb.EchoRequest{Text: "hello"})
	if err != nil {
		b.Fatal(err)
	}
	requestFrame, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 1,
		Payload:   requestPayload,
	})
	if err != nil {
		b.Fatal(err)
	}

	b.Run("NotifyDecode", func(b *testing.B) {
		var text string
		b.ReportAllocs()
		b.SetBytes(int64(len(notifyFrame)))
		for range b.N {
			var envelope commonv1.Envelope
			if err := proto.Unmarshal(notifyFrame, &envelope); err != nil {
				b.Fatal(err)
			}
			message, err := registry.Decode(envelope.GetMsgId(), envelope.GetRequestId(), envelope.GetPayload())
			if err != nil {
				b.Fatal(err)
			}
			text = message.(*echopb.EchoNotify).GetText()
		}
		if text != "hello" {
			b.Fatalf("decoded text = %q, want hello", text)
		}
	})

	b.Run("NotifyDispatch", func(b *testing.B) {
		var text string
		sender := &benchmarkClientSender{}
		dispatcher := MustDispatcher[any](registry, sender, map[uint32]any{
			uint32(echopb.MsgId_C2S_ECHO_NOTIFY): func(_ any, _ Context, message *echopb.EchoNotify) error {
				text = message.GetText()
				return nil
			},
		})
		b.ReportAllocs()
		b.SetBytes(int64(len(notifyFrame)))
		b.ResetTimer()
		for range b.N {
			if err := dispatcher.HandleFrame("agent", notifyFrame); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if text != "hello" || sender.bytes != 0 {
			b.Fatalf("notification result = %q, sent bytes = %d", text, sender.bytes)
		}
	})

	b.Run("RequestResponse", func(b *testing.B) {
		sender := &benchmarkClientSender{}
		dispatcher := MustDispatcher[any](registry, sender, map[uint32]any{
			uint32(echopb.MsgId_C2S_ECHO_REQUEST): func(_ any, ctx Context, message *echopb.EchoRequest) error {
				return ctx.Response(
					uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
					&echopb.EchoResponse{Text: message.GetText()},
				)
			},
		})
		b.ReportAllocs()
		b.SetBytes(int64(len(requestFrame)))
		b.ResetTimer()
		for range b.N {
			if err := dispatcher.HandleFrame("agent", requestFrame); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if sender.bytes == 0 {
			b.Fatal("request did not produce a response")
		}
	})
}

// benchmarkClientSender 丢弃已编码帧，仅累计字节，避免记录队列或二次解码干扰基线。
type benchmarkClientSender struct {
	bytes uint64
}

func (sender *benchmarkClientSender) Send(frame []byte) error {
	sender.bytes += uint64(len(frame))
	return nil
}
