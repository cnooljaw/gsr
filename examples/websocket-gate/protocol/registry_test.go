package protocol

import (
	"errors"
	"testing"

	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/examples/websocket-gate/protocol/catalog"
	"google.golang.org/protobuf/proto"
)

type recordedSender struct {
	envelopes []*commonv1.Envelope
}

func (sender *recordedSender) Send(envelope *commonv1.Envelope) error {
	sender.envelopes = append(sender.envelopes, envelope)
	return nil
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(
		commonv1.MessageCatalogGsrCommonV1Common,
		echopb.MessageCatalogGsrExampleEchoV1Echo,
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func testDispatcher(t *testing.T, registry *Registry, sender *recordedSender) *Dispatcher {
	t.Helper()
	session, err := NewSession(registry, sender)
	if err != nil {
		t.Fatalf("NewSession() error = %v", err)
	}
	dispatcher, err := NewDispatcher(registry, session)
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	return dispatcher
}

func testNotificationRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(
		[]catalog.Entry{
			{
				ID:        9001,
				Direction: catalog.ServerToClient,
				Kind:      catalog.Response,
				NewMessage: func() proto.Message {
					return &commonv1.ErrorResponse{}
				},
			},
			{
				ID:        7001,
				Direction: catalog.ClientToServer,
				Kind:      catalog.Notify,
				NewMessage: func() proto.Message {
					return &echopb.EchoRequest{}
				},
			},
		},
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func marshalEnvelope(t *testing.T, envelope *commonv1.Envelope) []byte {
	t.Helper()
	frame, err := proto.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal Envelope: %v", err)
	}
	return frame
}

// TestGeneratedRegistryDecodesMessageAndChecksRequestID 验证生成目录按 msg_id 解码并约束 Request 的 request_id。
func TestGeneratedRegistryDecodesMessageAndChecksRequestID(t *testing.T) {
	registry := testRegistry(t)
	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	message, err := registry.Decode(uint32(echopb.MsgId_C2S_ECHO_REQUEST), 12, payload)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if got := message.(*echopb.EchoRequest).GetText(); got != "hello" {
		t.Fatalf("decoded text = %q, want hello", got)
	}
	if _, err := registry.Decode(uint32(echopb.MsgId_C2S_ECHO_REQUEST), 0, payload); !errors.Is(err, ErrRequestIDMismatch) {
		t.Fatalf("Decode() error = %v, want ErrRequestIDMismatch", err)
	}
}

// TestTypedHandlerReturnsResponse 验证业务只注册已实现的强类型 Handler 即可完成解码和响应。
func TestTypedHandlerReturnsResponse(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(context Context, request *echopb.EchoRequest) error {
		return context.Response(
			uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
			&echopb.EchoResponse{Text: request.GetText()},
		)
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "typed"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 45,
		Payload:   payload,
	})
	if err := dispatcher.HandleFrame(frame); err != nil {
		t.Fatalf("HandleFrame() error = %v", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want 1", len(sender.envelopes))
	}
	response := sender.envelopes[0]
	if response.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || response.GetRequestId() != 45 {
		t.Fatalf("response = msg_id %d, request_id %d", response.GetMsgId(), response.GetRequestId())
	}
	var responsePayload echopb.EchoResponse
	if err := proto.Unmarshal(response.GetPayload(), &responsePayload); err != nil {
		t.Fatalf("unmarshal EchoResponse: %v", err)
	}
	if responsePayload.GetText() != "typed" {
		t.Fatalf("response text = %q, want typed", responsePayload.GetText())
	}
}

// TestUnknownMessageReturnsGenericError 验证未知 msg_id 通过通用 ErrorResponse 回传原 request_id。
func TestUnknownMessageReturnsGenericError(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 7777, RequestId: 46})
	if err := dispatcher.HandleFrame(frame); !errors.Is(err, ErrUnknownMessageID) {
		t.Fatalf("HandleFrame() error = %v, want ErrUnknownMessageID", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want 1", len(sender.envelopes))
	}
	response := sender.envelopes[0]
	if response.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || response.GetRequestId() != 46 {
		t.Fatalf("error Envelope = msg_id %d, request_id %d", response.GetMsgId(), response.GetRequestId())
	}
	var payload commonv1.ErrorResponse
	if err := proto.Unmarshal(response.GetPayload(), &payload); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if payload.GetCode() != commonv1.ErrorCode_ERROR_CODE_UNSUPPORTED_MESSAGE {
		t.Fatalf("ErrorResponse code = %v, want unsupported message", payload.GetCode())
	}
}

// TestDefinedMessageWithoutHandlerReturnsUnsupported 验证协议可以先定义，未实现 Handler 时按请求返回通用错误。
func TestDefinedMessageWithoutHandlerReturnsUnsupported(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 50,
	})
	if err := dispatcher.HandleFrame(frame); !errors.Is(err, ErrHandlerNotRegistered) {
		t.Fatalf("HandleFrame() error = %v, want ErrHandlerNotRegistered", err)
	}
	if len(sender.envelopes) != 1 || sender.envelopes[0].GetRequestId() != 50 {
		t.Fatalf("sent error response = %+v, want original request_id 50", sender.envelopes)
	}
}

// TestRequestWithoutHandlerResponseGetsError 验证 Request Handler 忘记响应时不会让客户端无限等待。
func TestRequestWithoutHandlerResponseGetsError(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(Context, *echopb.EchoRequest) error { return nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 47,
	})
	if err := dispatcher.HandleFrame(frame); !errors.Is(err, ErrResponseRequired) {
		t.Fatalf("HandleFrame() error = %v, want ErrResponseRequired", err)
	}
	if len(sender.envelopes) != 1 || sender.envelopes[0].GetRequestId() != 47 {
		t.Fatalf("sent error response = %+v, want original request_id 47", sender.envelopes)
	}
}

// TestNotificationNeedsNoResponse 验证 RequestID 为零的通知可以处理完成而不生成响应。
func TestNotificationNeedsNoResponse(t *testing.T) {
	registry := testNotificationRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(Context, *echopb.EchoRequest) error { return nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 7001})
	if err := dispatcher.HandleFrame(frame); err != nil {
		t.Fatalf("HandleFrame() error = %v", err)
	}
	if len(sender.envelopes) != 0 {
		t.Fatalf("notification sent %d responses, want none", len(sender.envelopes))
	}
}

// TestNotificationCannotSendResponse 验证 request_id 为零的通知不能调用 Response 伪造请求回复。
func TestNotificationCannotSendResponse(t *testing.T) {
	registry := testNotificationRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(context Context, _ *echopb.EchoRequest) error {
		return context.Response(uint32(commonv1.MsgId_S2C_ERROR_RESPONSE), &commonv1.ErrorResponse{})
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 7001})
	if err := dispatcher.HandleFrame(frame); !errors.Is(err, ErrMissingRequestID) {
		t.Fatalf("HandleFrame() error = %v, want ErrMissingRequestID", err)
	}
	if len(sender.envelopes) != 0 {
		t.Fatalf("notification sent %d responses, want none", len(sender.envelopes))
	}
}

// TestHandlerErrorDoesNotLeakDetails 验证 Handler 内部错误只转换为不含内部文本的通用响应。
func TestHandlerErrorDoesNotLeakDetails(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(Context, *echopb.EchoRequest) error {
		return errors.New("private database address")
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 49,
	})
	if err := dispatcher.HandleFrame(frame); err == nil {
		t.Fatal("HandleFrame() error = nil, want Handler error")
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want 1", len(sender.envelopes))
	}
	var response commonv1.ErrorResponse
	if err := proto.Unmarshal(sender.envelopes[0].GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if response.GetMessage() != "request could not be processed" {
		t.Fatalf("ErrorResponse message = %q, want safe generic text", response.GetMessage())
	}
}

// TestRequestRequiresIDAndOnlyOneResponse 验证 Request 必须有编号且一个请求最多发送一次响应。
func TestRequestRequiresIDAndOnlyOneResponse(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(context Context, _ *echopb.EchoRequest) error {
		if err := context.Response(uint32(echopb.MsgId_S2C_ECHO_RESPONSE), &echopb.EchoResponse{}); err != nil {
			return err
		}
		return context.Response(uint32(echopb.MsgId_S2C_ECHO_RESPONSE), &echopb.EchoResponse{})
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	requestWithoutID := marshalEnvelope(t, &commonv1.Envelope{
		MsgId: uint32(echopb.MsgId_C2S_ECHO_REQUEST),
	})
	if err := dispatcher.HandleFrame(requestWithoutID); !errors.Is(err, ErrRequestIDMismatch) {
		t.Fatalf("request without ID HandleFrame() error = %v, want ErrRequestIDMismatch", err)
	}
	request := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 48,
	})
	if err := dispatcher.HandleFrame(request); !errors.Is(err, ErrDuplicateResponse) {
		t.Fatalf("request HandleFrame() error = %v, want ErrDuplicateResponse", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want exactly one", len(sender.envelopes))
	}
}
