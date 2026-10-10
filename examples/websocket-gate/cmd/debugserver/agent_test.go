package main

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/cnooljaw/gsr"
	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/logging"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type echoTestClient struct {
	connection *websocket.Conn
}

func (client *echoTestClient) exchange(frame []byte) (*commonv1.Envelope, error) {
	if err := client.connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		return nil, err
	}
	if err := client.connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		return nil, err
	}
	_, response, err := client.connection.ReadMessage()
	if err != nil {
		return nil, err
	}
	var envelope commonv1.Envelope
	if err := proto.Unmarshal(response, &envelope); err != nil {
		return nil, err
	}
	return &envelope, nil
}

// TestEchoAgentReturnsTypedResponseWithRequestID 验证 Agent 将强类型响应关联回客户端原请求。
func TestEchoAgentReturnsTypedResponseWithRequestID(t *testing.T) {
	client := startEchoTestClient(t)
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
	responseEnvelope, err := client.exchange(frame)
	if err != nil {
		t.Fatalf("Echo round trip: %v", err)
	}
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
	client := startEchoTestClient(t)
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
	responseEnvelope, err := client.exchange(frame)
	if err != nil {
		t.Fatalf("Echo round trip: %v", err)
	}
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

// TestEchoAgentTurnsNotificationIntoPush 验证通知经 EchoService 的 Send 返回 Agent.Handle，再推送零 request_id 消息。
func TestEchoAgentTurnsNotificationIntoPush(t *testing.T) {
	client := startEchoTestClient(t)
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
	pushEnvelope, err := client.exchange(frame)
	if err != nil {
		t.Fatalf("Echo notification round trip: %v", err)
	}
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

// 复用三个已有协议用例，改为通过真实 Gate、Agent 和 EchoService 验证应用接线。
func startEchoTestClient(t *testing.T) *echoTestClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	addresses := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- gsr.Run(ctx, gsr.Config{Logging: logging.Config{Writer: io.Discard}}, &echoTestEntry{addresses: addresses})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Runtime 停止超时")
		}
	})
	var address string
	select {
	case address = <-addresses:
	case err := <-done:
		done <- err
		t.Fatalf("Gate 启动失败: %v", err)
	case <-ctx.Done():
		t.Fatal("等待 Gate 启动超时")
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws://"+address+"/ws", nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return &echoTestClient{connection: connection}
}

type echoTestEntry struct {
	addresses chan<- string
}

func (entry *echoTestEntry) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	id, err := self.CreateService(ctx, newEchoService())
	if err != nil {
		return err
	}
	if err := self.Register(echoServiceName, id); err != nil {
		return err
	}
	registry, err := newEchoRegistry()
	if err != nil {
		return err
	}
	address, err := self.OpenGate(ctx, gsr.GateConfig{
		Protocol: gate.ProtocolPlain, Address: "127.0.0.1:0", MaxConnections: 1,
		MaxOutboundQueueBytes: 64 << 10,
		NewAgent: func(client gate.ClientSession) gsr.Service {
			return newEchoAgent(client, registry)
		},
	})
	if err == nil {
		entry.addresses <- address
	}
	return err
}

func (entry *echoTestEntry) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (entry *echoTestEntry) OnStop(_ context.Context) error {
	return nil
}
