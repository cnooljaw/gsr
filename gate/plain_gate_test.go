package gate

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// 验证监听已启动但 Gate 尚未发布时，新连接有界等待并在发布后正常收发。
func TestPlainGateWaitsForServicePublication(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{ShutdownTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { local.Close() })
	gate, err := NewGateService(local, Config{
		Protocol: ProtocolPlain,
		Address:  "127.0.0.1:0", MaxConnections: 1, MaxOutboundQueueBytes: 1024,
		NewAgent: func(client ClientSession) runtime.Service {
			return &testAgent{client: client, disconnected: make(chan struct{})}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	publish := func() { releaseOnce.Do(func() { close(release) }) }
	defer publish()
	created := make(chan error, 1)
	go func() {
		_, err := local.CreateService(context.Background(), &delayedGateStart{gate: gate, ready: ready, release: release})
		created <- err
	}()
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("Gate listener did not start")
	}
	connection := dialTestGate(t, gate)
	defer connection.Close()
	frame, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 41})
	if err != nil {
		t.Fatal(err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatal(err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	reply := make(chan error, 1)
	go func() {
		_, _, err := connection.ReadMessage()
		reply <- err
	}()
	select {
	case err := <-reply:
		t.Fatalf("connection completed before Gate publication: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	publish()
	select {
	case err := <-created:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Gate publication did not complete")
	}
	select {
	case err := <-reply:
		if err != nil {
			t.Fatalf("connection failed after Gate publication: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client frame was not handled after Gate publication")
	}
	if metrics := gate.Metrics(); metrics.RejectedConnections != 0 || metrics.AcceptedConnections != 1 {
		t.Fatalf("connection metrics = %+v", metrics)
	}
}

// delayedGateStart 只在测试中延长 Runtime 的发布窗口，不改变生产生命周期。
type delayedGateStart struct {
	gate    *GateService
	ready   chan struct{}
	release chan struct{}
}

func (service *delayedGateStart) OnStart(ctx context.Context, self runtime.ServiceContext) error {
	if err := service.gate.OnStart(ctx, self); err != nil {
		return err
	}
	close(service.ready)
	select {
	case <-service.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (service *delayedGateStart) Handle(self runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.gate.Handle(self, command, args...)
}

func (service *delayedGateStart) OnStop(ctx context.Context) error {
	return service.gate.OnStop(ctx)
}

// TestPlainGateRoutesClientRequestAndResponse 验证 WebSocket commonv1.Envelope 经 Runtime 到 Agent 后可按原 request_id 回传。
func TestPlainGateRoutesClientRequestAndResponse(t *testing.T) {
	local, gate, agents := startTestGate(t, 8, 4)
	connection := dialTestGate(t, gate)
	defer connection.Close()
	var agent *testAgent
	select {
	case agent = <-agents:
	case <-time.After(time.Second):
		t.Fatal("Gate did not create the connection Agent")
	}

	request := &commonv1.Envelope{
		MsgId:     1001,
		RequestId: 41,
		Payload:   []byte("opaque test payload"),
	}
	requestBytes, err := proto.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, requestBytes); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	messageType, responseBytes, err := connection.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if messageType != websocket.BinaryMessage {
		t.Fatalf("message type = %d, want binary", messageType)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 41 {
		t.Fatalf("response = msg_id %d, request_id %d; want 1002, 41", response.GetMsgId(), response.GetRequestId())
	}
	if !bytes.Equal(response.GetPayload(), request.GetPayload()) {
		t.Fatalf("response payload = %q, want %q", response.GetPayload(), request.GetPayload())
	}

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent did not receive the disconnect command")
	}
	if got := gate.Metrics().AcceptedConnections; got != 1 {
		t.Fatalf("AcceptedConnections = %d, want 1", got)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateRetriesDisconnectAfterAgentMailboxRecovers 验证 PlainGate 能观察 Agent Mailbox 满载，并在容量恢复后补交通知。
func TestPlainGateRetriesDisconnectAfterAgentMailboxRecovers(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAgent := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAgent()
	agents := make(chan *blockingDebugAgent, 1)
	local, err := runtime.NewRuntime(runtime.Config{MailboxCapacity: 1, ShutdownTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	gate, err := NewGateService(local, Config{
		Protocol:              ProtocolPlain,
		Address:               "127.0.0.1:0",
		MaxConnections:        1,
		MaxOutboundQueueBytes: 64 << 10,
		NewAgent: func(ClientSession) runtime.Service {
			agent := &blockingDebugAgent{release: release, started: make(chan struct{}), disconnected: make(chan struct{})}
			agents <- agent
			return agent
		},
	})
	if err != nil {
		t.Fatalf("NewGateService() error = %v", err)
	}
	if _, err := local.CreateService(context.Background(), gate); err != nil {
		t.Fatalf("CreateService(Gate) error = %v", err)
	}
	connection := dialTestGate(t, gate)
	defer connection.Close()
	var agent *blockingDebugAgent
	select {
	case agent = <-agents:
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建连接 Agent")
	}
	frame, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 1})
	if err != nil {
		t.Fatalf("marshal test Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("WriteMessage(first frame) error = %v", err)
	}
	select {
	case <-agent.started:
	case <-time.After(time.Second):
		t.Fatal("Agent 未开始处理客户端消息")
	}
	for range 2 {
		if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			break
		}
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, _, err = connection.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseInternalServerErr)
	}
	select {
	case <-agent.disconnected:
		t.Fatal("Agent 在 Mailbox 仍满载时收到断开通知")
	case <-time.After(20 * time.Millisecond):
	}
	releaseAgent()
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent Mailbox 恢复后未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateStopNotifiesMappedAgent 验证单独停止 PlainGate 时仍会通知所有保留映射的 Agent。
func TestPlainGateStopNotifiesMappedAgent(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{ShutdownTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	agent := &testAgent{disconnected: make(chan struct{})}
	agentID, err := local.CreateService(context.Background(), agent)
	if err != nil {
		t.Fatalf("CreateService(Agent) error = %v", err)
	}
	clientConn := &clientConnection{}
	gate := &GateService{
		runtime:            local,
		connections:        make(map[*clientConnection]struct{}),
		agentsByConnection: map[*clientConnection]runtime.ServiceID{clientConn: agentID},
	}
	stopContext, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := gate.OnStop(stopContext); err != nil {
		t.Fatalf("Gate.OnStop() error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("PlainGate 单独停止后 Agent 未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateStopsClientMessageAdmissionBeforeClosingConnections 验证 PlainGate 标记停机后不会再把客户端帧送入 Agent Mailbox。
func TestPlainGateStopsClientMessageAdmissionBeforeClosingConnections(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	local, err := runtime.NewRuntime(runtime.Config{MailboxCapacity: 1, ShutdownTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	agent := &blockingDebugAgent{release: release, started: make(chan struct{}), disconnected: make(chan struct{})}
	agentID, err := local.CreateService(context.Background(), agent)
	if err != nil {
		t.Fatalf("CreateService(Agent) error = %v", err)
	}
	gate := &GateService{runtime: local}
	if err := gate.OnStop(context.Background()); err != nil {
		t.Fatalf("Gate.OnStop() error = %v", err)
	}
	if err := gate.deliverClientMessage(agentID, []byte("client frame")); !errors.Is(err, errGateStopping) {
		t.Fatalf("deliverClientMessage() error = %v, want %v", err, errGateStopping)
	}
	select {
	case <-agent.started:
		t.Fatal("Gate 停机后仍有客户端消息进入 Agent")
	default:
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateStopNotifiesHealthyAgentWhileAnotherMailboxIsFull 验证一个满载 Agent 不会挡住其他 Agent 的停机通知。
func TestPlainGateStopNotifiesHealthyAgentWhileAnotherMailboxIsFull(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAgent := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAgent()
	local, err := runtime.NewRuntime(runtime.Config{MailboxCapacity: 1, ShutdownTimeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	blocked := &blockingDebugAgent{release: release, started: make(chan struct{}), disconnected: make(chan struct{})}
	blockedID, err := local.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatalf("CreateService(blocked Agent) error = %v", err)
	}
	if err := local.DeliverClientMessage(blockedID, []byte("active")); err != nil {
		t.Fatalf("DeliverClientMessage(active) error = %v", err)
	}
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("blocked Agent 未开始处理消息")
	}
	if err := local.DeliverClientMessage(blockedID, []byte("queued")); err != nil {
		t.Fatalf("DeliverClientMessage(queued) error = %v", err)
	}
	healthy := &testAgent{disconnected: make(chan struct{})}
	healthyID, err := local.CreateService(context.Background(), healthy)
	if err != nil {
		t.Fatalf("CreateService(healthy Agent) error = %v", err)
	}
	blockedConnection := &clientConnection{}
	healthyConnection := &clientConnection{}
	gate := &GateService{
		runtime:     local,
		connections: make(map[*clientConnection]struct{}),
		agentsByConnection: map[*clientConnection]runtime.ServiceID{
			blockedConnection: blockedID,
			healthyConnection: healthyID,
		},
	}
	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopDone := make(chan error, 1)
	go func() { stopDone <- gate.OnStop(stopContext) }()
	select {
	case <-healthy.disconnected:
	case <-time.After(time.Second):
		releaseAgent()
		select {
		case <-stopDone:
		case <-time.After(time.Second):
			t.Error("Gate.OnStop() 在测试释放阻塞 Agent 后仍未返回")
		}
		t.Fatal("满载 Agent 阻塞时，健康 Agent 未及时收到停机通知")
	}
	releaseAgent()
	select {
	case <-blocked.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Mailbox 恢复后，阻塞 Agent 未收到停机通知")
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Gate.OnStop() error = %v", err)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateStopBoundsPendingDisconnectRetry 验证 Agent Mailbox 满载时 Gate 停机仍按自己的期限完成。
func TestPlainGateStopBoundsPendingDisconnectRetry(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAgent := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAgent()
	local, gate, _ := startTestGate(t, 1, 1)
	blocked := &blockingDebugAgent{release: release, started: make(chan struct{}), disconnected: make(chan struct{})}
	agentID, err := local.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatalf("CreateService(Agent) error = %v", err)
	}
	if err := local.DeliverClientMessage(agentID, []byte("active")); err != nil {
		t.Fatalf("DeliverClientMessage(active) error = %v", err)
	}
	select {
	case <-blocked.started:
	case <-time.After(time.Second):
		t.Fatal("Agent 未开始处理第一条消息")
	}
	if err := local.DeliverClientMessage(agentID, []byte("queued")); err != nil {
		t.Fatalf("DeliverClientMessage(queued) error = %v", err)
	}
	clientConn := &clientConnection{}
	gate.agentsByConnection[clientConn] = agentID
	gate.ioWorkers.Add(1)
	go func() {
		defer gate.ioWorkers.Done()
		gate.notifyDisconnected(gate.serviceID, clientConn)
	}()

	stopContext, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	stopDone := make(chan error, 1)
	go func() { stopDone <- gate.OnStop(stopContext) }()
	select {
	case stopErr := <-stopDone:
		if !errors.Is(stopErr, context.DeadlineExceeded) {
			t.Fatalf("Gate.OnStop() error = %v, want deadline exceeded while Agent remains full", stopErr)
		}
	case <-time.After(time.Second):
		releaseAgent()
		select {
		case <-stopDone:
		case <-time.After(time.Second):
			t.Error("释放 Agent 后 Gate.OnStop() 仍未完成")
		}
		t.Fatal("Gate.OnStop() 被连接协程的断开通知重试挡住")
	}
	releaseAgent()
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateStopReportsClientCloseFailure 验证 PlainGate 停机保留每条连接的 Close 帧和 socket 关闭错误。
func TestPlainGateStopReportsClientCloseFailure(t *testing.T) {
	closeFailures := []error{
		errors.New("first test socket close failure"),
		errors.New("second test socket close failure"),
	}
	closeFrameFailures := []error{
		errors.New("first test close frame write failure"),
		errors.New("second test close frame write failure"),
	}
	var closeCount atomic.Int32
	serverConnections := make(chan *websocket.Conn, len(closeFailures))
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := gateUpgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("Upgrade() error = %v", err)
			return
		}
		serverConnections <- connection
	}))
	server.Listener = &closeErrorTestListenerPlain{
		Listener:    server.Listener,
		closeErrors: closeFailures,
		closeCount:  &closeCount,
	}
	server.Start()
	t.Cleanup(server.Close)

	endpoint := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/"}
	connections := make(map[*clientConnection]struct{}, len(closeFailures))
	clientSockets := make([]*websocket.Conn, 0, len(closeFailures))
	defer func() {
		for _, clientSocket := range clientSockets {
			_ = clientSocket.Close()
		}
	}()
	for index := range closeFailures {
		clientSocket, _, err := websocket.DefaultDialer.Dial(endpoint.String(), nil)
		if err != nil {
			t.Fatalf("Dial(test WebSocket %d) error = %v", index, err)
		}
		clientSockets = append(clientSockets, clientSocket)
		var serverConnection *websocket.Conn
		select {
		case serverConnection = <-serverConnections:
		case <-time.After(time.Second):
			t.Fatalf("测试服务器未建立第 %d 条 WebSocket 连接", index+1)
		}
		serverSocket, ok := serverConnection.UnderlyingConn().(*closeErrorTestConnPlain)
		if !ok {
			t.Fatalf("UnderlyingConn() 类型 = %T, want *closeErrorTestConnPlain", serverConnection.UnderlyingConn())
		}
		serverSocket.failWrites(closeFrameFailures[index])
		connection := newClientConnection(serverConnection, 1<<20, 64<<10, &gateCounters{}, 0)
		connections[connection] = struct{}{}
	}

	gate := &GateService{connections: connections}
	stopErr := gate.OnStop(context.Background())
	for _, closeFailure := range closeFailures {
		if !errors.Is(stopErr, closeFailure) {
			t.Errorf("Gate.OnStop() error = %v, want to preserve %v", stopErr, closeFailure)
		}
	}
	for _, closeFrameFailure := range closeFrameFailures {
		if !errors.Is(stopErr, closeFrameFailure) {
			t.Errorf("Gate.OnStop() error = %v, want to preserve %v", stopErr, closeFrameFailure)
		}
	}
	if got := closeCount.Load(); got != int32(len(closeFailures)) {
		t.Fatalf("socket Close() calls = %d, want %d despite close errors", got, len(closeFailures))
	}
}

// 验证 Gate runner 创建其他 Agent 时，已有连接的客户端帧仍可直接进入对应 Agent。
func TestPlainGateDeliversExistingClientFramesWhileCreatingAnotherAgent(t *testing.T) {
	local, gate, agents := startTestGate(t, 8, 4)
	firstClient := dialTestGate(t, gate)
	defer firstClient.Close()
	select {
	case <-agents:
	case <-time.After(time.Second):
		t.Fatal("Gate did not create the first connection Agent")
	}

	secondAgentStarting := make(chan struct{})
	releaseSecondAgent := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSecondAgent) }) }
	defer release()
	gate.config.NewAgent = func(client ClientSession) runtime.Service {
		close(secondAgentStarting)
		<-releaseSecondAgent
		return &testAgent{client: client, disconnected: make(chan struct{})}
	}

	secondConnection := newClientConnection(nil, gate.config.MaxFrameBytes, gate.config.MaxOutboundQueueBytes, &gate.counters, 0)
	connectResult := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, connectErr := local.Call(ctx, gate.serviceID, gateConnectCommand, secondConnection)
		connectResult <- connectErr
	}()
	select {
	case <-secondAgentStarting:
	case <-time.After(time.Second):
		t.Fatal("Gate did not start creating another Agent")
	}

	request, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     1001,
		RequestId: 42,
	})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := firstClient.WriteMessage(websocket.BinaryMessage, request); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	if err := firstClient.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	messageType, responseBytes, err := firstClient.ReadMessage()
	if err != nil {
		t.Fatalf("existing client did not receive a response while Gate runner was creating another Agent: %v", err)
	}
	if messageType != websocket.BinaryMessage {
		t.Fatalf("message type = %d, want binary", messageType)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal response Envelope: %v", err)
	}
	if response.GetRequestId() != 42 || response.GetMsgId() != 1002 {
		t.Fatalf("response = msg_id %d, request_id %d; want 1002, 42", response.GetMsgId(), response.GetRequestId())
	}

	release()
	select {
	case err := <-connectResult:
		if err != nil {
			t.Fatalf("Runtime.Call(create second Agent) error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Agent creation did not finish after Gate runner was released")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateShutdownClosesActiveClient 验证 Runtime 关闭时 Gate 会关闭活跃连接并等待自己的 I/O 退出。
func TestPlainGateShutdownClosesActiveClient(t *testing.T) {
	local, gate, _ := startTestGate(t, 8, 4)
	connection := dialTestGate(t, gate)
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() error = %v", err)
	}
	if _, _, err := connection.ReadMessage(); err == nil {
		t.Fatal("ReadMessage() succeeded after Runtime.Close()")
	}
}

// TestPlainGateRejectsConnectionAboveLimit 验证 Plain 模式同样在 TCP 接纳处拒绝超限连接。
func TestPlainGateRejectsConnectionAboveLimit(t *testing.T) {
	_, gate, agents := startTestGate(t, 8, 1)
	first := dialTestGate(t, gate)
	defer first.Close()
	select {
	case <-agents:
	case <-time.After(time.Second):
		t.Fatal("Gate did not create the first connection Agent")
	}

	endpoint := url.URL{Scheme: "ws", Host: gate.Addr().String(), Path: "/ws"}
	second, response, err := websocket.DefaultDialer.Dial(endpoint.String(), nil)
	if second != nil {
		_ = second.Close()
	}
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err == nil || response != nil {
		t.Fatalf("second Dial() = response %v, error %v; want TCP rejection before HTTP", response, err)
	}
	if got := gate.Metrics().RejectedConnections; got != 1 {
		t.Fatalf("RejectedConnections = %d, want 1", got)
	}
}

// TestPlainGateAllowsAnyOrigin 验证调试 Gate 也允许跨主机 Origin 升级 WebSocket。
func TestPlainGateAllowsAnyOrigin(t *testing.T) {
	_, gate, _ := startTestGate(t, 8, 2)
	endpoint := "ws://" + gate.Addr().String() + "/ws"
	connection, response, err := websocket.DefaultDialer.Dial(endpoint, http.Header{
		"Origin": []string{"https://another.example"},
	})
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("cross-origin Dial() error = %v", err)
	}
	defer connection.Close()
}

// TestPlainGateCountsAgentCreationRejectionOnce 验证一次 Agent 创建失败只计入一条被拒绝连接。
func TestPlainGateCountsAgentCreationRejectionOnce(t *testing.T) {
	for _, test := range []struct {
		name     string
		newAgent func(ClientSession) runtime.Service
	}{
		{name: "constructor returns nil", newAgent: func(ClientSession) runtime.Service { return nil }},
		{name: "Agent start fails", newAgent: func(ClientSession) runtime.Service {
			return &testAgent{startErr: errors.New("start failed")}
		}},
		{name: "Agent reports mailbox full", newAgent: func(ClientSession) runtime.Service {
			return &testAgent{startErr: runtime.ErrMailboxFull}
		}},
		{name: "Agent reports service not found", newAgent: func(ClientSession) runtime.Service {
			return &testAgent{startErr: runtime.ErrServiceNotFound}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, gate, _ := startTestGate(t, 8, 1)
			var creations atomic.Int32
			gate.config.NewAgent = func(client ClientSession) runtime.Service {
				creations.Add(1)
				return test.newAgent(client)
			}
			connection := dialTestGate(t, gate)
			defer connection.Close()
			if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
				t.Fatalf("SetReadDeadline() error = %v", err)
			}
			if _, _, err := connection.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
				t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseInternalServerErr)
			}
			if got := gate.Metrics().RejectedConnections; got != 1 {
				t.Fatalf("RejectedConnections = %d, want 1", got)
			}
			if got := creations.Load(); got != 1 {
				t.Fatalf("Agent constructor calls = %d, want 1", got)
			}
		})
	}
}

// 验证创建 Call 超时后迟到的 Agent 仍收到断线通知，连接只计为拒绝而不计为成功。
func TestPlainGateDisconnectsAgentCreatedAfterCallTimeout(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{
		MaxCallWait:     40 * time.Millisecond,
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	finishCreation := func() { releaseOnce.Do(func() { close(release) }) }
	defer finishCreation()
	agent := &testAgent{disconnected: make(chan struct{})}
	gate, err := NewGateService(local, Config{
		Protocol:              ProtocolPlain,
		Address:               "127.0.0.1:0",
		MaxConnections:        1,
		MaxOutboundQueueBytes: 1024,
		NewAgent: func(client ClientSession) runtime.Service {
			agent.client = client
			close(started)
			<-release
			return agent
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.CreateService(context.Background(), gate); err != nil {
		t.Fatal(err)
	}
	connection := dialTestGate(t, gate)
	defer connection.Close()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Gate 未开始创建 Agent")
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	_, _, err = connection.ReadMessage()
	if !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() error = %v, want close 1011", err)
	}
	finishCreation()
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("迟到创建的 Agent 未收到断线通知")
	}
	if metrics := gate.Metrics(); metrics.AcceptedConnections != 0 || metrics.RejectedConnections != 1 {
		t.Fatalf("connection metrics = %+v, want accepted 0, rejected 1", metrics)
	}
}

// TestPlainGateLogsAgentCreationFailure 验证 Agent 创建失败只产生一条绑定 Gate 来源的结构化故障日志。
func TestPlainGateLogsAgentCreationFailure(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	local, err := runtime.NewRuntime(runtime.Config{Logger: logger, ShutdownTimeout: time.Second})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
		if err := logger.Close(context.Background()); err != nil {
			t.Errorf("Logger.Close() error = %v", err)
		}
	})
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}
	gate, err := NewGateService(local, Config{
		Protocol: ProtocolPlain,
		Address:  "127.0.0.1:0", MaxConnections: 1, MaxOutboundQueueBytes: 1024,
		NewAgent: func(ClientSession) runtime.Service { return nil },
	})
	if err != nil {
		t.Fatalf("NewGateService() error = %v", err)
	}
	gateID, err := local.CreateService(context.Background(), gate)
	if err != nil {
		t.Fatalf("CreateService(Gate) error = %v", err)
	}
	connection := dialTestGate(t, gate)
	defer connection.Close()
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if _, _, err := connection.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() error = %v, want internal error close", err)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}
	var matches int
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'}) {
		var entry struct {
			Message string         `json:"message"`
			Service uint64         `json:"service"`
			Fields  map[string]any `json:"fields"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode log entry: %v", err)
		}
		if entry.Message != "Gate connection failure" {
			continue
		}
		matches++
		if entry.Service != uint64(gateID) {
			t.Fatalf("Gate log service = %d, want %d", entry.Service, gateID)
		}
		if entry.Fields["category"] != "agent_create" || entry.Fields["close_code"] != float64(websocket.CloseInternalServerErr) {
			t.Fatalf("Gate log fields = %v, want Agent creation failure", entry.Fields)
		}
	}
	if matches != 1 {
		t.Fatalf("Gate failure logs = %d, want 1", matches)
	}
}

// TestPlainGateHTTPServerErrorUsesSharedLogger 验证 HTTP 内部错误进入共享日志且不泄露原始文本。
func TestPlainGateHTTPServerErrorUsesSharedLogger(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(context.Background()); err != nil {
			t.Errorf("Logger.Close() error = %v", err)
		}
	})
	local, gate, _ := startTestGateWithLogger(t, 8, 1, logger)
	if gate.server.ErrorLog == nil {
		t.Fatal("PlainGate HTTP server has no shared error logger")
	}
	gate.server.ErrorLog.Print("http: panic serving 127.0.0.1:12345: private-client-data")
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}
	if bytes.Contains(output.Bytes(), []byte("private-client-data")) {
		t.Fatal("PlainGate log contains raw HTTP server error")
	}
	var matches int
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'}) {
		var entry struct {
			Message string         `json:"message"`
			Service uint64         `json:"service"`
			Fields  map[string]any `json:"fields"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode log entry: %v", err)
		}
		if entry.Message == "Gate HTTP server error" && entry.Fields["category"] == "handler_panic" {
			matches++
			if entry.Service != uint64(gate.serviceID) {
				t.Fatalf("HTTP error service = %d, want %d", entry.Service, gate.serviceID)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("HTTP server error logs = %d, want 1", matches)
	}
}

// TestPlainGateCloseErrorLogRedactsReason 验证连接诊断保留对端 Close 码而不记录对端文本。
func TestPlainGateCloseErrorLogRedactsReason(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(context.Background()); err != nil {
			t.Errorf("Logger.Close() error = %v", err)
		}
	})
	gate := &GateService{logger: logger}
	gate.logConnectionFailure(nil, "write", websocket.CloseGoingAway, &websocket.CloseError{
		Code: websocket.CloseNormalClosure, Text: "private-client-close-reason-do-not-log",
	})
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}
	if bytes.Contains(output.Bytes(), []byte("private-client-close-reason-do-not-log")) {
		t.Fatal("PlainGate log contains client-supplied Close reason")
	}
	var entry struct {
		Message string         `json:"message"`
		Fields  map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &entry); err != nil {
		t.Fatalf("decode log entry: %v", err)
	}
	if entry.Message != "Gate connection failure" || entry.Fields["peer_close_code"] != float64(websocket.CloseNormalClosure) {
		t.Fatalf("PlainGate close log = %q, %v; want peer close code", entry.Message, entry.Fields)
	}
}

// TestPlainGateLogsUnsupportedFrameWithoutClientText 验证非二进制帧的诊断不泄露客户端原文。
func TestPlainGateLogsUnsupportedFrameWithoutClientText(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(context.Background()); err != nil {
			t.Errorf("Logger.Close() error = %v", err)
		}
	})
	local, gate, agents := startTestGateWithLogger(t, 8, 1, logger)
	connection := dialTestGate(t, gate)
	defer connection.Close()
	select {
	case <-agents:
	case <-time.After(time.Second):
		t.Fatal("Gate did not create the connection Agent")
	}
	const clientText = "private-client-content-do-not-log"
	if err := connection.WriteMessage(websocket.TextMessage, []byte(clientText)); err != nil {
		t.Fatalf("WriteMessage(text) error = %v", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if _, _, err := connection.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseUnsupportedData) {
		t.Fatalf("ReadMessage() error = %v, want unsupported data close", err)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}
	if bytes.Contains(output.Bytes(), []byte(clientText)) {
		t.Fatal("PlainGate log contains client text")
	}
	var matches int
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte{'\n'}) {
		var entry struct {
			Message string         `json:"message"`
			Fields  map[string]any `json:"fields"`
		}
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatalf("decode log entry: %v", err)
		}
		if entry.Message == "Gate connection failure" && entry.Fields["category"] == "unsupported_data" {
			matches++
			if entry.Fields["close_code"] != float64(websocket.CloseUnsupportedData) {
				t.Fatalf("close_code = %v, want %d", entry.Fields["close_code"], websocket.CloseUnsupportedData)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("unsupported_data logs = %d, want 1", matches)
	}
}

// TestPlainGateClosesIdleHTTPConnection 验证未升级的 HTTP keep-alive 连接不会无限占用 socket。
func TestPlainGateClosesIdleHTTPConnection(t *testing.T) {
	_, gate, _ := startTestGate(t, 8, 1)
	connection, err := net.Dial("tcp", gate.Addr().String())
	if err != nil {
		t.Fatalf("Dial(Gate) error = %v", err)
	}
	defer connection.Close()

	request, err := http.NewRequest(http.MethodGet, "http://"+gate.Addr().String()+"/not-ws", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	if err := request.Write(connection); err != nil {
		t.Fatalf("Write(HTTP request) error = %v", err)
	}
	reader := bufio.NewReader(connection)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		t.Fatalf("ReadResponse() error = %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound || response.Close {
		t.Fatalf("HTTP response = status %d, close %t; want 404 with keep-alive", response.StatusCode, response.Close)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("drain HTTP response body: %v", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(defaultReadHeaderTimeout + time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatalf("idle HTTP connection read error = %v, want EOF after the idle timeout", err)
	}
}

// TestPlainGateReadHeaderTimeoutReleasesConnectionCapacity 验证 Plain 的部分 HTTP 请求占用 TCP 名额，但超时后恢复接入。
func TestPlainGateReadHeaderTimeoutReleasesConnectionCapacity(t *testing.T) {
	local, gate, agents := startTestGate(t, 8, 1)
	partial, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(partial HTTP) error = %v", err)
	}
	defer partial.Close()
	started := time.Now()
	if _, err := io.WriteString(partial, "GET /ws HTTP/1.1\r\nHost: "+gate.Addr().String()+"\r\n"); err != nil {
		t.Fatalf("Write(partial HTTP header) error = %v", err)
	}

	if err := partial.SetReadDeadline(started.Add(defaultReadHeaderTimeout + 2*time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(partial HTTP) error = %v", err)
	}
	if _, err := io.Copy(io.Discard, partial); err != nil {
		t.Fatalf("partial HTTP connection did not close after header timeout: %v", err)
	}
	if elapsed := time.Since(started); elapsed < defaultReadHeaderTimeout-time.Second {
		t.Fatalf("partial HTTP connection closed after %v, before header timeout", elapsed)
	}
	client := dialTestGate(t, gate)
	defer client.Close()
	select {
	case <-agents:
	case <-time.After(time.Second):
		t.Fatal("HTTP header timeout did not release connection capacity")
	}
	// Agent 构造通知早于 HTTP 入口完成计数，等待接纳完成再断言。
	waitForGateMetric(t, gate, "AcceptedConnections", func(metrics Metrics) uint64 {
		return metrics.AcceptedConnections
	}, 1)
	metrics := gate.Metrics()
	if metrics.AcceptedConnections != 1 || metrics.RejectedConnections != 0 {
		t.Fatalf("Gate metrics = accepted %d, rejected %d; want 1, 0", metrics.AcceptedConnections, metrics.RejectedConnections)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestPlainGateStopClosesPartialHTTPHeader 验证停机立即关闭未完成请求头的连接，不等待请求头超时。
func TestPlainGateStopClosesPartialHTTPHeader(t *testing.T) {
	local, gate, _ := startTestGate(t, 8, 1)
	partial, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(partial HTTP) error = %v", err)
	}
	defer partial.Close()
	if _, err := io.WriteString(partial, "GET /ws HTTP/1.1\r\nHost: "+gate.Addr().String()+"\r\n"); err != nil {
		t.Fatalf("Write(partial HTTP header) error = %v", err)
	}
	if err := partial.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline(pending header) error = %v", err)
	}
	var byteRead [1]byte
	_, err = partial.Read(byteRead[:])
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("Read(partial HTTP) error = %v, want pending request timeout", err)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := local.Stop(stopContext); err != nil {
		t.Fatalf("Runtime.Stop(partial HTTP) error = %v", err)
	}
	if err := partial.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(closed header) error = %v", err)
	}
	if _, err := partial.Read(byteRead[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("Read(after Runtime.Stop) error = %v, want EOF", err)
	}
}

// TestNewGateServiceRejectsPublicAndUnboundedConfig 验证 PlainGate 不接受公网监听或缺少有限容量的配置。
func TestNewGateServiceRejectsPublicAndUnboundedConfig(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	base := Config{
		Protocol:              ProtocolPlain,
		Address:               "127.0.0.1:0",
		MaxConnections:        2,
		MaxOutboundQueueBytes: 1024,
		NewAgent:              func(ClientSession) runtime.Service { return &testAgent{} },
	}
	for _, test := range []struct {
		name   string
		update func(*Config)
	}{
		{name: "wildcard address", update: func(config *Config) { config.Address = "0.0.0.0:9000" }},
		{name: "unknown protocol", update: func(config *Config) { config.Protocol = Protocol(255) }},
		{name: "missing connection limit", update: func(config *Config) { config.MaxConnections = 0 }},
		{name: "missing outbound queue limit", update: func(config *Config) { config.MaxOutboundQueueBytes = 0 }},
		{name: "missing agent constructor", update: func(config *Config) { config.NewAgent = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := base
			test.update(&config)
			if _, err := NewGateService(local, config); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewGateService() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

// TestClientSessionBoundsOutboundQueuePlain 验证不透明字节和队列条目开销合计不能超过出站上限。
func TestClientSessionBoundsOutboundQueuePlain(t *testing.T) {
	client := newClientConnection(nil, 128, queueEntryOverhead+2, &gateCounters{}, 0)
	frame := []byte{0xff, 0x00}
	if err := client.Send(frame); err != nil {
		t.Fatalf("Send(first) error = %v", err)
	}
	if err := client.Send(frame); !errors.Is(err, ErrClientQueueFull) {
		t.Fatalf("Send(second) error = %v, want ErrClientQueueFull", err)
	}
	if got := client.counters.outboundQueueFull.Load(); got != 1 {
		t.Fatalf("outbound queue full count = %d, want 1", got)
	}
}

type testAgent struct {
	client       ClientSession
	disconnected chan struct{}
	once         sync.Once
	startErr     error
}

func (agent *testAgent) OnStart(context.Context, runtime.ServiceContext) error { return agent.startErr }

func (agent *testAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected test Agent command")
	}
	agent.once.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *testAgent) HandleClientMessage(_ runtime.CommandContext, frame []byte) error {
	var envelope commonv1.Envelope
	if err := proto.Unmarshal(frame, &envelope); err != nil {
		return err
	}
	if envelope.GetMsgId() != 1001 {
		return errors.New("unexpected test Agent message id")
	}
	return sendTestEnvelope(agent.client, &commonv1.Envelope{
		MsgId:     1002,
		RequestId: envelope.GetRequestId(),
		Payload:   envelope.GetPayload(),
	})
}

func (*testAgent) OnStop(context.Context) error { return nil }

type blockingDebugAgent struct {
	release      <-chan struct{}
	started      chan struct{}
	disconnected chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
}

func (*blockingDebugAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *blockingDebugAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected blocking PlainGate Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *blockingDebugAgent) HandleClientMessage(runtime.CommandContext, []byte) error {
	agent.startOnce.Do(func() { close(agent.started) })
	<-agent.release
	return nil
}

func (*blockingDebugAgent) OnStop(context.Context) error { return nil }

type closeErrorTestListenerPlain struct {
	net.Listener
	closeErrors []error
	closeCount  *atomic.Int32
	accepted    chan *closeErrorTestConnPlain
	mu          sync.Mutex
	next        int
}

func (listener *closeErrorTestListenerPlain) Accept() (net.Conn, error) {
	connection, err := listener.Listener.Accept()
	if err != nil {
		return nil, err
	}
	listener.mu.Lock()
	var closeErr error
	if listener.next < len(listener.closeErrors) {
		closeErr = listener.closeErrors[listener.next]
		listener.next++
	}
	listener.mu.Unlock()
	wrapped := &closeErrorTestConnPlain{Conn: connection, closeErr: closeErr, closeCount: listener.closeCount}
	if listener.accepted != nil {
		listener.accepted <- wrapped
	}
	return wrapped, nil
}

type closeErrorTestConnPlain struct {
	net.Conn
	closeErr    error
	closeCount  *atomic.Int32
	once        sync.Once
	mu          sync.Mutex
	writeErr    error
	deadlineErr error
}

func (connection *closeErrorTestConnPlain) Write(data []byte) (int, error) {
	connection.mu.Lock()
	writeErr := connection.writeErr
	connection.mu.Unlock()
	if writeErr != nil {
		return 0, writeErr
	}
	return connection.Conn.Write(data)
}

func (connection *closeErrorTestConnPlain) failWrites(err error) {
	connection.mu.Lock()
	connection.writeErr = err
	connection.mu.Unlock()
}

// SetWriteDeadline 允许测试模拟底层 socket 设置写截止时间失败。
func (connection *closeErrorTestConnPlain) SetWriteDeadline(deadline time.Time) error {
	connection.mu.Lock()
	deadlineErr := connection.deadlineErr
	connection.mu.Unlock()
	if deadlineErr != nil {
		return deadlineErr
	}
	return connection.Conn.SetWriteDeadline(deadline)
}

func (connection *closeErrorTestConnPlain) failWriteDeadline(err error) {
	connection.mu.Lock()
	connection.deadlineErr = err
	connection.mu.Unlock()
}

func (connection *closeErrorTestConnPlain) Close() error {
	connection.once.Do(func() {
		_ = connection.Conn.Close()
		if connection.closeCount != nil {
			connection.closeCount.Add(1)
		}
	})
	return connection.closeErr
}

func startTestGate(t *testing.T, mailboxCapacity, maxConnections int) (*runtime.Runtime, *GateService, <-chan *testAgent) {
	t.Helper()
	return startTestGateWithLogger(t, mailboxCapacity, maxConnections, nil)
}

func startTestGateWithLogger(t *testing.T, mailboxCapacity, maxConnections int, logger *logging.Logger) (*runtime.Runtime, *GateService, <-chan *testAgent) {
	t.Helper()
	local, err := runtime.NewRuntime(runtime.Config{MailboxCapacity: mailboxCapacity, ShutdownTimeout: 2 * time.Second, Logger: logger})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	agents := make(chan *testAgent, 1)
	gate, err := NewGateService(local, Config{
		Protocol:              ProtocolPlain,
		Address:               "127.0.0.1:0",
		MaxConnections:        maxConnections,
		MaxOutboundQueueBytes: 64 << 10,
		NewAgent: func(clientSession ClientSession) runtime.Service {
			agent := &testAgent{client: clientSession, disconnected: make(chan struct{})}
			agents <- agent
			return agent
		},
	})
	if err != nil {
		t.Fatalf("NewGateService() error = %v", err)
	}
	if _, err := local.CreateService(context.Background(), gate); err != nil {
		t.Fatalf("CreateService(Gate) error = %v", err)
	}
	return local, gate, agents
}

func dialTestGate(t *testing.T, gate *GateService) *websocket.Conn {
	t.Helper()
	address := gate.Addr()
	if address == nil {
		t.Fatal("GateService.Addr() is nil after creation")
	}
	endpoint := url.URL{Scheme: "ws", Host: address.String(), Path: "/ws"}
	connection, response, err := websocket.DefaultDialer.Dial(endpoint.String(), nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial(%s) error = %v", endpoint.String(), err)
	}
	return connection
}
