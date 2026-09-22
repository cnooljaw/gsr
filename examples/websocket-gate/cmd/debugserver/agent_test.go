package main

import (
	"context"
	"testing"

	"github.com/cnooljaw/gsr/examples/websocket-gate/debuggate"
	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"google.golang.org/protobuf/proto"
)

type recordedClientSession struct {
	envelopes []*debuggate.Envelope
}

func (session *recordedClientSession) Send(envelope *debuggate.Envelope) error {
	session.envelopes = append(session.envelopes, envelope)
	return nil
}

func (*recordedClientSession) Close() error {
	return nil
}

// TestEchoAgentReturnsTypedResponseWithRequestID 验证 Agent 将强类型响应关联回客户端原请求。
func TestEchoAgentReturnsTypedResponseWithRequestID(t *testing.T) {
	client := &recordedClientSession{}
	registry, err := newEchoRegistry()
	if err != nil {
		t.Fatalf("newEchoRegistry() error = %v", err)
	}
	agent := &echoAgent{client: client, registry: registry}
	if err := agent.OnStart(context.Background(), nil); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "hello"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	frame, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 37,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal Envelope: %v", err)
	}
	if err := agent.HandleClientMessage(nil, frame); err != nil {
		t.Fatalf("HandleClientMessage() error = %v", err)
	}
	if len(client.envelopes) != 1 {
		t.Fatalf("Agent sent %d Envelopes, want 1", len(client.envelopes))
	}
	responseEnvelope := client.envelopes[0]
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 37 {
		t.Fatalf("response Envelope = msg_id %d, request_id %d; want %d, 37",
			responseEnvelope.GetMsgId(), responseEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_RESPONSE)
	}
	var response echopb.EchoResponse
	if err := proto.Unmarshal(responseEnvelope.GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal EchoResponse: %v", err)
	}
	if response.GetCode() != echopb.EchoResultCode_ECHO_RESULT_OK || response.GetText() != "hello" {
		t.Fatalf("EchoResponse = code %v, text %q, want OK and hello", response.GetCode(), response.GetText())
	}
}

// TestEchoAgentReturnsBusinessRejection 验证业务拒绝仍使用原 request_id 的业务 Response，而不是通用 ErrorResponse。
func TestEchoAgentReturnsBusinessRejection(t *testing.T) {
	client := &recordedClientSession{}
	registry, err := newEchoRegistry()
	if err != nil {
		t.Fatalf("newEchoRegistry() error = %v", err)
	}
	agent := &echoAgent{client: client, registry: registry}
	if err := agent.OnStart(context.Background(), nil); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	payload, err := proto.Marshal(&echopb.EchoRequest{})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	frame, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 38,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal Envelope: %v", err)
	}
	if err := agent.HandleClientMessage(nil, frame); err != nil {
		t.Fatalf("HandleClientMessage() error = %v", err)
	}
	if len(client.envelopes) != 1 {
		t.Fatalf("Agent sent %d Envelopes, want 1", len(client.envelopes))
	}
	responseEnvelope := client.envelopes[0]
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 38 {
		t.Fatalf("business response = msg_id %d, request_id %d; want %d, 38",
			responseEnvelope.GetMsgId(), responseEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_RESPONSE)
	}
	var response echopb.EchoResponse
	if err := proto.Unmarshal(responseEnvelope.GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal EchoResponse: %v", err)
	}
	if response.GetCode() != echopb.EchoResultCode_ECHO_RESULT_TEXT_REQUIRED || response.GetMessage() != "text is required" {
		t.Fatalf("business response = code %v, message %q; want TEXT_REQUIRED and safe message",
			response.GetCode(), response.GetMessage())
	}
}

// TestEchoAgentTurnsNotificationIntoPush 验证零 request_id 的通知不产生响应，而是可以主动推送一条服务端消息。
func TestEchoAgentTurnsNotificationIntoPush(t *testing.T) {
	client := &recordedClientSession{}
	registry, err := newEchoRegistry()
	if err != nil {
		t.Fatalf("newEchoRegistry() error = %v", err)
	}
	agent := &echoAgent{client: client, registry: registry}
	if err := agent.OnStart(context.Background(), nil); err != nil {
		t.Fatalf("OnStart() error = %v", err)
	}
	payload, err := proto.Marshal(&echopb.EchoNotify{Text: "notify"})
	if err != nil {
		t.Fatalf("marshal EchoNotify: %v", err)
	}
	frame, err := proto.Marshal(&commonv1.Envelope{
		MsgId:   uint32(echopb.MsgId_C2S_ECHO_NOTIFY),
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("marshal Envelope: %v", err)
	}
	if err := agent.HandleClientMessage(nil, frame); err != nil {
		t.Fatalf("HandleClientMessage() error = %v", err)
	}
	if len(client.envelopes) != 1 {
		t.Fatalf("Agent sent %d Envelopes, want 1 push", len(client.envelopes))
	}
	pushEnvelope := client.envelopes[0]
	if pushEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_PUSH) || pushEnvelope.GetRequestId() != 0 {
		t.Fatalf("push Envelope = msg_id %d, request_id %d; want %d, 0",
			pushEnvelope.GetMsgId(), pushEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_PUSH)
	}
	var push echopb.EchoPush
	if err := proto.Unmarshal(pushEnvelope.GetPayload(), &push); err != nil {
		t.Fatalf("unmarshal EchoPush: %v", err)
	}
	if push.GetText() != "notify" {
		t.Fatalf("EchoPush text = %q, want notify", push.GetText())
	}
}
