package prodgate

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/examples/websocket-gate/protocol"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// protocolGateAgent 将真实协议分发器接入生产 Gate 测试，验证未知消息不会破坏连接。
type protocolGateAgent struct {
	client       ClientSession
	dispatcher   *protocol.Dispatcher
	disconnected chan struct{}
	stopOnce     sync.Once
}

func (agent *protocolGateAgent) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	registry, err := protocol.NewRegistry(
		commonv1.MessageCatalogGsrCommonV1Common,
		echopb.MessageCatalogGsrExampleEchoV1Echo,
	)
	if err != nil {
		return err
	}
	session, err := protocol.NewSession(registry, agent.client)
	if err != nil {
		return err
	}
	dispatcher, err := protocol.NewDispatcher(registry, session)
	if err != nil {
		return err
	}
	if err := protocol.Register(dispatcher, agent.onEcho); err != nil {
		return err
	}
	if err := protocol.Register(dispatcher, agent.onEchoNotify); err != nil {
		return err
	}
	agent.dispatcher = dispatcher
	return nil
}

func (agent *protocolGateAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected protocol Gate Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *protocolGateAgent) HandleClientMessage(_ runtime.CommandContext, frame []byte) error {
	if agent.dispatcher == nil {
		return errors.New("protocol Gate Agent dispatcher is not initialized")
	}
	return agent.dispatcher.HandleFrame(frame)
}

func (agent *protocolGateAgent) onEcho(context protocol.Context, request *echopb.EchoRequest) error {
	return context.Response(
		uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
		&echopb.EchoResponse{
			Code: echopb.EchoResultCode_ECHO_RESULT_OK,
			Text: request.GetText(),
		},
	)
}

func (agent *protocolGateAgent) onEchoNotify(context protocol.Context, request *echopb.EchoNotify) error {
	return context.Push(
		uint32(echopb.MsgId_S2C_ECHO_PUSH),
		&echopb.EchoPush{Text: request.GetText()},
	)
}

func (agent *protocolGateAgent) OnStop(context.Context) error {
	return agent.client.Close()
}

// TestProdGateKeepsConnectionAfterUnknownMessage 验证合法认证帧中的未知 msg_id 只回业务层 ErrorResponse，连接仍可继续处理后续请求。
func TestProdGateKeepsConnectionAfterUnknownMessage(t *testing.T) {
	agents := make(chan *protocolGateAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &protocolGateAgent{client: client, disconnected: make(chan struct{})}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := <-agents

	unknownBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 7777, RequestId: 90})
	if err != nil {
		t.Fatalf("marshal unknown Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, unknownBytes)); err != nil {
		t.Fatalf("WriteMessage(unknown frame) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(unknown response) error = %v", err)
	}
	var errorEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &errorEnvelope); err != nil {
		t.Fatalf("unmarshal ErrorResponse Envelope: %v", err)
	}
	if errorEnvelope.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || errorEnvelope.GetRequestId() != 90 {
		t.Fatalf("ErrorResponse Envelope = msg_id %d, request_id %d; want %d, 90",
			errorEnvelope.GetMsgId(), errorEnvelope.GetRequestId(), commonv1.MsgId_S2C_ERROR_RESPONSE)
	}
	var errorResponse commonv1.ErrorResponse
	if err := proto.Unmarshal(errorEnvelope.GetPayload(), &errorResponse); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if errorResponse.GetCode() != commonv1.ErrorCode_ERROR_CODE_UNSUPPORTED_MESSAGE {
		t.Fatalf("ErrorResponse code = %v, want unsupported message", errorResponse.GetCode())
	}

	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "still alive"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	validBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 91,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal valid Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 2, validBytes)); err != nil {
		t.Fatalf("WriteMessage(valid frame) error = %v", err)
	}
	_, responseBytes, err = readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(valid response) error = %v", err)
	}
	var responseEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &responseEnvelope); err != nil {
		t.Fatalf("unmarshal valid response Envelope: %v", err)
	}
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 91 {
		t.Fatalf("valid response Envelope = msg_id %d, request_id %d; want %d, 91",
			responseEnvelope.GetMsgId(), responseEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_RESPONSE)
	}
	var response echopb.EchoResponse
	if err := proto.Unmarshal(responseEnvelope.GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal EchoResponse: %v", err)
	}
	if response.GetText() != "still alive" {
		t.Fatalf("EchoResponse text = %q, want still alive", response.GetText())
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateKeepsConnectionAfterUnknownNotification 验证未知通知不回 ErrorResponse，且同一连接仍可处理后续请求。
func TestProdGateKeepsConnectionAfterUnknownNotification(t *testing.T) {
	agents := make(chan *protocolGateAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &protocolGateAgent{client: client, disconnected: make(chan struct{})}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := <-agents

	unknownBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 7778})
	if err != nil {
		t.Fatalf("marshal unknown notification Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, unknownBytes)); err != nil {
		t.Fatalf("WriteMessage(unknown notification frame) error = %v", err)
	}

	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "notification ignored"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	validBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 92,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal valid Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 2, validBytes)); err != nil {
		t.Fatalf("WriteMessage(valid frame) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(valid response) error = %v", err)
	}
	var responseEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &responseEnvelope); err != nil {
		t.Fatalf("unmarshal valid response Envelope: %v", err)
	}
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 92 {
		t.Fatalf("valid response Envelope = msg_id %d, request_id %d; want %d, 92",
			responseEnvelope.GetMsgId(), responseEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_RESPONSE)
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateDeliversNotificationPush 验证生产 Gate 能将零 request_id 的客户端通知转换为服务端主动 Push。
func TestProdGateDeliversNotificationPush(t *testing.T) {
	agents := make(chan *protocolGateAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &protocolGateAgent{client: client, disconnected: make(chan struct{})}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := <-agents

	payload, err := proto.Marshal(&echopb.EchoNotify{Text: "push from notification"})
	if err != nil {
		t.Fatalf("marshal EchoNotify: %v", err)
	}
	notificationBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:   uint32(echopb.MsgId_C2S_ECHO_NOTIFY),
		Payload: payload,
	})
	if err != nil {
		t.Fatalf("marshal notification Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, notificationBytes)); err != nil {
		t.Fatalf("WriteMessage(notification frame) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(push) error = %v", err)
	}
	var responseEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &responseEnvelope); err != nil {
		t.Fatalf("unmarshal push Envelope: %v", err)
	}
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_PUSH) || responseEnvelope.GetRequestId() != 0 {
		t.Fatalf("push Envelope = msg_id %d, request_id %d; want %d, 0",
			responseEnvelope.GetMsgId(), responseEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_PUSH)
	}
	var response echopb.EchoPush
	if err := proto.Unmarshal(responseEnvelope.GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal EchoPush: %v", err)
	}
	if response.GetText() != "push from notification" {
		t.Fatalf("EchoPush text = %q, want push from notification", response.GetText())
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}
