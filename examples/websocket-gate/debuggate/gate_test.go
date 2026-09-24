package debuggate

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// TestDebugGateRoutesClientRequestAndResponse 验证 WebSocket Envelope 经 Runtime 到 Agent 后可按原 request_id 回传。
func TestDebugGateRoutesClientRequestAndResponse(t *testing.T) {
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

// TestDebugGateStopReportsClientCloseFailure 验证 DebugGate 停机保留每条连接的 Close 帧和 socket 关闭错误。
func TestDebugGateStopReportsClientCloseFailure(t *testing.T) {
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
		connection, err := debugUpgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("Upgrade() error = %v", err)
			return
		}
		serverConnections <- connection
	}))
	server.Listener = &closeErrorTestListener{
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
		serverSocket, ok := serverConnection.UnderlyingConn().(*closeErrorTestConn)
		if !ok {
			t.Fatalf("UnderlyingConn() 类型 = %T, want *closeErrorTestConn", serverConnection.UnderlyingConn())
		}
		serverSocket.failWrites(closeFrameFailures[index])
		connection := newClientConnection(serverConnection, 1<<20, 64<<10, &gateCounters{})
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
func TestDebugGateDeliversExistingClientFramesWhileCreatingAnotherAgent(t *testing.T) {
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

	secondConnection := &clientConnection{
		maxFrame: gate.config.MaxFrameBytes,
		maxQueue: gate.config.MaxOutboundQueueBytes,
		counters: &gate.counters,
		wake:     make(chan struct{}, 1),
		closedCh: make(chan struct{}),
	}
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

// TestDebugGateShutdownClosesActiveClient 验证 Runtime 关闭时 Gate 会关闭活跃连接并等待自己的 I/O 退出。
func TestDebugGateShutdownClosesActiveClient(t *testing.T) {
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

// TestDebugGateRejectsConnectionAboveLimit 验证达到显式连接上限后 Gate 在 WebSocket 升级前拒绝新连接。
func TestDebugGateRejectsConnectionAboveLimit(t *testing.T) {
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
	if err == nil || response == nil || response.StatusCode != 503 {
		t.Fatalf("second Dial() = response %v, error %v; want HTTP 503", response, err)
	}
	if got := gate.Metrics().RejectedConnections; got != 1 {
		t.Fatalf("RejectedConnections = %d, want 1", got)
	}
}

// TestNewGateServiceRejectsPublicAndUnboundedConfig 验证 DebugGate 不接受公网监听或缺少有限容量的配置。
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

// TestClientSessionBoundsOutboundQueue 验证序列化帧和队列条目开销合计不能超过出站字节上限。
func TestClientSessionBoundsOutboundQueue(t *testing.T) {
	client := &clientConnection{
		maxFrame: 128,
		maxQueue: queueEntryOverhead + 2,
		counters: &gateCounters{},
		wake:     make(chan struct{}, 1),
		closedCh: make(chan struct{}),
	}
	envelope := &Envelope{MsgId: 1}
	if err := client.Send(envelope); err != nil {
		t.Fatalf("Send(first) error = %v", err)
	}
	if err := client.Send(envelope); !errors.Is(err, ErrClientQueueFull) {
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
}

func (*testAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

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
	return agent.client.Send(&commonv1.Envelope{
		MsgId:     1002,
		RequestId: envelope.GetRequestId(),
		Payload:   envelope.GetPayload(),
	})
}

func (*testAgent) OnStop(context.Context) error { return nil }

type closeErrorTestListener struct {
	net.Listener
	closeErrors []error
	closeCount  *atomic.Int32
	accepted    chan *closeErrorTestConn
	mu          sync.Mutex
	next        int
}

func (listener *closeErrorTestListener) Accept() (net.Conn, error) {
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
	wrapped := &closeErrorTestConn{Conn: connection, closeErr: closeErr, closeCount: listener.closeCount}
	if listener.accepted != nil {
		listener.accepted <- wrapped
	}
	return wrapped, nil
}

type closeErrorTestConn struct {
	net.Conn
	closeErr   error
	closeCount *atomic.Int32
	once       sync.Once
	mu         sync.Mutex
	writeErr   error
}

func (connection *closeErrorTestConn) Write(data []byte) (int, error) {
	connection.mu.Lock()
	writeErr := connection.writeErr
	connection.mu.Unlock()
	if writeErr != nil {
		return 0, writeErr
	}
	return connection.Conn.Write(data)
}

func (connection *closeErrorTestConn) failWrites(err error) {
	connection.mu.Lock()
	connection.writeErr = err
	connection.mu.Unlock()
}

func (connection *closeErrorTestConn) Close() error {
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
	local, err := runtime.NewRuntime(runtime.Config{MailboxCapacity: mailboxCapacity, ShutdownTimeout: 2 * time.Second})
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
