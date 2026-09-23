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

const protocolTimeoutCommand runtime.CommandID = 0x7100

// timeoutProtocolAgent 将 Agent 内部的 Core Call 超时接到客户端协议错误回复，验证超时归属不进入 Gate。
type timeoutProtocolAgent struct {
	client       ClientSession
	target       runtime.ServiceID
	self         runtime.ServiceContext
	dispatcher   *protocol.Dispatcher
	disconnected chan struct{}
	stopOnce     sync.Once
}

// blockingProtocolTarget 为测试中的慢目标，释放前不会完成 Core Command。
type blockingProtocolTarget struct {
	started     chan struct{}
	done        chan struct{}
	release     chan struct{}
	startOnce   sync.Once
	doneOnce    sync.Once
	releaseOnce sync.Once
}

func (target *blockingProtocolTarget) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (target *blockingProtocolTarget) Handle(_ runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != protocolTimeoutCommand {
		return nil, errors.New("unexpected timeout target command")
	}
	target.startOnce.Do(func() { close(target.started) })
	<-target.release
	target.doneOnce.Do(func() { close(target.done) })
	return nil, nil
}

func (target *blockingProtocolTarget) OnStop(context.Context) error {
	target.releaseOnce.Do(func() { close(target.release) })
	return nil
}

func (agent *timeoutProtocolAgent) OnStart(_ context.Context, self runtime.ServiceContext) error {
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
	agent.self = self
	agent.dispatcher = dispatcher
	return nil
}

func (agent *timeoutProtocolAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected timeout protocol Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *timeoutProtocolAgent) HandleClientMessage(_ runtime.CommandContext, frame []byte) error {
	if agent.dispatcher == nil {
		return errors.New("timeout protocol Agent dispatcher is not initialized")
	}
	return agent.dispatcher.HandleFrame(frame)
}

func (agent *timeoutProtocolAgent) onEcho(handlerContext protocol.Context, _ *echopb.EchoRequest) error {
	callContext, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := agent.self.Call(callContext, agent.target, protocolTimeoutCommand); err != nil {
		return err
	}
	return handlerContext.Response(
		uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
		&echopb.EchoResponse{Code: echopb.EchoResultCode_ECHO_RESULT_OK},
	)
}

func (agent *timeoutProtocolAgent) OnStop(context.Context) error {
	return agent.client.Close()
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

// TestProdGateReturnsErrorAfterAgentCallTimeout 验证 Agent 内部 Core Call 超时后回传通用错误，迟到内部完成不再生成第二条客户端响应。
func TestProdGateReturnsErrorAfterAgentCallTimeout(t *testing.T) {
	target := &blockingProtocolTarget{
		started: make(chan struct{}),
		done:    make(chan struct{}),
		release: make(chan struct{}),
	}
	t.Cleanup(func() { target.releaseOnce.Do(func() { close(target.release) }) })
	var targetID runtime.ServiceID
	agents := make(chan *timeoutProtocolAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &timeoutProtocolAgent{
			client:       client,
			target:       targetID,
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	var err error
	targetID, err = local.CreateService(context.Background(), target)
	if err != nil {
		t.Fatalf("CreateService(timeout target) error = %v", err)
	}
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := <-agents

	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "timeout"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	requestBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 97,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(request frame) error = %v", err)
	}
	select {
	case <-target.started:
	case <-time.After(time.Second):
		t.Fatal("timeout target did not start")
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(timeout response) error = %v", err)
	}
	var errorEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &errorEnvelope); err != nil {
		t.Fatalf("unmarshal timeout ErrorResponse Envelope: %v", err)
	}
	if errorEnvelope.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || errorEnvelope.GetRequestId() != 97 {
		t.Fatalf("timeout ErrorResponse Envelope = msg_id %d, request_id %d; want %d, 97",
			errorEnvelope.GetMsgId(), errorEnvelope.GetRequestId(), commonv1.MsgId_S2C_ERROR_RESPONSE)
	}
	var errorResponse commonv1.ErrorResponse
	if err := proto.Unmarshal(errorEnvelope.GetPayload(), &errorResponse); err != nil {
		t.Fatalf("unmarshal timeout ErrorResponse: %v", err)
	}
	if errorResponse.GetCode() != commonv1.ErrorCode_ERROR_CODE_INTERNAL {
		t.Fatalf("timeout ErrorResponse code = %v, want internal", errorResponse.GetCode())
	}

	target.releaseOnce.Do(func() { close(target.release) })
	select {
	case <-target.done:
	case <-time.After(time.Second):
		t.Fatal("timeout target did not finish after release")
	}
	if err := connection.SetReadDeadline(time.Now().Add(200 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline(late response) error = %v", err)
	}
	_, _, err = connection.ReadMessage()
	if err == nil {
		t.Fatal("late Core Call completion produced a second client response")
	}
	if !isTimeoutError(err) {
		t.Fatalf("ReadMessage(late response) error = %v, want timeout", err)
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

// TestProdGateRejectsNotificationWithRequestID 验证 Notify 带非零 request_id 时回传关联的 INVALID_REQUEST。
func TestProdGateRejectsNotificationWithRequestID(t *testing.T) {
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

	payload, err := proto.Marshal(&echopb.EchoNotify{Text: "invalid request id"})
	if err != nil {
		t.Fatalf("marshal EchoNotify: %v", err)
	}
	requestBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_NOTIFY),
		RequestId: 98,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal invalid notification Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(invalid notification frame) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(invalid notification response) error = %v", err)
	}
	var errorEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &errorEnvelope); err != nil {
		t.Fatalf("unmarshal ErrorResponse Envelope: %v", err)
	}
	if errorEnvelope.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || errorEnvelope.GetRequestId() != 98 {
		t.Fatalf("ErrorResponse Envelope = msg_id %d, request_id %d; want %d, 98",
			errorEnvelope.GetMsgId(), errorEnvelope.GetRequestId(), commonv1.MsgId_S2C_ERROR_RESPONSE)
	}
	var errorResponse commonv1.ErrorResponse
	if err := proto.Unmarshal(errorEnvelope.GetPayload(), &errorResponse); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if errorResponse.GetCode() != commonv1.ErrorCode_ERROR_CODE_INVALID_REQUEST {
		t.Fatalf("ErrorResponse code = %v, want invalid request", errorResponse.GetCode())
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("客户端断开后 Agent 未收到通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateRejectsRequestWithoutRequestID 验证无编号 Request 不产生无关联响应，且同一连接仍可继续使用。
func TestProdGateRejectsRequestWithoutRequestID(t *testing.T) {
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

	invalidPayload, err := proto.Marshal(&echopb.EchoRequest{Text: "missing request id"})
	if err != nil {
		t.Fatalf("marshal EchoRequest without request ID: %v", err)
	}
	invalidBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:   uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		Payload: invalidPayload,
	})
	if err != nil {
		t.Fatalf("marshal Request without request ID: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, invalidBytes)); err != nil {
		t.Fatalf("WriteMessage(Request without request ID) error = %v", err)
	}

	validPayload, err := proto.Marshal(&echopb.EchoRequest{Text: "connection remains usable"})
	if err != nil {
		t.Fatalf("marshal valid EchoRequest: %v", err)
	}
	validBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 99,
		Payload:   validPayload,
	})
	if err != nil {
		t.Fatalf("marshal valid Request: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 2, validBytes)); err != nil {
		t.Fatalf("WriteMessage(valid Request) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(valid response) error = %v", err)
	}
	var responseEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &responseEnvelope); err != nil {
		t.Fatalf("unmarshal valid response Envelope: %v", err)
	}
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 99 {
		t.Fatalf("response Envelope = msg_id %d, request_id %d; want %d, 99",
			responseEnvelope.GetMsgId(), responseEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_RESPONSE)
	}
	var response echopb.EchoResponse
	if err := proto.Unmarshal(responseEnvelope.GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal EchoResponse: %v", err)
	}
	if response.GetText() != "connection remains usable" {
		t.Fatalf("EchoResponse text = %q, want connection remains usable", response.GetText())
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("客户端断开后 Agent 未收到通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateKeepsConnectionAfterMalformedEnvelope 验证无法解码的 Envelope 不生成无关联响应，且连接可继续处理请求。
func TestProdGateKeepsConnectionAfterMalformedEnvelope(t *testing.T) {
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

	malformedEnvelope := []byte{0x0a, 0x05, 0x01}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, malformedEnvelope)); err != nil {
		t.Fatalf("WriteMessage(malformed Envelope) error = %v", err)
	}

	requestPayload, err := proto.Marshal(&echopb.EchoRequest{Text: "envelope decode recovered"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	requestBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 100,
		Payload:   requestPayload,
	})
	if err != nil {
		t.Fatalf("marshal valid Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 2, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(valid Envelope) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(valid response) error = %v", err)
	}
	var responseEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &responseEnvelope); err != nil {
		t.Fatalf("unmarshal valid response Envelope: %v", err)
	}
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 100 {
		t.Fatalf("response Envelope = msg_id %d, request_id %d; want %d, 100",
			responseEnvelope.GetMsgId(), responseEnvelope.GetRequestId(), echopb.MsgId_S2C_ECHO_RESPONSE)
	}
	var response echopb.EchoResponse
	if err := proto.Unmarshal(responseEnvelope.GetPayload(), &response); err != nil {
		t.Fatalf("unmarshal EchoResponse: %v", err)
	}
	if response.GetText() != "envelope decode recovered" {
		t.Fatalf("EchoResponse text = %q, want envelope decode recovered", response.GetText())
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("客户端断开后 Agent 未收到通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateReturnsErrorForInvalidPayload 验证已知消息编号的坏 payload 由协议层返回通用错误，且不会关闭连接。
func TestProdGateReturnsErrorForInvalidPayload(t *testing.T) {
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

	invalidBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 93,
		Payload:   []byte{0xff},
	})
	if err != nil {
		t.Fatalf("marshal invalid payload Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, invalidBytes)); err != nil {
		t.Fatalf("WriteMessage(invalid payload frame) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(invalid payload response) error = %v", err)
	}
	var errorEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &errorEnvelope); err != nil {
		t.Fatalf("unmarshal ErrorResponse Envelope: %v", err)
	}
	if errorEnvelope.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || errorEnvelope.GetRequestId() != 93 {
		t.Fatalf("ErrorResponse Envelope = msg_id %d, request_id %d; want %d, 93",
			errorEnvelope.GetMsgId(), errorEnvelope.GetRequestId(), commonv1.MsgId_S2C_ERROR_RESPONSE)
	}
	var errorResponse commonv1.ErrorResponse
	if err := proto.Unmarshal(errorEnvelope.GetPayload(), &errorResponse); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if errorResponse.GetCode() != commonv1.ErrorCode_ERROR_CODE_INVALID_REQUEST {
		t.Fatalf("ErrorResponse code = %v, want invalid request", errorResponse.GetCode())
	}

	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "after invalid payload"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	validBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 94,
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
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 94 {
		t.Fatalf("valid response Envelope = msg_id %d, request_id %d; want %d, 94",
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

// TestProdGateReturnsErrorForWrongDirection 验证客户端反向发送服务端 Push 编号时返回通用错误，且连接仍可继续使用。
func TestProdGateReturnsErrorForWrongDirection(t *testing.T) {
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

	payload, err := proto.Marshal(&echopb.EchoPush{Text: "wrong direction"})
	if err != nil {
		t.Fatalf("marshal EchoPush: %v", err)
	}
	wrongDirectionBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_S2C_ECHO_PUSH),
		RequestId: 95,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("marshal wrong-direction Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, wrongDirectionBytes)); err != nil {
		t.Fatalf("WriteMessage(wrong-direction frame) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(wrong-direction response) error = %v", err)
	}
	var errorEnvelope commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &errorEnvelope); err != nil {
		t.Fatalf("unmarshal ErrorResponse Envelope: %v", err)
	}
	if errorEnvelope.GetMsgId() != uint32(commonv1.MsgId_S2C_ERROR_RESPONSE) || errorEnvelope.GetRequestId() != 95 {
		t.Fatalf("ErrorResponse Envelope = msg_id %d, request_id %d; want %d, 95",
			errorEnvelope.GetMsgId(), errorEnvelope.GetRequestId(), commonv1.MsgId_S2C_ERROR_RESPONSE)
	}
	var errorResponse commonv1.ErrorResponse
	if err := proto.Unmarshal(errorEnvelope.GetPayload(), &errorResponse); err != nil {
		t.Fatalf("unmarshal ErrorResponse: %v", err)
	}
	if errorResponse.GetCode() != commonv1.ErrorCode_ERROR_CODE_INVALID_REQUEST {
		t.Fatalf("ErrorResponse code = %v, want invalid request", errorResponse.GetCode())
	}

	validPayload, err := proto.Marshal(&echopb.EchoRequest{Text: "after wrong direction"})
	if err != nil {
		t.Fatalf("marshal EchoRequest: %v", err)
	}
	validBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     uint32(echopb.MsgId_C2S_ECHO_REQUEST),
		RequestId: 96,
		Payload:   validPayload,
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
	if responseEnvelope.GetMsgId() != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || responseEnvelope.GetRequestId() != 96 {
		t.Fatalf("valid response Envelope = msg_id %d, request_id %d; want %d, 96",
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
