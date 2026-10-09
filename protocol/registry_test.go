package protocol

import (
	"errors"
	"testing"

	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/protocol/catalog"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

type recordedSender struct {
	envelopes []*commonv1.Envelope
	sendErr   error
}

func (sender *recordedSender) Send(frame []byte) error {
	var envelope commonv1.Envelope
	if err := proto.Unmarshal(frame, &envelope); err != nil {
		return err
	}
	sender.envelopes = append(sender.envelopes, &envelope)
	return sender.sendErr
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(
		echopb.MessageCatalogGsrExampleEchoV1Echo,
	)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return registry
}

func testDispatcher(t *testing.T, registry *Registry, sender *recordedSender) *Dispatcher[string] {
	t.Helper()
	dispatcher, err := NewDispatcher[string](registry, sender)
	if err != nil {
		t.Fatalf("NewDispatcher() error = %v", err)
	}
	return dispatcher
}

// 验证 Dispatcher 构造器拒绝缺失目录和空连接能力，不接受业务手工拼装内部组件。
func TestNewDispatcherRejectsMissingDependencies(t *testing.T) {
	registry := testRegistry(t)
	var typedNil *recordedSender
	for _, testCase := range []struct {
		name     string
		registry *Registry
		sender   ClientSender
	}{
		{name: "nil registry", sender: &recordedSender{}},
		{name: "nil sender", registry: registry},
		{name: "typed nil sender", registry: registry, sender: typedNil},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewDispatcher[string](testCase.registry, testCase.sender); err == nil {
				t.Fatal("missing dependency accepted")
			}
		})
	}
}

// 验证普通 Command 等协议 Handler 之外的业务流程也能通过 Dispatcher 发送强类型 Push。
func TestDispatcherPushUsesConstructorRegistry(t *testing.T) {
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, testRegistry(t), sender)
	if err := dispatcher.Push(uint32(echopb.MsgId_S2C_ECHO_PUSH), &echopb.EchoPush{Text: "outside handler"}); err != nil {
		t.Fatal(err)
	}
	if len(sender.envelopes) != 1 || sender.envelopes[0].GetRequestId() != 0 {
		t.Fatalf("Push envelopes = %v", sender.envelopes)
	}
	var message echopb.EchoPush
	if err := proto.Unmarshal(sender.envelopes[0].GetPayload(), &message); err != nil || message.GetText() != "outside handler" {
		t.Fatalf("Push payload = %v, %v", &message, err)
	}
	if err := dispatcher.Push(uint32(echopb.MsgId_S2C_ECHO_PUSH), &echopb.EchoResponse{}); !errors.Is(err, ErrPayloadTypeMismatch) {
		t.Fatalf("wrong Push payload error = %v", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatal("invalid Push reached client")
	}
}

func testNotificationRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry(
		[]catalog.Entry{
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

// TestNewRegistryRejectsAmbiguousEntries 验证重复业务映射与覆盖、重映射框架通用协议均在构造时拒绝。
func TestNewRegistryRejectsAmbiguousEntries(t *testing.T) {
	testCases := []struct {
		name    string
		entries []catalog.Entry
	}{
		{
			name: "reserved common msg_id",
			entries: []catalog.Entry{{
				ID:         uint32(commonv1.MsgId_S2C_ERROR_RESPONSE),
				Direction:  catalog.ClientToServer,
				Kind:       catalog.Request,
				NewMessage: func() proto.Message { return &echopb.EchoRequest{} },
			}},
		},
		{
			name: "remapped common payload",
			entries: []catalog.Entry{{
				ID:         1004,
				Direction:  catalog.ServerToClient,
				Kind:       catalog.Response,
				NewMessage: func() proto.Message { return &commonv1.ErrorResponse{} },
			}},
		},
		{
			name:    "explicit common catalog",
			entries: commonv1.MessageCatalogGsrCommonV1Common,
		},
		{
			name: "duplicate msg_id",
			entries: []catalog.Entry{
				{
					ID:         1001,
					Direction:  catalog.ClientToServer,
					Kind:       catalog.Request,
					NewMessage: func() proto.Message { return &echopb.EchoRequest{} },
				},
				{
					ID:         1001,
					Direction:  catalog.ServerToClient,
					Kind:       catalog.Response,
					NewMessage: func() proto.Message { return &echopb.EchoResponse{} },
				},
			},
		},
		{
			name: "duplicate payload type",
			entries: []catalog.Entry{
				{
					ID:         1002,
					Direction:  catalog.ClientToServer,
					Kind:       catalog.Request,
					NewMessage: func() proto.Message { return &echopb.EchoRequest{} },
				},
				{
					ID:         1003,
					Direction:  catalog.ClientToServer,
					Kind:       catalog.Request,
					NewMessage: func() proto.Message { return &echopb.EchoRequest{} },
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := NewRegistry(testCase.entries); err == nil {
				t.Fatal("NewRegistry() error = nil, want ambiguous catalog error")
			}
		})
	}
}

// TestTypedHandlerReturnsResponse 验证强类型 Handler 收到每次传入的上下文，并完成解码和响应。
func TestTypedHandlerReturnsResponse(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	var receivedScopes []string
	if err := Register(dispatcher, func(self string, context Context, request *echopb.EchoRequest) error {
		receivedScopes = append(receivedScopes, self)
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
	if err := dispatcher.HandleFrame("first-agent-context", frame); err != nil {
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
	if err := dispatcher.HandleFrame("next-agent-context", frame); err != nil {
		t.Fatalf("second HandleFrame() error = %v", err)
	}
	if len(receivedScopes) != 2 || receivedScopes[0] != "first-agent-context" || receivedScopes[1] != "next-agent-context" {
		t.Fatalf("handler contexts = %v, want each invocation's context", receivedScopes)
	}
}

// TestResponseRejectsPayloadTypeMismatch 验证 Response 的 msg_id 与 Protobuf 类型不匹配时不会发送错误载荷。
func TestResponseRejectsPayloadTypeMismatch(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(_ string, context Context, _ *echopb.EchoRequest) error {
		return context.Response(
			uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
			&echopb.EchoNotify{Text: "wrong payload type"},
		)
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 57,
	})
	if err := dispatcher.HandleFrame("", frame); !errors.Is(err, ErrPayloadTypeMismatch) {
		t.Fatalf("HandleFrame() error = %v, want ErrPayloadTypeMismatch", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want one generic ErrorResponse", len(sender.envelopes))
	}
	errorEnvelope := sender.envelopes[0]
	if errorEnvelope.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || errorEnvelope.GetRequestId() != 57 {
		t.Fatalf("ErrorResponse Envelope = msg_id %d, request_id %d; want %d, 57",
			errorEnvelope.GetMsgId(), errorEnvelope.GetRequestId(), commonv1.MsgId_S2C_ERROR_RESPONSE)
	}
	var errorResponse commonv1.ErrorResponse
	if err := proto.Unmarshal(errorEnvelope.GetPayload(), &errorResponse); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if errorResponse.GetCode() != commonv1.ErrorCode_ERROR_CODE_INTERNAL || errorResponse.GetMessage() != "request could not be processed" {
		t.Fatalf("ErrorResponse = code %v, message %q; want safe generic internal error",
			errorResponse.GetCode(), errorResponse.GetMessage())
	}
}

// TestMaximumRequestIDReturnsUnchanged 验证最大 uint32 request_id 可被处理并原样写回响应。
func TestMaximumRequestIDReturnsUnchanged(t *testing.T) {
	const maximumRequestID uint32 = ^uint32(0)
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(_ string, context Context, _ *echopb.EchoRequest) error {
		if got := context.RequestID(); got != maximumRequestID {
			return errors.New("Handler received a changed request_id")
		}
		return context.Response(
			uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
			&echopb.EchoResponse{Text: "maximum request_id"},
		)
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: maximumRequestID,
	})
	if err := dispatcher.HandleFrame("", frame); err != nil {
		t.Fatalf("HandleFrame() error = %v", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want 1", len(sender.envelopes))
	}
	if got := sender.envelopes[0].GetRequestId(); got != maximumRequestID {
		t.Fatalf("response request_id = %d, want %d", got, maximumRequestID)
	}
}

// TestTypedNotificationCanPush 验证 Notify Handler 可以发送 request_id 为零的服务端 Push。
func TestTypedNotificationCanPush(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(_ string, context Context, notification *echopb.EchoNotify) error {
		return context.Push(
			uint32(echopb.MsgId_S2C_ECHO_PUSH),
			&echopb.EchoPush{Text: notification.GetText()},
		)
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	payload, err := proto.Marshal(&echopb.EchoNotify{Text: "notify"})
	if err != nil {
		t.Fatalf("marshal EchoNotify: %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:   uint32(echopb.MsgId_C2S_ECHO_NOTIFY),
		Payload: payload,
	})
	if err := dispatcher.HandleFrame("", frame); err != nil {
		t.Fatalf("HandleFrame() error = %v", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want 1", len(sender.envelopes))
	}
	push := sender.envelopes[0]
	if push.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_PUSH) || push.GetRequestId() != 0 {
		t.Fatalf("push Envelope = msg_id %d, request_id %d; want %d, 0",
			push.GetMsgId(), push.GetRequestId(), echopb.MsgId_S2C_ECHO_PUSH)
	}
	var message echopb.EchoPush
	if err := proto.Unmarshal(push.GetPayload(), &message); err != nil {
		t.Fatalf("unmarshal EchoPush: %v", err)
	}
	if message.GetText() != "notify" {
		t.Fatalf("EchoPush text = %q, want notify", message.GetText())
	}
}

// TestNotificationCannotPushResponseMessage 验证 Notify Handler 不能把 Response 编号当作主动 Push 发送。
func TestNotificationCannotPushResponseMessage(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(_ string, context Context, _ *echopb.EchoNotify) error {
		return context.Push(
			uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
			&echopb.EchoResponse{},
		)
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId: uint32(echopb.MsgId_C2S_ECHO_NOTIFY),
	})
	if err := dispatcher.HandleFrame("", frame); err == nil {
		t.Fatal("HandleFrame() error = nil, want Push kind validation error")
	}
	if len(sender.envelopes) != 0 {
		t.Fatalf("sent Envelope count = %d, want no invalid Push", len(sender.envelopes))
	}
}

// TestUnknownMessageReturnsGenericError 验证未知 msg_id 通过通用 ErrorResponse 回传原 request_id。
func TestUnknownMessageReturnsGenericError(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 7777, RequestId: 46})
	if err := dispatcher.HandleFrame("", frame); !errors.Is(err, ErrUnknownMessageID) {
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

// TestUnknownNotificationDoesNotGenerateResponse 验证 request_id 为零的未知消息不会伪造 ErrorResponse。
func TestUnknownNotificationDoesNotGenerateResponse(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 7777})
	if err := dispatcher.HandleFrame("", frame); !errors.Is(err, ErrUnknownMessageID) {
		t.Fatalf("HandleFrame() error = %v, want ErrUnknownMessageID", err)
	}
	if len(sender.envelopes) != 0 {
		t.Fatalf("unknown notification sent %d responses, want none", len(sender.envelopes))
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
	if err := dispatcher.HandleFrame("", frame); !errors.Is(err, ErrHandlerNotRegistered) {
		t.Fatalf("HandleFrame() error = %v, want ErrHandlerNotRegistered", err)
	}
	if len(sender.envelopes) != 1 || sender.envelopes[0].GetRequestId() != 50 {
		t.Fatalf("sent error response = %+v, want original request_id 50", sender.envelopes)
	}
}

// TestDefinedNotificationWithoutHandlerDoesNotRespond 验证已定义但未注册 Handler 的 Notify 不产生无法关联的响应。
func TestDefinedNotificationWithoutHandlerDoesNotRespond(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId: uint32(echopb.MsgId_C2S_ECHO_NOTIFY),
	})
	if err := dispatcher.HandleFrame("", frame); !errors.Is(err, ErrHandlerNotRegistered) {
		t.Fatalf("HandleFrame() error = %v, want ErrHandlerNotRegistered", err)
	}
	if len(sender.envelopes) != 0 {
		t.Fatalf("unimplemented notification sent %d responses, want none", len(sender.envelopes))
	}
}

// TestRequestWithoutHandlerResponseGetsError 验证 Request Handler 忘记响应时不会让客户端无限等待。
func TestRequestWithoutHandlerResponseGetsError(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(string, Context, *echopb.EchoRequest) error { return nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 47,
	})
	if err := dispatcher.HandleFrame("", frame); !errors.Is(err, ErrResponseRequired) {
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
	if err := Register(dispatcher, func(string, Context, *echopb.EchoRequest) error { return nil }); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 7001})
	if err := dispatcher.HandleFrame("", frame); err != nil {
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
	if err := Register(dispatcher, func(_ string, context Context, _ *echopb.EchoRequest) error {
		return context.Response(uint32(commonv1.MsgId_S2C_ERROR_RESPONSE), &commonv1.ErrorResponse{})
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{MsgId: 7001})
	if err := dispatcher.HandleFrame("", frame); !errors.Is(err, ErrMissingRequestID) {
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
	if err := Register(dispatcher, func(string, Context, *echopb.EchoRequest) error {
		return errors.New("private database address")
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 49,
	})
	if err := dispatcher.HandleFrame("", frame); err == nil {
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

// TestErrorResponseSendFailurePreservesHandlerError 验证 ErrorResponse 发送失败时原始 Handler 错误仍可供 Runtime 诊断。
func TestErrorResponseSendFailurePreservesHandlerError(t *testing.T) {
	handlerErr := errors.New("business dependency unavailable")
	sendErr := errors.New("client session closed")
	registry := testRegistry(t)
	sender := &recordedSender{sendErr: sendErr}
	dispatcher := testDispatcher(t, registry, sender)
	if err := Register(dispatcher, func(string, Context, *echopb.EchoRequest) error {
		return handlerErr
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	frame := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 58,
	})
	err := dispatcher.HandleFrame("", frame)
	if !errors.Is(err, handlerErr) || !errors.Is(err, sendErr) {
		t.Fatalf("HandleFrame() error = %v, want both Handler and response send errors", err)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("send attempts = %d, want one generic ErrorResponse", len(sender.envelopes))
	}
	errorEnvelope := sender.envelopes[0]
	if errorEnvelope.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || errorEnvelope.GetRequestId() != 58 {
		t.Fatalf("ErrorResponse Envelope = msg_id %d, request_id %d; want %d, 58",
			errorEnvelope.GetMsgId(), errorEnvelope.GetRequestId(), commonv1.MsgId_S2C_ERROR_RESPONSE)
	}
	var errorResponse commonv1.ErrorResponse
	if err := proto.Unmarshal(errorEnvelope.GetPayload(), &errorResponse); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if errorResponse.GetCode() != commonv1.ErrorCode_ERROR_CODE_INTERNAL || errorResponse.GetMessage() != "request could not be processed" {
		t.Fatalf("ErrorResponse = code %v, message %q; want safe generic internal error",
			errorResponse.GetCode(), errorResponse.GetMessage())
	}
}

// TestRequestRequiresIDAndOnlyOneResponse 验证 Request 必须有编号且一个请求最多发送一次响应。
func TestRequestRequiresIDAndOnlyOneResponse(t *testing.T) {
	registry := testRegistry(t)
	sender := &recordedSender{}
	dispatcher := testDispatcher(t, registry, sender)
	handlerCalls := 0
	if err := Register(dispatcher, func(_ string, context Context, _ *echopb.EchoRequest) error {
		handlerCalls++
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
	if err := dispatcher.HandleFrame("", requestWithoutID); !errors.Is(err, ErrRequestIDMismatch) {
		t.Fatalf("request without ID HandleFrame() error = %v, want ErrRequestIDMismatch", err)
	}
	if handlerCalls != 0 {
		t.Fatalf("Handler calls after request without ID = %d, want 0", handlerCalls)
	}
	request := marshalEnvelope(t, &commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 48,
	})
	if err := dispatcher.HandleFrame("", request); !errors.Is(err, ErrDuplicateResponse) {
		t.Fatalf("request HandleFrame() error = %v, want ErrDuplicateResponse", err)
	}
	if handlerCalls != 1 {
		t.Fatalf("Handler calls after valid request = %d, want 1", handlerCalls)
	}
	if len(sender.envelopes) != 1 {
		t.Fatalf("sent Envelope count = %d, want exactly one", len(sender.envelopes))
	}
}
