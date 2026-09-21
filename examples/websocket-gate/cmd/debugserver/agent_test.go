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
	envelope *debuggate.Envelope
}

func (session *recordedClientSession) Send(envelope *debuggate.Envelope) error {
	session.envelope = envelope
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
	if client.envelope == nil {
		t.Fatal("Agent did not send an Envelope")
	}
	if client.envelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || client.envelope.GetRequestId() != 37 {
		t.Fatalf("response Envelope = msg_id %d, request_id %d; want %d, 37",
			client.envelope.GetMsgId(), client.envelope.GetRequestId(), echopb.MsgId_S2C_ECHO_RESPONSE)
	}
	var response echopb.EchoResponse
	if err := proto.Unmarshal(client.envelope.GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal EchoResponse: %v", err)
	}
	if response.GetText() != "hello" {
		t.Fatalf("EchoResponse text = %q, want %q", response.GetText(), "hello")
	}
}
