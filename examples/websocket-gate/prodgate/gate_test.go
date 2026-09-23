package prodgate

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	commonv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// TestProdGateAuthenticatesAndDeliversClientFrames 验证 WSS 握手、认证帧投递、Envelope 写回和断开通知形成完整链路。
func TestProdGateAuthenticatesAndDeliversClientFrames(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()

	var agent *productionTestAgent
	select {
	case agent = <-agents:
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建连接 Agent")
	}

	request := &commonv1.Envelope{
		MsgId:     1001,
		RequestId: 41,
		Payload:   []byte("authenticated payload"),
	}
	requestBytes, err := proto.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(authenticated frame) error = %v", err)
	}

	select {
	case delivered := <-agent.received:
		if !bytes.Equal(delivered, requestBytes) {
			t.Fatalf("Agent 收到 Envelope = %x，want %x", delivered, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("Gate 未将已认证 Envelope 投递给 Agent")
	}

	responseType, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(response) error = %v", err)
	}
	if responseType != websocket.BinaryMessage {
		t.Fatalf("response message type = %d, want binary", responseType)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != request.GetRequestId() {
		t.Fatalf("response = msg_id %d, request_id %d; want 1002, %d", response.GetMsgId(), response.GetRequestId(), request.GetRequestId())
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
		t.Fatal("Agent 未收到客户端断开通知")
	}
	if got := gate.Metrics().AcceptedConnections; got != 1 {
		t.Fatalf("AcceptedConnections = %d, want 1", got)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateRejectsLateAgentResponseAfterDisconnect 验证客户端断开后 Agent 的迟到写回返回稳定关闭错误。
func TestProdGateRejectsLateAgentResponseAfterDisconnect(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 1, time.Second)
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)

	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent 未收到客户端断开通知")
	}
	if err := agent.client.Send(&commonv1.Envelope{MsgId: 1002, RequestId: 91}); !errors.Is(err, ErrClientSessionClosed) {
		t.Fatalf("late ClientSession.Send() error = %v, want %v", err, ErrClientSessionClosed)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateSupportsIngressTerminatedTLS 验证 Ingress 终止 TLS 后 Pod 仍可用普通 WebSocket 完成同一套应用握手和消息投递。
func TestProdGateSupportsIngressTerminatedTLS(t *testing.T) {
	agents := make(chan *productionTestAgent, 1)
	_, gate := startConfiguredHTTPProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	connection := dialPlainTestClient(t, gate)
	defer connection.Close()
	macKey := authenticateTestClient(t, connection)
	agent := receiveProductionAgent(t, agents)

	request, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     1001,
		RequestId: 7,
		Payload:   []byte("ingress terminated"),
	})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, request)); err != nil {
		t.Fatalf("WriteMessage(authenticated frame) error = %v", err)
	}
	select {
	case delivered := <-agent.received:
		if !bytes.Equal(delivered, request) {
			t.Fatalf("Agent 收到 Envelope = %x，want %x", delivered, request)
		}
	case <-time.After(time.Second):
		t.Fatal("普通 HTTP WebSocket 未将已认证 Envelope 投递给 Agent")
	}

	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(response) error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 7 {
		t.Fatalf("response = msg_id %d, request_id %d; want 1002, 7", response.GetMsgId(), response.GetRequestId())
	}
}

// TestProdGateRejectsInvalidMACBeforeDelivery 验证错误 MAC 在进入 Agent Mailbox 前被拒绝并关闭连接。
func TestProdGateRejectsInvalidMACBeforeDelivery(t *testing.T) {
	_, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 9})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	frame := makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)
	frame[len(frame)-1] ^= 0xff
	if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("WriteMessage(tampered frame) error = %v", err)
	}

	_, _, err = readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.ClosePolicyViolation)
	}
	select {
	case delivered := <-agent.received:
		t.Fatalf("invalid MAC reached Agent as %x", delivered)
	case <-time.After(30 * time.Millisecond):
	}
}

// TestProdGateWritesConnectionFailureToRuntimeLogger 验证 Gate 复用 Runtime 的共享 Logger，并记录安全失败而不记录原始载荷。
func TestProdGateWritesConnectionFailureToRuntimeLogger(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Close(context.Background()) })
	local, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second, logger)
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 11, Payload: []byte("secret payload")})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	frame := makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)
	frame[len(frame)-1] ^= 0xff
	if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("WriteMessage(tampered frame) error = %v", err)
	}
	_, _, _ = readTestBinaryMessage(t, connection)
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent 未收到断开通知")
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() error = %v", err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	foundFailure := false
	for decoder.More() {
		var entry struct {
			Service uint64         `json:"service"`
			Message string         `json:"message"`
			Fields  map[string]any `json:"fields"`
		}
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("decode log entry: %v", err)
		}
		if entry.Message != "Gate connection failure" {
			continue
		}
		foundFailure = true
		if entry.Fields["category"] != "mac" {
			t.Fatalf("Gate failure category = %v, want mac", entry.Fields["category"])
		}
		if entry.Service == 0 {
			t.Fatal("Gate failure log has no Runtime ServiceID")
		}
		if _, exists := entry.Fields["connection_id"]; !exists {
			t.Fatal("Gate failure log has no connection_id")
		}
		if _, exists := entry.Fields["secret payload"]; exists {
			t.Fatal("Gate failure log contains raw payload field")
		}
	}
	if !foundFailure {
		t.Fatal("missing Gate connection failure log")
	}
}

// TestProdGateSuppressesRepeatedConnectionFailures 验证同一连接同一类别只记录首条故障，并在结束时记录抑制数量。
func TestProdGateSuppressesRepeatedConnectionFailures(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Close(context.Background()) })
	client := newClientConnection(nil, 128, queueEntryOverhead, &gateCounters{}, 7)
	gate := &GateService{logger: logger}
	gate.logConnectionFailure(client, "mac", websocket.ClosePolicyViolation, errClientMACInvalid)
	gate.logConnectionFailure(client, "mac", websocket.ClosePolicyViolation, errClientMACInvalid)
	gate.flushConnectionDiagnostics(client)
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	var messages []string
	var suppressed uint64
	for decoder.More() {
		var entry struct {
			Message string `json:"message"`
			Fields  struct {
				Suppressed uint64 `json:"suppressed"`
			} `json:"fields"`
		}
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("decode log entry: %v", err)
		}
		messages = append(messages, entry.Message)
		if entry.Message == "Gate repeated failures" {
			suppressed = entry.Fields.Suppressed
		}
	}
	if len(messages) != 2 || messages[0] != "Gate connection failure" || messages[1] != "Gate repeated failures" || suppressed != 1 {
		t.Fatalf("log messages = %v, suppressed = %d", messages, suppressed)
	}
}

// TestProdGateAppliesConnectionLimitBeforeTLSHandshake 验证连接上限也覆盖尚未完成 TLS/HTTP 握手的 TCP 连接。
func TestProdGateAppliesConnectionLimitBeforeTLSHandshake(t *testing.T) {
	_, gate, _, _ := startTestProductionGate(t, 1, time.Second)
	first, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(first raw TCP) error = %v", err)
	}
	defer first.Close()

	second, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(second raw TCP) error = %v", err)
	}
	_ = second.SetReadDeadline(time.Now().Add(time.Second))
	var response [1]byte
	if _, err := second.Read(response[:]); err == nil {
		t.Fatal("超出连接上限的 TCP 连接没有被关闭")
	}
	_ = second.Close()

	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
}

// TestProdGateHandshakeTimeoutClosesConnection 验证客户端未发送首个密钥交换包时会按策略关闭并计数。
func TestProdGateHandshakeTimeoutClosesConnection(t *testing.T) {
	_, gate, _, clientTLS := startTestProductionGate(t, 2, 25*time.Millisecond)
	connection := dialProductionTestClient(t, gate, clientTLS)
	defer connection.Close()

	_, _, err := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocketHandshakeCloseCode) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocketHandshakeCloseCode)
	}
	waitForGateMetric(t, gate, "HandshakeTimeouts", func(metrics Metrics) uint64 {
		return metrics.HandshakeTimeouts
	}, 1)
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
}

// TestProdGateRejectsNonBinaryHandshake 验证应用层握手只接受二进制消息，并且不会创建 Agent。
func TestProdGateRejectsNonBinaryHandshake(t *testing.T) {
	_, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	connection := dialProductionTestClient(t, gate, clientTLS)
	defer connection.Close()

	if err := connection.WriteMessage(websocket.TextMessage, []byte("handshake must be binary")); err != nil {
		t.Fatalf("WriteMessage(text handshake) error = %v", err)
	}
	_, _, err := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseUnsupportedData) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseUnsupportedData)
	}
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	select {
	case agent := <-agents:
		t.Fatalf("invalid handshake created Agent %#v", agent)
	case <-time.After(30 * time.Millisecond):
	}
}

// TestProdGateRejectsInvalidHandshake 验证无法解析或不符合固定握手格式的消息会在 Agent 创建前关闭连接。
func TestProdGateRejectsInvalidHandshake(t *testing.T) {
	_, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	connection := dialProductionTestClient(t, gate, clientTLS)
	defer connection.Close()

	if err := connection.WriteMessage(websocket.BinaryMessage, []byte{0xff, 0x00}); err != nil {
		t.Fatalf("WriteMessage(invalid handshake) error = %v", err)
	}
	_, _, err := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseProtocolError) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseProtocolError)
	}
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	select {
	case agent := <-agents:
		t.Fatalf("invalid handshake created Agent %#v", agent)
	case <-time.After(30 * time.Millisecond):
	}
}

// TestProdGateRejectsOversizedHandshake 验证首个握手帧超过上限时返回消息过大并累计帧超限计数。
func TestProdGateRejectsOversizedHandshake(t *testing.T) {
	_, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	connection := dialProductionTestClient(t, gate, clientTLS)
	defer connection.Close()

	oversized := bytes.Repeat([]byte{'x'}, (1<<20)+1)
	if err := connection.WriteMessage(websocket.BinaryMessage, oversized); err != nil {
		t.Fatalf("WriteMessage(oversized handshake) error = %v", err)
	}
	_, _, err := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseMessageTooBig)
	}
	waitForGateMetric(t, gate, "FrameTooLarge", func(metrics Metrics) uint64 {
		return metrics.FrameTooLarge
	}, 1)
	select {
	case agent := <-agents:
		t.Fatalf("oversized handshake created Agent %#v", agent)
	case <-time.After(30 * time.Millisecond):
	}
}

// TestProdGateRejectsNonBinaryAuthenticatedFrame 验证认证完成后仍只接受二进制业务帧，并通知 Agent 连接断开。
func TestProdGateRejectsNonBinaryAuthenticatedFrame(t *testing.T) {
	_, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)

	if err := connection.WriteMessage(websocket.TextMessage, []byte("business frame must be binary")); err != nil {
		t.Fatalf("WriteMessage(text business frame) error = %v", err)
	}
	_, _, err := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseUnsupportedData) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseUnsupportedData)
	}
	select {
	case frame := <-agent.received:
		t.Fatalf("non-binary frame reached Agent as %x", frame)
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Agent 未收到认证连接断开通知")
	}
}

// TestProdGateClosesConnectionWhenAgentMailboxIsFull 验证 Agent 正在处理消息时 Mailbox 满载会关闭当前客户端连接并累计计数。
func TestProdGateClosesConnectionWhenAgentMailboxIsFull(t *testing.T) {
	release := make(chan struct{})
	agents := make(chan *blockingInboundAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 1,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &blockingInboundAgent{
			release:      release,
			started:      make(chan struct{}),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveBlockingInboundAgent(t, agents)

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 71})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(first frame) error = %v", err)
	}
	select {
	case <-agent.started:
	case <-time.After(time.Second):
		t.Fatal("Agent 未开始处理第一条客户端消息")
	}

	for sequence := uint64(2); sequence < 10; sequence++ {
		if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], sequence, requestBytes)); err != nil {
			break
		}
	}
	_, _, err = readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseInternalServerErr)
	}
	waitForGateMetric(t, gate, "InboundMailboxFull", func(metrics Metrics) uint64 {
		return metrics.InboundMailboxFull
	}, 1)
	close(release)
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Mailbox 满载后 Agent 未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateReportsOutboundQueueFull 验证 Agent 的 ClientSession 在出站队列不足时返回稳定错误并累计计数。
func TestProdGateReportsOutboundQueueFull(t *testing.T) {
	agents := make(chan *burstOutboundAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, queueEntryOverhead+1, func(client ClientSession) runtime.Service {
		agent := &burstOutboundAgent{
			client:       client,
			result:       make(chan error, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveBurstOutboundAgent(t, agents)

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 72})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(request frame) error = %v", err)
	}
	select {
	case err := <-agent.result:
		if !errors.Is(err, ErrClientQueueFull) {
			t.Fatalf("ClientSession.Send() error = %v, want %v", err, ErrClientQueueFull)
		}
	case <-time.After(time.Second):
		t.Fatal("Agent 未观察到出站队列拒绝")
	}
	waitForGateMetric(t, gate, "OutboundQueueFull", func(metrics Metrics) uint64 {
		return metrics.OutboundQueueFull
	}, 1)
	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("出站队列拒绝后 Agent 未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateBoundsSlowClientOutboundQueue 验证客户端不读取时写循环遇到背压仍只保留有界出站数据并返回队列满错误。
func TestProdGateBoundsSlowClientOutboundQueue(t *testing.T) {
	agents := make(chan *slowOutboundAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 128<<10, func(client ClientSession) runtime.Service {
		agent := &slowOutboundAgent{
			client:       client,
			result:       make(chan error, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	tcpConnection, ok := tcpConnectionFromWebSocket(connection)
	if !ok {
		t.Skip("测试客户端不是 TCP 连接")
	}
	if err := tcpConnection.SetReadBuffer(1024); err != nil {
		t.Fatalf("SetReadBuffer() error = %v", err)
	}
	agent := receiveSlowOutboundAgent(t, agents)

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 73})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(request frame) error = %v", err)
	}
	select {
	case err := <-agent.result:
		if !errors.Is(err, ErrClientQueueFull) {
			t.Fatalf("slow client Send() error = %v, want %v", err, ErrClientQueueFull)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("慢客户端未触发有界出站队列拒绝")
	}
	waitForGateMetric(t, gate, "OutboundQueueFull", func(metrics Metrics) uint64 {
		return metrics.OutboundQueueFull
	}, 1)
	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("慢客户端断开后 Agent 未收到通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateLongSlowClientBackpressure 在显式压力开关下验证慢客户端持续不读时出站队列有界，写超时关闭也不被误判为业务错误。
func TestProdGateLongSlowClientBackpressure(t *testing.T) {
	if os.Getenv("GSR_GATE_STRESS") != "1" {
		t.Skip("显式设置 GSR_GATE_STRESS=1 运行慢客户端压力验收")
	}
	stressDuration := 5 * time.Second
	if value := os.Getenv("GSR_GATE_SLOW_CLIENT_SECONDS"); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds <= 0 {
			t.Fatalf("GSR_GATE_SLOW_CLIENT_SECONDS = %q, want positive integer", value)
		}
		stressDuration = time.Duration(seconds) * time.Second
	}

	agents := make(chan *longSlowOutboundAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 3 * time.Second,
	}, 1, time.Second, 1<<20, 128<<10, func(client ClientSession) runtime.Service {
		agent := &longSlowOutboundAgent{
			client:       client,
			duration:     stressDuration,
			result:       make(chan longSlowOutboundResult, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	tcpConnection, ok := tcpConnectionFromWebSocket(connection)
	if !ok {
		t.Skip("测试客户端不是 TCP 连接")
	}
	if err := tcpConnection.SetReadBuffer(1024); err != nil {
		t.Fatalf("SetReadBuffer() error = %v", err)
	}
	agent := receiveLongSlowOutboundAgent(t, agents)

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(request frame) error = %v", err)
	}
	select {
	case result := <-agent.result:
		if result.err != nil {
			t.Fatalf("慢客户端持续发送遇到非预期错误: %v", result.err)
		}
		if result.sent == 0 || result.queueFull == 0 {
			t.Fatalf("slow client result = %+v, want sent and queueFull both positive", result)
		}
		t.Logf("slow_client_duration=%s sent=%d queue_full=%d closed=%t", stressDuration, result.sent, result.queueFull, result.closed)
	case <-time.After(stressDuration + time.Second):
		t.Fatal("慢客户端压力 Agent 未在预期时间结束")
	}
	if got := gate.Metrics().OutboundQueueFull; got == 0 {
		t.Fatal("OutboundQueueFull = 0, want positive count")
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close(client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("慢客户端压力结束后 Agent 未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateHandlesConcurrentConnections 验证多条连接并发发送时消息仍进入各自 Agent，并能在关闭后全部回收。
func TestProdGateHandlesConcurrentConnections(t *testing.T) {
	const (
		connectionCount  = 16
		messagesPerAgent = 32
	)
	agents := make(chan *loadTestAgent, connectionCount)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 64,
		ShutdownTimeout: 2 * time.Second,
	}, connectionCount, time.Second, 1<<20, 64<<10, func(ClientSession) runtime.Service {
		agent := &loadTestAgent{disconnected: make(chan struct{})}
		agents <- agent
		return agent
	})

	clients := make([]*websocket.Conn, 0, connectionCount)
	keys := make([][clientMACKeySize]byte, 0, connectionCount)
	testAgents := make([]*loadTestAgent, 0, connectionCount)
	for index := 0; index < connectionCount; index++ {
		connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
		clients = append(clients, connection)
		keys = append(keys, macKey)
		testAgents = append(testAgents, receiveLoadTestAgent(t, agents))
	}
	defer func() {
		for _, connection := range clients {
			_ = connection.Close()
		}
	}()

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	frames := make([][][]byte, connectionCount)
	for index := range clients {
		frames[index] = make([][]byte, messagesPerAgent)
		for messageIndex := range frames[index] {
			frames[index][messageIndex] = makeAuthenticatedTestFrame(t, keys[index][:], uint64(messageIndex+1), requestBytes)
		}
	}

	var writers sync.WaitGroup
	for index, connection := range clients {
		writers.Add(1)
		go func(index int, connection *websocket.Conn) {
			defer writers.Done()
			for messageIndex, frame := range frames[index] {
				if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
					t.Errorf("connection %d message %d WriteMessage() error = %v", index, messageIndex, err)
					return
				}
			}
		}(index, connection)
	}
	writers.Wait()
	for _, connection := range clients {
		if err := connection.Close(); err != nil {
			t.Fatalf("Close(client) error = %v", err)
		}
	}

	expectedMessages := uint64(connectionCount * messagesPerAgent)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		var processed uint64
		for _, agent := range testAgents {
			processed += agent.processed.Load()
		}
		if processed == expectedMessages {
			break
		}
		time.Sleep(time.Millisecond)
	}
	var processed uint64
	for _, agent := range testAgents {
		processed += agent.processed.Load()
	}
	if processed != expectedMessages {
		t.Fatalf("processed messages = %d, want %d", processed, expectedMessages)
	}
	for _, agent := range testAgents {
		select {
		case <-agent.disconnected:
		case <-time.After(time.Second):
			t.Fatal("并发连接关闭后 Agent 未全部收到断开通知")
		}
	}
	if got := gate.Metrics().AcceptedConnections; got != connectionCount {
		t.Fatalf("AcceptedConnections = %d, want %d", got, connectionCount)
	}
	if got := gate.Metrics().RejectedConnections; got != 0 {
		t.Fatalf("RejectedConnections = %d, want 0", got)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateMixedConnectionStress 验证多连接持续发送期间所有成功写入的请求均由 Agent 处理，且没有 Mailbox 满载或连接拒绝。
func TestProdGateMixedConnectionStress(t *testing.T) {
	if os.Getenv("GSR_GATE_STRESS") != "1" {
		t.Skip("显式设置 GSR_GATE_STRESS=1 运行生产 Gate 压力验收")
	}
	stressDuration := 10 * time.Second
	if value := os.Getenv("GSR_GATE_STRESS_SECONDS"); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds <= 0 {
			t.Fatalf("GSR_GATE_STRESS_SECONDS = %q, want positive integer", value)
		}
		stressDuration = time.Duration(seconds) * time.Second
	}

	const (
		connectionCount = 16
		sendInterval    = 10 * time.Millisecond
	)
	agents := make(chan *loadTestAgent, connectionCount)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 256,
		ShutdownTimeout: 3 * time.Second,
	}, connectionCount, time.Second, 1<<20, 256<<10, func(ClientSession) runtime.Service {
		agent := &loadTestAgent{disconnected: make(chan struct{})}
		agents <- agent
		return agent
	})

	clients := make([]*websocket.Conn, 0, connectionCount)
	keys := make([][clientMACKeySize]byte, 0, connectionCount)
	testAgents := make([]*loadTestAgent, 0, connectionCount)
	for index := 0; index < connectionCount; index++ {
		connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
		clients = append(clients, connection)
		keys = append(keys, macKey)
		testAgents = append(testAgents, receiveLoadTestAgent(t, agents))
	}
	defer func() {
		for _, connection := range clients {
			_ = connection.Close()
		}
	}()

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001})
	if err != nil {
		t.Fatalf("marshal stress Envelope: %v", err)
	}
	deadline := time.Now().Add(stressDuration)
	var sent atomic.Uint64
	var writers sync.WaitGroup
	writers.Add(connectionCount)
	for index, connection := range clients {
		go func(connection *websocket.Conn, macKey [clientMACKeySize]byte) {
			defer writers.Done()
			ticker := time.NewTicker(sendInterval)
			defer ticker.Stop()
			var sequence uint64 = 1
			for {
				select {
				case <-ticker.C:
					if time.Now().After(deadline) {
						return
					}
					frame := makeAuthenticatedTestFrame(t, macKey[:], sequence, requestBytes)
					if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
						return
					}
					sent.Add(1)
					sequence++
				}
			}
		}(connection, keys[index])
	}
	writers.Wait()

	accepted := sent.Load()
	for _, connection := range clients {
		_ = connection.Close()
	}
	processingDeadline := time.Now().Add(5 * time.Second)
	var processed uint64
	for time.Now().Before(processingDeadline) {
		processed = 0
		for _, agent := range testAgents {
			processed += agent.processed.Load()
		}
		if processed >= accepted {
			break
		}
		time.Sleep(time.Millisecond)
	}
	processed = 0
	for _, agent := range testAgents {
		processed += agent.processed.Load()
	}
	metrics := gate.Metrics()
	t.Logf("stress_duration=%s connections=%d sent=%d processed=%d inbound_mailbox_full=%d rejected=%d", stressDuration, connectionCount, accepted, processed, metrics.InboundMailboxFull, metrics.RejectedConnections)
	if accepted == 0 {
		t.Fatal("stress test sent no client messages")
	}
	if processed != accepted {
		t.Fatalf("processed messages = %d, successful client writes = %d", processed, accepted)
	}
	if metrics.InboundMailboxFull != 0 || metrics.RejectedConnections != 0 {
		t.Fatalf("Gate rejected traffic: inbound_mailbox_full=%d rejected=%d", metrics.InboundMailboxFull, metrics.RejectedConnections)
	}
	for _, agent := range testAgents {
		select {
		case <-agent.disconnected:
		case <-time.After(time.Second):
			t.Fatal("压力连接关闭后 Agent 未全部收到断开通知")
		}
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateFailurePhaseStress 验证重复坏握手和坏 MAC 不进入业务处理，且故障连接关闭后新连接仍能正常往返。
func TestProdGateFailurePhaseStress(t *testing.T) {
	if os.Getenv("GSR_GATE_STRESS") != "1" {
		t.Skip("显式设置 GSR_GATE_STRESS=1 运行失败阶段压力验收")
	}
	attempts := 32
	if value := os.Getenv("GSR_GATE_FAILURE_ATTEMPTS"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			t.Fatalf("GSR_GATE_FAILURE_ATTEMPTS = %q, want positive integer", value)
		}
		attempts = parsed
	}

	agents := make(chan *productionTestAgent, attempts+1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 16,
		ShutdownTimeout: 3 * time.Second,
	}, attempts, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	badHandshake := []byte{handshakeVersion, 0}
	for index := 0; index < attempts; index++ {
		connection := dialProductionTestClient(t, gate, clientTLS)
		if err := connection.WriteMessage(websocket.BinaryMessage, badHandshake); err != nil {
			t.Fatalf("WriteMessage(invalid handshake %d) error = %v", index, err)
		}
		_, _, err := readTestBinaryMessage(t, connection)
		if !websocket.IsCloseError(err, websocket.CloseProtocolError) {
			t.Fatalf("invalid handshake %d ReadMessage() error = %v, want WebSocket close %d", index, err, websocket.CloseProtocolError)
		}
		_ = connection.Close()
	}
	if got := gate.Metrics().RejectedConnections; got < uint64(attempts) {
		t.Fatalf("RejectedConnections after bad handshakes = %d, want at least %d", got, attempts)
	}

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 91})
	if err != nil {
		t.Fatalf("marshal bad MAC request Envelope: %v", err)
	}
	for index := 0; index < attempts; index++ {
		connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
		agent := receiveProductionAgent(t, agents)
		frame := makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)
		frame[len(frame)-1] ^= 0xff
		if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Fatalf("WriteMessage(bad MAC %d) error = %v", index, err)
		}
		_, _, err := readTestBinaryMessage(t, connection)
		if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
			t.Fatalf("bad MAC %d ReadMessage() error = %v, want WebSocket close %d", index, err, websocket.ClosePolicyViolation)
		}
		select {
		case delivered := <-agent.received:
			t.Fatalf("bad MAC %d reached Agent as %x", index, delivered)
		case <-time.After(10 * time.Millisecond):
		}
		select {
		case <-agent.disconnected:
		case <-time.After(time.Second):
			t.Fatalf("bad MAC %d Agent 未收到断开通知", index)
		}
		_ = connection.Close()
	}

	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(healthy recovery request) error = %v", err)
	}
	select {
	case received := <-agent.received:
		if !bytes.Equal(received, requestBytes) {
			t.Fatalf("recovery Agent received %x, want %x", received, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("healthy connection after failure stress did not reach Agent")
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(healthy recovery response) error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal healthy recovery response: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 91 {
		t.Fatalf("healthy recovery response = msg_id %d, request_id %d; want 1002, 91", response.GetMsgId(), response.GetRequestId())
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close(healthy recovery connection) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("healthy recovery connection close did not notify Agent")
	}

	metrics := gate.Metrics()
	t.Logf("failure_attempts=%d rejected=%d accepted=%d inbound_mailbox_full=%d", attempts, metrics.RejectedConnections, metrics.AcceptedConnections, metrics.InboundMailboxFull)
	if metrics.AcceptedConnections < uint64(attempts+1) {
		t.Fatalf("AcceptedConnections after recovery request = %d, want at least %d", metrics.AcceptedConnections, attempts+1)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateRejectsClientMessageForAgentWithoutHandler 验证没有客户端处理能力的 Agent 会在入站投递阶段被拒绝并正常断开。
func TestProdGateRejectsClientMessageForAgentWithoutHandler(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() { _ = logger.Close(context.Background()) })
	agents := make(chan *plainTestAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		Logger:          logger,
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(ClientSession) runtime.Service {
		agent := &plainTestAgent{disconnected: make(chan struct{})}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receivePlainTestAgent(t, agents)

	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 74, Payload: []byte("unsupported client message")})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(request frame) error = %v", err)
	}
	_, _, err = readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocket.CloseInternalServerErr)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("不支持客户端消息的 Agent 未收到断开通知")
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() error = %v", err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	foundDeliveryFailure := false
	for decoder.More() {
		var entry struct {
			Message string         `json:"message"`
			Fields  map[string]any `json:"fields"`
		}
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("decode log entry: %v", err)
		}
		if entry.Message == "Gate connection failure" && entry.Fields["category"] == "inbound_delivery" {
			foundDeliveryFailure = true
		}
	}
	if !foundDeliveryFailure {
		t.Fatal("missing inbound_delivery Gate failure log")
	}
}

// TestProdGateRuntimeCloseReleasesActiveClient 验证 Runtime 关闭时生产 Gate 会关闭已认证连接并等待自己的 I/O goroutine。
func TestProdGateRuntimeCloseReleasesActiveClient(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	_ = receiveProductionAgent(t, agents)

	closeResult := make(chan error, 1)
	go func() {
		closeResult <- local.Close()
	}()
	_, _, readErr := readTestBinaryMessage(t, connection)
	if readErr == nil {
		t.Fatal("Runtime.Close() 后客户端连接仍然可读")
	}
	select {
	case err := <-closeResult:
		if err != nil {
			t.Fatalf("Runtime.Close() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Runtime.Close() 未等待 Gate I/O goroutine 退出")
	}
}

// TestProdGateDisconnectsAgentCreatedAfterCallTimeout 验证 Gate Call 超时后迟到创建的 Agent 仍会收到断开事件并退出。
func TestProdGateDisconnectsAgentCreatedAfterCallTimeout(t *testing.T) {
	serverTLS, clientTLS := testTLSConfigs(t)
	local, err := runtime.NewRuntime(runtime.Config{
		MaxCallWait:     40 * time.Millisecond,
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	})
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

	agentStarted := make(chan struct{})
	releaseAgent := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseAgent) }) }
	defer release()
	agents := make(chan *productionTestAgent, 1)
	gate, err := NewGateService(local, Config{
		Address:               "127.0.0.1:0",
		TLSConfig:             serverTLS,
		MaxConnections:        1,
		MaxOutboundQueueBytes: 64 << 10,
		NewAgent: func(client ClientSession) runtime.Service {
			agent := &productionTestAgent{client: client, disconnected: make(chan struct{})}
			agents <- agent
			close(agentStarted)
			<-releaseAgent
			return agent
		},
	})
	if err != nil {
		t.Fatalf("NewGateService() error = %v", err)
	}
	if _, err := local.CreateService(context.Background(), gate); err != nil {
		t.Fatalf("CreateService(Gate) error = %v", err)
	}
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	select {
	case <-agentStarted:
	case <-time.After(time.Second):
		t.Fatal("Gate 未开始创建 Agent")
	}
	agent := receiveProductionAgent(t, agents)
	_, _, closeErr := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(closeErr, websocket.CloseInternalServerErr) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", closeErr, websocket.CloseInternalServerErr)
	}
	release()
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("迟到创建的 Agent 未收到断开通知")
	}
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections = %d, want 0 after timed-out connection", got)
	}
	if got := gate.Metrics().RejectedConnections; got != 1 {
		t.Fatalf("RejectedConnections = %d, want 1", got)
	}
}

// TestNewGateServiceRejectsInvalidTLSAndCapacity 验证生产 Gate 支持可选 TLS，并拒绝无效容量和 TLS 配置。
func TestNewGateServiceRejectsInvalidTLSAndCapacity(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})

	serverTLS, _ := testTLSConfigs(t)
	base := Config{
		Address:               "127.0.0.1:0",
		TLSConfig:             serverTLS,
		MaxConnections:        1,
		MaxOutboundQueueBytes: queueEntryOverhead,
		NewAgent:              func(ClientSession) runtime.Service { return &productionTestAgent{} },
	}
	for _, testCase := range []struct {
		name   string
		update func(*Config)
	}{
		{name: "missing certificate", update: func(config *Config) { config.TLSConfig = &tls.Config{} }},
		{name: "TLS below 1.2", update: func(config *Config) { config.TLSConfig.MinVersion = tls.VersionTLS10 }},
		{name: "empty connection limit", update: func(config *Config) { config.MaxConnections = 0 }},
		{name: "short outbound queue", update: func(config *Config) { config.MaxOutboundQueueBytes = queueEntryOverhead - 1 }},
		{name: "named port", update: func(config *Config) { config.Address = "127.0.0.1:https" }},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			config := base
			config.TLSConfig = base.TLSConfig.Clone()
			testCase.update(&config)
			if _, err := NewGateService(local, config); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewGateService() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
	noTLS := base
	noTLS.TLSConfig = nil
	if _, err := NewGateService(local, noTLS); err != nil {
		t.Fatalf("NewGateService() without TLS error = %v", err)
	}
}

// TestClientSessionBoundsOutboundQueue 验证生产 ClientSession 按帧字节和固定条目开销限制出站队列。
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

type productionTestAgent struct {
	client       ClientSession
	received     chan []byte
	disconnected chan struct{}
	once         sync.Once
}

func (*productionTestAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *productionTestAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected production test Agent command")
	}
	agent.once.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *productionTestAgent) HandleClientMessage(_ runtime.CommandContext, frame []byte) error {
	if agent.received != nil {
		agent.received <- append([]byte(nil), frame...)
	}
	var envelope commonv1.Envelope
	if err := proto.Unmarshal(frame, &envelope); err != nil {
		return err
	}
	if agent.client == nil {
		return nil
	}
	return agent.client.Send(&commonv1.Envelope{
		MsgId:     1002,
		RequestId: envelope.GetRequestId(),
		Payload:   envelope.GetPayload(),
	})
}

func (*productionTestAgent) OnStop(context.Context) error { return nil }

type blockingInboundAgent struct {
	release      <-chan struct{}
	started      chan struct{}
	disconnected chan struct{}
	startOnce    sync.Once
	stopOnce     sync.Once
}

func (*blockingInboundAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *blockingInboundAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected blocking inbound Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *blockingInboundAgent) HandleClientMessage(_ runtime.CommandContext, _ []byte) error {
	agent.startOnce.Do(func() { close(agent.started) })
	<-agent.release
	return nil
}

func (*blockingInboundAgent) OnStop(context.Context) error { return nil }

type burstOutboundAgent struct {
	client       ClientSession
	result       chan error
	disconnected chan struct{}
	resultOnce   sync.Once
	stopOnce     sync.Once
}

func (*burstOutboundAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *burstOutboundAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected burst outbound Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *burstOutboundAgent) HandleClientMessage(_ runtime.CommandContext, _ []byte) error {
	err := agent.client.Send(&commonv1.Envelope{
		MsgId:   1002,
		Payload: bytes.Repeat([]byte{'x'}, 128),
	})
	agent.resultOnce.Do(func() { agent.result <- err })
	return nil
}

func (*burstOutboundAgent) OnStop(context.Context) error { return nil }

type slowOutboundAgent struct {
	client       ClientSession
	result       chan error
	disconnected chan struct{}
	resultOnce   sync.Once
	stopOnce     sync.Once
}

func (*slowOutboundAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *slowOutboundAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected slow outbound Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *slowOutboundAgent) HandleClientMessage(_ runtime.CommandContext, _ []byte) error {
	payload := bytes.Repeat([]byte{'y'}, 64<<10)
	for attempt := 0; attempt < 64; attempt++ {
		err := agent.client.Send(&commonv1.Envelope{MsgId: 1002, Payload: payload})
		if err != nil {
			agent.resultOnce.Do(func() { agent.result <- err })
			return nil
		}
	}
	agent.resultOnce.Do(func() { agent.result <- errors.New("slow client queue did not fill") })
	return nil
}

func (*slowOutboundAgent) OnStop(context.Context) error { return nil }

type longSlowOutboundResult struct {
	sent      uint64
	queueFull uint64
	closed    bool
	err       error
}

type longSlowOutboundAgent struct {
	client       ClientSession
	duration     time.Duration
	result       chan longSlowOutboundResult
	disconnected chan struct{}
	stopOnce     sync.Once
}

func (*longSlowOutboundAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *longSlowOutboundAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected long slow outbound Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *longSlowOutboundAgent) HandleClientMessage(_ runtime.CommandContext, _ []byte) error {
	deadline := time.Now().Add(agent.duration)
	payload := bytes.Repeat([]byte{'z'}, 64<<10)
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	var result longSlowOutboundResult
	for {
		select {
		case <-ticker.C:
			if time.Now().After(deadline) {
				agent.result <- result
				return nil
			}
			err := agent.client.Send(&commonv1.Envelope{MsgId: 1002, Payload: payload})
			switch {
			case err == nil:
				result.sent++
			case errors.Is(err, ErrClientQueueFull):
				result.queueFull++
			case errors.Is(err, ErrClientSessionClosed):
				result.closed = true
				agent.result <- result
				return nil
			default:
				result.err = err
				agent.result <- result
				return nil
			}
		}
	}
}

func (*longSlowOutboundAgent) OnStop(context.Context) error { return nil }

type loadTestAgent struct {
	processed    atomic.Uint64
	disconnected chan struct{}
	stopOnce     sync.Once
}

func (*loadTestAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *loadTestAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected load test Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (agent *loadTestAgent) HandleClientMessage(_ runtime.CommandContext, _ []byte) error {
	agent.processed.Add(1)
	return nil
}

func (*loadTestAgent) OnStop(context.Context) error { return nil }

type plainTestAgent struct {
	disconnected chan struct{}
	stopOnce     sync.Once
}

func (*plainTestAgent) OnStart(context.Context, runtime.ServiceContext) error { return nil }

func (agent *plainTestAgent) Handle(commandContext runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != ClientDisconnectedCommand {
		return nil, errors.New("unexpected plain test Agent command")
	}
	agent.stopOnce.Do(func() { close(agent.disconnected) })
	commandContext.Exit()
	return nil, nil
}

func (*plainTestAgent) OnStop(context.Context) error { return nil }

func startTestProductionGate(
	t *testing.T,
	maxConnections int,
	handshakeTimeout time.Duration,
	loggers ...*logging.Logger,
) (*runtime.Runtime, *GateService, <-chan *productionTestAgent, *tls.Config) {
	t.Helper()
	var logger *logging.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	agents := make(chan *productionTestAgent, maxConnections+1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		Logger:          logger,
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, maxConnections, handshakeTimeout, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	return local, gate, agents, clientTLS
}

// startConfiguredProductionGate 使用给定 Runtime 与容量配置启动测试用生产 Gate。
func startConfiguredProductionGate(
	t *testing.T,
	runtimeConfig runtime.Config,
	maxConnections int,
	handshakeTimeout time.Duration,
	maxFrameBytes int,
	maxOutboundQueueBytes int,
	newAgent func(ClientSession) runtime.Service,
) (*runtime.Runtime, *GateService, *tls.Config) {
	t.Helper()
	serverTLS, clientTLS := testTLSConfigs(t)
	local, gate := startConfiguredProductionGateWithTLS(t, runtimeConfig, maxConnections, handshakeTimeout, maxFrameBytes, maxOutboundQueueBytes, serverTLS, newAgent)
	return local, gate, clientTLS
}

// startConfiguredHTTPProductionGate 启动由 Ingress 终止 TLS、Pod 使用普通 HTTP WebSocket 的测试 Gate。
func startConfiguredHTTPProductionGate(
	t *testing.T,
	runtimeConfig runtime.Config,
	maxConnections int,
	handshakeTimeout time.Duration,
	maxFrameBytes int,
	maxOutboundQueueBytes int,
	newAgent func(ClientSession) runtime.Service,
) (*runtime.Runtime, *GateService) {
	t.Helper()
	return startConfiguredProductionGateWithTLS(t, runtimeConfig, maxConnections, handshakeTimeout, maxFrameBytes, maxOutboundQueueBytes, nil, newAgent)
}

// startConfiguredProductionGateWithTLS 使用给定 TLS 配置启动测试用生产 Gate。
func startConfiguredProductionGateWithTLS(
	t *testing.T,
	runtimeConfig runtime.Config,
	maxConnections int,
	handshakeTimeout time.Duration,
	maxFrameBytes int,
	maxOutboundQueueBytes int,
	serverTLS *tls.Config,
	newAgent func(ClientSession) runtime.Service,
) (*runtime.Runtime, *GateService) {
	t.Helper()
	local, err := runtime.NewRuntime(runtimeConfig)
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
		Address:               "127.0.0.1:0",
		TLSConfig:             serverTLS,
		MaxConnections:        maxConnections,
		HandshakeTimeout:      handshakeTimeout,
		MaxFrameBytes:         maxFrameBytes,
		MaxOutboundQueueBytes: maxOutboundQueueBytes,
		NewAgent:              newAgent,
	})
	if err != nil {
		t.Fatalf("NewGateService() error = %v", err)
	}
	if _, err := local.CreateService(context.Background(), gate); err != nil {
		t.Fatalf("CreateService(Gate) error = %v", err)
	}
	return local, gate
}

func testTLSConfigs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	certificateServer := httptest.NewTLSServer(http.NotFoundHandler())
	t.Cleanup(certificateServer.Close)
	certificate := certificateServer.TLS.Certificates[0]
	leaf, err := x509.ParseCertificate(certificate.Certificate[0])
	if err != nil {
		t.Fatalf("ParseCertificate() error = %v", err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	serverName := ""
	if len(leaf.DNSNames) > 0 {
		serverName = leaf.DNSNames[0]
	} else if len(leaf.IPAddresses) > 0 {
		serverName = leaf.IPAddresses[0].String()
	} else {
		t.Fatal("httptest certificate has no usable subject alternative name")
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}, &tls.Config{
		RootCAs:    roots,
		ServerName: serverName,
		MinVersion: tls.VersionTLS12,
	}
}

func dialProductionTestClient(t *testing.T, gate *GateService, clientTLS *tls.Config) *websocket.Conn {
	t.Helper()
	endpoint := url.URL{Scheme: "wss", Host: gate.Addr().String(), Path: "/ws"}
	dialer := websocket.Dialer{TLSClientConfig: clientTLS, HandshakeTimeout: time.Second}
	connection, response, err := dialer.Dial(endpoint.String(), nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial(%s) error = %v", endpoint.String(), err)
	}
	return connection
}

func dialPlainTestClient(t *testing.T, gate *GateService) *websocket.Conn {
	t.Helper()
	endpoint := url.URL{Scheme: "ws", Host: gate.Addr().String(), Path: "/ws"}
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	connection, response, err := dialer.Dial(endpoint.String(), nil)
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
	if err != nil {
		t.Fatalf("Dial(%s) error = %v", endpoint.String(), err)
	}
	return connection
}

func tcpConnectionFromWebSocket(connection *websocket.Conn) (*net.TCPConn, bool) {
	netConnection := connection.NetConn()
	if tlsConnection, ok := netConnection.(*tls.Conn); ok {
		netConnection = tlsConnection.NetConn()
	}
	tcpConnection, ok := netConnection.(*net.TCPConn)
	return tcpConnection, ok
}

func dialAuthenticatedTestClient(t *testing.T, gate *GateService, clientTLS *tls.Config) (*websocket.Conn, [clientMACKeySize]byte) {
	t.Helper()
	connection := dialProductionTestClient(t, gate, clientTLS)
	return connection, authenticateTestClient(t, connection)
}

func authenticateTestClient(t *testing.T, connection *websocket.Conn) [clientMACKeySize]byte {
	t.Helper()
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	clientPublicKey := privateKey.PublicKey().Bytes()
	request := marshalHandshakeRequest(t, clientPublicKey)
	if err := connection.WriteMessage(websocket.BinaryMessage, request); err != nil {
		t.Fatalf("WriteMessage(handshake) error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(handshake response) error = %v", err)
	}
	if len(responseBytes) != handshakeFrameSize || responseBytes[0] != handshakeVersion {
		t.Fatalf("handshake response = %x, want version %d and %d bytes", responseBytes, handshakeVersion, handshakeFrameSize)
	}
	serverPublicKeyBytes := responseBytes[1:]
	serverPublicKey, err := ecdh.X25519().NewPublicKey(serverPublicKeyBytes)
	if err != nil {
		t.Fatalf("NewPublicKey() error = %v", err)
	}
	sharedSecret, err := privateKey.ECDH(serverPublicKey)
	if err != nil {
		t.Fatalf("ECDH() error = %v", err)
	}
	macKey, err := deriveClientMACKey(sharedSecret, clientPublicKey, serverPublicKeyBytes)
	if err != nil {
		t.Fatalf("deriveClientMACKey() error = %v", err)
	}
	return macKey
}

func makeAuthenticatedTestFrame(t *testing.T, macKey []byte, sequence uint64, envelope []byte) []byte {
	t.Helper()
	frame := make([]byte, clientSequenceSize, clientSequenceSize+len(envelope)+clientMACSize)
	binary.BigEndian.PutUint64(frame, sequence)
	frame = append(frame, envelope...)
	mac := hmacSHA256(macKey, frame)
	return append(frame, mac[:clientMACSize]...)
}

func readTestBinaryMessage(t *testing.T, connection *websocket.Conn) (int, []byte, error) {
	t.Helper()
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	messageType, message, err := connection.ReadMessage()
	_ = connection.SetReadDeadline(time.Time{})
	return messageType, message, err
}

func receiveProductionAgent(t *testing.T, agents <-chan *productionTestAgent) *productionTestAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建连接 Agent")
		return nil
	}
}

func receiveBlockingInboundAgent(t *testing.T, agents <-chan *blockingInboundAgent) *blockingInboundAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建 blocking inbound Agent")
		return nil
	}
}

func receiveBurstOutboundAgent(t *testing.T, agents <-chan *burstOutboundAgent) *burstOutboundAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建 burst outbound Agent")
		return nil
	}
}

func receiveSlowOutboundAgent(t *testing.T, agents <-chan *slowOutboundAgent) *slowOutboundAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建 slow outbound Agent")
		return nil
	}
}

func receiveLongSlowOutboundAgent(t *testing.T, agents <-chan *longSlowOutboundAgent) *longSlowOutboundAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建 long slow outbound Agent")
		return nil
	}
}

func receiveLoadTestAgent(t *testing.T, agents <-chan *loadTestAgent) *loadTestAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建 load test Agent")
		return nil
	}
}

func receivePlainTestAgent(t *testing.T, agents <-chan *plainTestAgent) *plainTestAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建 plain test Agent")
		return nil
	}
}

// waitForGateMetric 等待连接处理 goroutine 将一个 Gate 计数写入可观察快照。
func waitForGateMetric(t *testing.T, gate *GateService, name string, read func(Metrics) uint64, want uint64) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := read(gate.Metrics()); got >= want {
			if got != want {
				t.Fatalf("%s = %d, want %d", name, got, want)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("%s did not reach %d", name, want)
}
