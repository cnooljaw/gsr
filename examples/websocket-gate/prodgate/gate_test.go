package prodgate

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
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

// TestProdGateAllowsAnyOrigin 验证跨主机 Origin 不阻止 WebSocket 升级和应用握手。
func TestProdGateAllowsAnyOrigin(t *testing.T) {
	_, gate := startConfiguredHTTPProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 2, time.Second, 1<<20, 64<<10, func(ClientSession) runtime.Service {
		return &plainTestAgent{disconnected: make(chan struct{})}
	})
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
	authenticateTestClient(t, connection)
}

// TestProdGateClosesIdleNonWebSocketHTTPConnections 验证普通 HTTP 请求的 keep-alive 空闲连接会超时释放 Gate 容量。
func TestProdGateClosesIdleNonWebSocketHTTPConnections(t *testing.T) {
	_, gate := startConfiguredHTTPProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(ClientSession) runtime.Service {
		return &plainTestAgent{disconnected: make(chan struct{})}
	})

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
	if response.StatusCode != http.StatusNotFound || response.Close {
		t.Fatalf("HTTP response = status %d, close %t; want 404 with keep-alive", response.StatusCode, response.Close)
	}
	if _, err := io.Copy(io.Discard, response.Body); err != nil {
		t.Fatalf("drain HTTP response body: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("Close(HTTP response body) error = %v", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(defaultHTTPIdleTimeout + time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	if _, err := reader.ReadByte(); !errors.Is(err, io.EOF) {
		t.Fatalf("idle HTTP connection read error = %v, want EOF after the idle timeout", err)
	}
}

// TestProdGateReadHeaderTimeoutReleasesConnectionSlot 验证未完成 HTTP 请求头的连接超时后归还 TCP 名额。
func TestProdGateReadHeaderTimeoutReleasesConnectionSlot(t *testing.T) {
	local, gate := startConfiguredHTTPProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(ClientSession) runtime.Service {
		return &plainTestAgent{disconnected: make(chan struct{})}
	})
	partial, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(partial HTTP) error = %v", err)
	}
	defer partial.Close()
	started := time.Now()
	if _, err := io.WriteString(partial, "GET /ws HTTP/1.1\r\nHost: "+gate.Addr().String()+"\r\n"); err != nil {
		t.Fatalf("Write(partial HTTP header) error = %v", err)
	}

	extra, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(over-capacity HTTP) error = %v", err)
	}
	defer extra.Close()
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	if err := partial.SetReadDeadline(started.Add(defaultReadHeaderTimeout + 2*time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(partial HTTP) error = %v", err)
	}
	if _, err := io.Copy(io.Discard, partial); err != nil {
		t.Fatalf("partial HTTP connection did not close after header timeout: %v", err)
	}
	if elapsed := time.Since(started); elapsed < defaultReadHeaderTimeout-time.Second {
		t.Fatalf("partial HTTP connection closed after %v, before header timeout", elapsed)
	}

	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   time.Second,
	}
	recoveryDeadline := time.Now().Add(time.Second)
	for {
		response, err := client.Get("http://" + gate.Addr().String() + "/not-ws")
		if err == nil {
			if err := response.Body.Close(); err != nil {
				t.Fatalf("Close(recovered HTTP body) error = %v", err)
			}
			if response.StatusCode != http.StatusNotFound {
				t.Fatalf("HTTP status after header timeout = %d, want 404", response.StatusCode)
			}
			break
		}
		if time.Now().After(recoveryDeadline) {
			t.Fatalf("HTTP request did not recover after header timeout: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateStopClosesPartialHTTPHeader 验证停机立即关闭未完成请求头的连接，不等待请求头超时。
func TestProdGateStopClosesPartialHTTPHeader(t *testing.T) {
	local, gate := startConfiguredHTTPProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(ClientSession) runtime.Service {
		return &plainTestAgent{disconnected: make(chan struct{})}
	})
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

// TestProdGateConcurrentBadMACDoesNotBlockHealthyConnection 验证多条坏 MAC 连接并发失败时健康连接仍能完成请求。
func TestProdGateConcurrentBadMACDoesNotBlockHealthyConnection(t *testing.T) {
	const (
		badClientCount  = 8
		healthyMessages = 16
		mailboxCapacity = 64
		connectionLimit = badClientCount + 1
	)
	agents := make(chan *productionTestAgent, connectionLimit)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: mailboxCapacity,
		ShutdownTimeout: 2 * time.Second,
	}, connectionLimit, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, healthyMessages),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})

	healthyConnection, healthyMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer healthyConnection.Close()
	healthyAgent := receiveProductionAgent(t, agents)
	badConnections := make([]*websocket.Conn, badClientCount)
	badMACKeys := make([][clientMACKeySize]byte, badClientCount)
	badAgents := make([]*productionTestAgent, badClientCount)
	for index := range badConnections {
		connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
		badConnections[index] = connection
		badMACKeys[index] = macKey
		badAgents[index] = receiveProductionAgent(t, agents)
	}
	defer func() {
		for _, connection := range badConnections {
			_ = connection.Close()
		}
	}()

	badRequest, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 200})
	if err != nil {
		t.Fatalf("marshal bad MAC request Envelope: %v", err)
	}
	start := make(chan struct{})
	var failures sync.WaitGroup
	for index, connection := range badConnections {
		failures.Add(1)
		go func(index int, connection *websocket.Conn) {
			defer failures.Done()
			<-start
			frame := makeAuthenticatedTestFrame(t, badMACKeys[index][:], 1, badRequest)
			frame[len(frame)-1] ^= 0xff
			if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				t.Errorf("bad MAC connection %d WriteMessage() error = %v", index, err)
				return
			}
			_, _, err := readTestBinaryMessage(t, connection)
			if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
				t.Errorf("bad MAC connection %d ReadMessage() error = %v, want close %d", index, err, websocket.ClosePolicyViolation)
			}
		}(index, connection)
	}
	close(start)

	for index := 0; index < healthyMessages; index++ {
		requestID := uint32(300 + index)
		requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: requestID})
		if err != nil {
			t.Fatalf("marshal healthy request %d: %v", index, err)
		}
		frame := makeAuthenticatedTestFrame(t, healthyMACKey[:], uint64(index+1), requestBytes)
		if err := healthyConnection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Fatalf("healthy connection WriteMessage(%d) error = %v", index, err)
		}
		_, responseBytes, err := readTestBinaryMessage(t, healthyConnection)
		if err != nil {
			t.Fatalf("healthy connection ReadMessage(%d) error = %v", index, err)
		}
		var response commonv1.Envelope
		if err := proto.Unmarshal(responseBytes, &response); err != nil {
			t.Fatalf("unmarshal healthy response %d: %v", index, err)
		}
		if response.GetMsgId() != 1002 || response.GetRequestId() != requestID {
			t.Fatalf("healthy response %d = msg_id %d, request_id %d; want 1002, %d",
				index, response.GetMsgId(), response.GetRequestId(), requestID)
		}
	}
	failures.Wait()

	for index, agent := range badAgents {
		select {
		case <-agent.disconnected:
		case <-time.After(time.Second):
			t.Fatalf("bad MAC connection %d Agent 未收到断开通知", index)
		}
		select {
		case delivered := <-agent.received:
			t.Fatalf("bad MAC connection %d reached Agent as %x", index, delivered)
		default:
		}
	}
	if got := len(healthyAgent.received); got != healthyMessages {
		t.Fatalf("healthy Agent processed %d messages, want %d", got, healthyMessages)
	}
	if got := gate.Metrics().InboundMailboxFull; got != 0 {
		t.Fatalf("InboundMailboxFull = %d, want 0", got)
	}
	if err := healthyConnection.Close(); err != nil {
		t.Fatalf("Close(healthy client) error = %v", err)
	}
	select {
	case <-healthyAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("健康 Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateConcurrentBadHandshakeDoesNotBlockHealthyConnection 验证多条无效 X25519 握手并发失败时健康连接仍能完成请求。
func TestProdGateConcurrentBadHandshakeDoesNotBlockHealthyConnection(t *testing.T) {
	const (
		badClientCount  = 8
		healthyMessages = 8
		connectionLimit = badClientCount + 1
	)
	agents := make(chan *productionTestAgent, connectionLimit)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 32,
		ShutdownTimeout: 2 * time.Second,
	}, connectionLimit, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, healthyMessages),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})

	healthyConnection, healthyMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer healthyConnection.Close()
	healthyAgent := receiveProductionAgent(t, agents)
	badConnections := make([]*websocket.Conn, badClientCount)
	for index := range badConnections {
		badConnections[index] = dialProductionTestClient(t, gate, clientTLS)
		if err := badConnections[index].SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			t.Fatalf("SetReadDeadline(bad handshake %d) error = %v", index, err)
		}
	}
	defer func() {
		for _, connection := range badConnections {
			_ = connection.Close()
		}
	}()

	invalidHandshake := make([]byte, handshakeFrameSize)
	invalidHandshake[0] = handshakeVersion
	start := make(chan struct{})
	var failures sync.WaitGroup
	for index, connection := range badConnections {
		failures.Add(1)
		go func(index int, connection *websocket.Conn) {
			defer failures.Done()
			<-start
			if err := connection.WriteMessage(websocket.BinaryMessage, invalidHandshake); err != nil {
				t.Errorf("bad handshake connection %d WriteMessage() error = %v", index, err)
				return
			}
			_, _, err := connection.ReadMessage()
			if !websocket.IsCloseError(err, websocket.CloseProtocolError) {
				t.Errorf("bad handshake connection %d ReadMessage() error = %v, want close %d", index, err, websocket.CloseProtocolError)
			}
		}(index, connection)
	}
	close(start)

	for index := 0; index < healthyMessages; index++ {
		requestID := uint32(600 + index)
		requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: requestID})
		if err != nil {
			t.Fatalf("marshal healthy request %d: %v", index, err)
		}
		frame := makeAuthenticatedTestFrame(t, healthyMACKey[:], uint64(index+1), requestBytes)
		if err := healthyConnection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Fatalf("healthy connection WriteMessage(%d) error = %v", index, err)
		}
		_, responseBytes, err := readTestBinaryMessage(t, healthyConnection)
		if err != nil {
			t.Fatalf("healthy connection ReadMessage(%d) error = %v", index, err)
		}
		var response commonv1.Envelope
		if err := proto.Unmarshal(responseBytes, &response); err != nil {
			t.Fatalf("unmarshal healthy response %d: %v", index, err)
		}
		if response.GetMsgId() != 1002 || response.GetRequestId() != requestID {
			t.Fatalf("healthy response %d = msg_id %d, request_id %d; want 1002, %d",
				index, response.GetMsgId(), response.GetRequestId(), requestID)
		}
	}
	failures.Wait()
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, badClientCount)
	if got := len(agents); got != 0 {
		t.Fatalf("invalid handshakes created %d extra Agent services", got)
	}
	if got := len(healthyAgent.received); got != healthyMessages {
		t.Fatalf("healthy Agent processed %d messages, want %d", got, healthyMessages)
	}
	if got := gate.Metrics().InboundMailboxFull; got != 0 {
		t.Fatalf("InboundMailboxFull = %d, want 0", got)
	}
	if err := healthyConnection.Close(); err != nil {
		t.Fatalf("Close(healthy client) error = %v", err)
	}
	select {
	case <-healthyAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("健康 Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateRecoversFromAgentConstructorPanic 验证 Agent 构造器 panic 只拒绝当前连接，Gate 仍能接受后续连接。
func TestProdGateRecoversFromAgentConstructorPanic(t *testing.T) {
	agents := make(chan *productionTestAgent, 1)
	var constructorCalls atomic.Int32
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 2, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		if constructorCalls.Add(1) == 1 {
			panic("agent initialization failed")
		}
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})

	failedConnection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	_, _, err := readTestBinaryMessage(t, failedConnection)
	if !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("Agent constructor panic close error = %v, want close %d", err, websocket.CloseInternalServerErr)
	}
	// 服务端已发关闭帧，TLS close_notify 可能因连接关闭返回 broken pipe，不影响本用例断言。
	_ = failedConnection.Close()

	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 401})
	if err != nil {
		t.Fatalf("marshal healthy request: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("healthy connection WriteMessage() error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("healthy connection ReadMessage() error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal healthy response: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 401 {
		t.Fatalf("healthy response = msg_id %d, request_id %d; want 1002, 401", response.GetMsgId(), response.GetRequestId())
	}
	if got := gate.Metrics().RejectedConnections; got != 1 {
		t.Fatalf("RejectedConnections = %d, want 1", got)
	}
	if got := gate.Metrics().AcceptedConnections; got != 1 {
		t.Fatalf("AcceptedConnections = %d, want 1", got)
	}
	if got := constructorCalls.Load(); got != 2 {
		t.Fatalf("Agent constructor called %d times, want 2", got)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close(healthy client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("healthy Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateRecoversFromAgentStartFailure 验证 Agent OnStart 失败会回滚该实例并拒绝当前连接，Gate 仍能服务后续连接。
func TestProdGateRecoversFromAgentStartFailure(t *testing.T) {
	agents := make(chan *productionTestAgent, 1)
	failedAgentStopped := make(chan struct{})
	var constructorCalls atomic.Int32
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 2, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		if constructorCalls.Add(1) == 1 {
			return &startFailingProductionTestAgent{
				productionTestAgent: &productionTestAgent{},
				stopCalled:          failedAgentStopped,
			}
		}
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})

	failedConnection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	_, _, err := readTestBinaryMessage(t, failedConnection)
	if !websocket.IsCloseError(err, websocket.CloseInternalServerErr) {
		t.Fatalf("Agent OnStart failure close error = %v, want close %d", err, websocket.CloseInternalServerErr)
	}
	select {
	case <-failedAgentStopped:
	case <-time.After(time.Second):
		t.Fatal("OnStart 失败的 Agent 未执行 OnStop 清理")
	}
	// 服务端已发关闭帧，TLS close_notify 可能因连接关闭返回 broken pipe，不影响本用例断言。
	_ = failedConnection.Close()

	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 402})
	if err != nil {
		t.Fatalf("marshal healthy request: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("healthy connection WriteMessage() error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("healthy connection ReadMessage() error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal healthy response: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 402 {
		t.Fatalf("healthy response = msg_id %d, request_id %d; want 1002, 402", response.GetMsgId(), response.GetRequestId())
	}
	if got := gate.Metrics().RejectedConnections; got != 1 {
		t.Fatalf("RejectedConnections = %d, want 1", got)
	}
	if got := gate.Metrics().AcceptedConnections; got != 1 {
		t.Fatalf("AcceptedConnections = %d, want 1", got)
	}
	if got := constructorCalls.Load(); got != 2 {
		t.Fatalf("Agent constructor called %d times, want 2", got)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close(healthy client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("healthy Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateRejectsCrossConnectionReplayBeforeDelivery 验证连接 A 的认证帧不能在连接 B 重放，且新连接仍可正常请求。
func TestProdGateRejectsCrossConnectionReplayBeforeDelivery(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 3, time.Second)
	sourceConnection, sourceMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer sourceConnection.Close()
	sourceAgent := receiveProductionAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 96})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	frameFromSourceConnection := makeAuthenticatedTestFrame(t, sourceMACKey[:], 1, requestBytes)
	if err := sourceConnection.WriteMessage(websocket.BinaryMessage, frameFromSourceConnection); err != nil {
		t.Fatalf("WriteMessage(source authenticated frame) error = %v", err)
	}
	select {
	case delivered := <-sourceAgent.received:
		if !bytes.Equal(delivered, requestBytes) {
			t.Fatalf("source Agent 收到 Envelope = %x，want %x", delivered, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("Gate 未投递源连接的认证请求")
	}
	_, responseBytes, err := readTestBinaryMessage(t, sourceConnection)
	if err != nil {
		t.Fatalf("ReadMessage(source response) error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal source response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 96 {
		t.Fatalf("source response = msg_id %d, request_id %d; want 1002, 96", response.GetMsgId(), response.GetRequestId())
	}

	targetConnection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer targetConnection.Close()
	targetAgent := receiveProductionAgent(t, agents)
	if err := targetConnection.WriteMessage(websocket.BinaryMessage, frameFromSourceConnection); err != nil {
		t.Fatalf("WriteMessage(frame replayed across connections) error = %v", err)
	}
	_, _, err = readTestBinaryMessage(t, targetConnection)
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("ReadMessage(cross-connection replay) error = %v, want WebSocket close %d", err, websocket.ClosePolicyViolation)
	}
	select {
	case replayed := <-targetAgent.received:
		t.Fatalf("cross-connection replay reached target Agent as %x", replayed)
	default:
	}
	select {
	case <-targetAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("跨连接重放被拒绝后目标 Agent 未收到断开通知")
	}

	recoveryConnection, recoveryMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer recoveryConnection.Close()
	recoveryAgent := receiveProductionAgent(t, agents)
	if err := recoveryConnection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, recoveryMACKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(recovery frame) error = %v", err)
	}
	select {
	case delivered := <-recoveryAgent.received:
		if !bytes.Equal(delivered, requestBytes) {
			t.Fatalf("recovery Agent 收到 Envelope = %x，want %x", delivered, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("拒绝跨连接重放后，新连接未接收序号一请求")
	}
	_, responseBytes, err = readTestBinaryMessage(t, recoveryConnection)
	if err != nil {
		t.Fatalf("ReadMessage(recovery response) error = %v", err)
	}
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal recovery response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 96 {
		t.Fatalf("recovery response = msg_id %d, request_id %d; want 1002, 96", response.GetMsgId(), response.GetRequestId())
	}
	if err := recoveryConnection.Close(); err != nil {
		t.Fatalf("Close(recovery client) error = %v", err)
	}
	select {
	case <-recoveryAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("新连接断开后 Agent 未收到通知")
	}
	if err := sourceConnection.Close(); err != nil {
		t.Fatalf("Close(source client) error = %v", err)
	}
	select {
	case <-sourceAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("源连接断开后 Agent 未收到通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateRejectsSameConnectionReplayBeforeDelivery 验证已处理过的认证帧不能在同一连接再次投递给 Agent。
func TestProdGateRejectsSameConnectionReplayBeforeDelivery(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 1, time.Second)
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 98})
	if err != nil {
		t.Fatalf("marshal request Envelope: %v", err)
	}
	frame := makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)
	if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("WriteMessage(first authenticated frame) error = %v", err)
	}
	select {
	case delivered := <-agent.received:
		if !bytes.Equal(delivered, requestBytes) {
			t.Fatalf("Agent 收到 Envelope = %x，want %x", delivered, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("Gate 未投递首次认证请求")
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(first response) error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal first response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 98 {
		t.Fatalf("first response = msg_id %d, request_id %d; want 1002, 98", response.GetMsgId(), response.GetRequestId())
	}

	if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
		t.Fatalf("WriteMessage(replayed authenticated frame) error = %v", err)
	}
	_, _, err = readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.ClosePolicyViolation) {
		t.Fatalf("ReadMessage(replayed frame) error = %v, want WebSocket close %d", err, websocket.ClosePolicyViolation)
	}
	select {
	case duplicate := <-agent.received:
		t.Fatalf("replayed frame reached Agent as %x", duplicate)
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("同连接重放被拒绝后 Agent 未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
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
	if bytes.Contains(output.Bytes(), []byte("secret payload")) {
		t.Fatal("Gate failure log contains unauthenticated client payload")
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
	}
	if !foundFailure {
		t.Fatal("missing Gate connection failure log")
	}
}

// TestProdGateLogsPreHandshakeCapacityRejection 验证握手前的 TCP 容量拒绝进入共享日志并绑定 Gate 来源。
func TestProdGateLogsPreHandshakeCapacityRejection(t *testing.T) {
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
	local, gate, _, _ := startTestProductionGate(t, 1, time.Second, logger)
	first, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(first) error = %v", err)
	}
	defer first.Close()
	second, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(second) error = %v", err)
	}
	defer second.Close()
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}
	var matches int
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	for decoder.More() {
		var entry struct {
			Service uint64         `json:"service"`
			Message string         `json:"message"`
			Fields  map[string]any `json:"fields"`
		}
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("decode log entry: %v", err)
		}
		if entry.Message == "Gate connection rejected" && entry.Fields["reason"] == "capacity" {
			matches++
			if entry.Service != uint64(gate.serviceID) {
				t.Fatalf("capacity rejection service = %d, want %d", entry.Service, gate.serviceID)
			}
		}
	}
	if matches != 1 {
		t.Fatalf("capacity rejection logs = %d, want 1", matches)
	}
}

// TestProdGateCapacityRejectionSurvivesClosedLogger 验证日志入口不可用时连接上限、拒绝计数和停机仍正常。
func TestProdGateCapacityRejectionSurvivesClosedLogger(t *testing.T) {
	logger, err := logging.New(logging.Config{Writer: io.Discard})
	if err != nil {
		t.Fatalf("logging.New() error = %v", err)
	}
	t.Cleanup(func() {
		if err := logger.Close(context.Background()); err != nil {
			t.Errorf("Logger.Close() error = %v", err)
		}
	})
	local, gate, _, _ := startTestProductionGate(t, 1, time.Second, logger)
	if err := logger.Close(context.Background()); err != nil {
		t.Fatalf("Logger.Close() error = %v", err)
	}
	first, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(first) error = %v", err)
	}
	defer first.Close()
	second, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(second) error = %v", err)
	}
	defer second.Close()
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateCapacityRejectionsKeepExistingClientResponsive 验证握手前容量拒绝不会阻塞已接纳连接的业务往返。
func TestProdGateCapacityRejectionsKeepExistingClientResponsive(t *testing.T) {
	const (
		rounds          = 4
		rejectsPerRound = 4
	)
	local, gate, agents, clientTLS := startTestProductionGate(t, 1, time.Second)
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)

	for round := 0; round < rounds; round++ {
		for attempt := 0; attempt < rejectsPerRound; attempt++ {
			extra, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
			if err != nil {
				t.Fatalf("Dial(over-capacity %d/%d) error = %v", round, attempt, err)
			}
			if err := extra.Close(); err != nil {
				t.Fatalf("Close(over-capacity %d/%d) error = %v", round, attempt, err)
			}
		}
		wantRejected := uint64((round + 1) * rejectsPerRound)
		waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
			return metrics.RejectedConnections
		}, wantRejected)

		requestID := uint32(round + 1)
		requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: requestID})
		if err != nil {
			t.Fatalf("marshal request %d: %v", round, err)
		}
		frame := makeAuthenticatedTestFrame(t, macKey[:], uint64(round+1), requestBytes)
		if err := connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			t.Fatalf("WriteMessage(request %d) error = %v", round, err)
		}
		_, responseBytes, err := readTestBinaryMessage(t, connection)
		if err != nil {
			t.Fatalf("ReadMessage(response %d) error = %v", round, err)
		}
		var response commonv1.Envelope
		if err := proto.Unmarshal(responseBytes, &response); err != nil {
			t.Fatalf("unmarshal response %d: %v", round, err)
		}
		if response.GetMsgId() != 1002 || response.GetRequestId() != requestID {
			t.Fatalf("response %d = msg_id %d, request_id %d", round, response.GetMsgId(), response.GetRequestId())
		}
		select {
		case delivered := <-agent.received:
			if !bytes.Equal(delivered, requestBytes) {
				t.Fatalf("Agent request %d = %x, want %x", round, delivered, requestBytes)
			}
		case <-time.After(time.Second):
			t.Fatalf("Agent did not receive request %d", round)
		}
	}
	if got := gate.Metrics().AcceptedConnections; got != 1 {
		t.Fatalf("AcceptedConnections = %d, want 1", got)
	}
	if got := gate.Metrics().InboundMailboxFull; got != 0 {
		t.Fatalf("InboundMailboxFull = %d, want 0", got)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
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

// TestProdGateTLSHandshakeTimeoutReleasesConnectionSlot 验证未发送 ClientHello 的连接超时后归还 TCP 名额。
func TestProdGateTLSHandshakeTimeoutReleasesConnectionSlot(t *testing.T) {
	local, gate, _, clientTLS := startTestProductionGate(t, 1, time.Second)
	pending, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(pending TLS) error = %v", err)
	}
	defer pending.Close()
	started := time.Now()
	if err := pending.SetReadDeadline(started.Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline(pending TLS) error = %v", err)
	}
	var byteRead [1]byte
	_, err = pending.Read(byteRead[:])
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("Read(pending TLS) error = %v, want pending handshake timeout", err)
	}
	extra, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(over-capacity TLS) error = %v", err)
	}
	defer extra.Close()
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	if err := pending.SetReadDeadline(started.Add(defaultReadHeaderTimeout + 2*time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(TLS timeout) error = %v", err)
	}
	if _, err := io.Copy(io.Discard, pending); err != nil {
		if errors.As(err, &timeout) && timeout.Timeout() {
			t.Fatalf("pending TLS connection did not close by handshake timeout: %v", err)
		}
	}
	if elapsed := time.Since(started); elapsed < defaultReadHeaderTimeout-time.Second {
		t.Fatalf("pending TLS connection closed after %v, before handshake timeout", elapsed)
	}

	recoveryDeadline := time.Now().Add(time.Second)
	for {
		connection, err := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", gate.Addr().String(), clientTLS)
		if err == nil {
			if err := connection.Close(); err != nil {
				t.Fatalf("Close(recovered TLS) error = %v", err)
			}
			break
		}
		if time.Now().After(recoveryDeadline) {
			t.Fatalf("TLS connection did not recover after handshake timeout: %v", err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections = %d, want no application handshake", got)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateAppliesConnectionLimitBeforeTLSHandshake 验证连接上限覆盖未完成 TLS 握手的 TCP 连接，且断开后会归还名额。
func TestProdGateAppliesConnectionLimitBeforeTLSHandshake(t *testing.T) {
	_, gate, _, clientTLS := startTestProductionGate(t, 1, time.Second)
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
	if err := first.Close(); err != nil {
		t.Fatalf("Close(first raw TCP) error = %v", err)
	}

	endpoint := url.URL{Scheme: "wss", Host: gate.Addr().String(), Path: "/ws"}
	dialer := websocket.Dialer{TLSClientConfig: clientTLS, HandshakeTimeout: time.Second}
	deadline := time.Now().Add(time.Second)
	var recovered *websocket.Conn
	for recovered == nil && time.Now().Before(deadline) {
		connection, response, dialErr := dialer.Dial(endpoint.String(), nil)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if dialErr == nil {
			recovered = connection
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if recovered == nil {
		t.Fatal("首条 TCP 连接断开后，Gate 未归还连接名额")
	}
	_ = recovered.Close()
}

// TestProdGateHandshakeTimeoutClosesConnection 验证未完成的应用握手占用连接名额，超时关闭后名额可供后续客户端使用。
func TestProdGateHandshakeTimeoutClosesConnection(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 1, 250*time.Millisecond)
	connection := dialProductionTestClient(t, gate, clientTLS)
	defer connection.Close()
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections before client handshake = %d, want 0", got)
	}

	second, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(second raw TCP) error = %v", err)
	}
	if err := second.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(second raw TCP) error = %v", err)
	}
	var rawByte [1]byte
	if _, err := second.Read(rawByte[:]); err == nil {
		t.Fatal("未完成应用握手时，超出连接上限的 TCP 连接没有被关闭")
	}
	if err := second.Close(); err != nil {
		t.Fatalf("Close(second raw TCP) error = %v", err)
	}
	if got := gate.Metrics().RejectedConnections; got != 1 {
		t.Fatalf("RejectedConnections while application handshake is pending = %d, want 1", got)
	}

	_, _, err = readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocketHandshakeCloseCode) {
		t.Fatalf("ReadMessage() error = %v, want WebSocket close %d", err, websocketHandshakeCloseCode)
	}
	waitForGateMetric(t, gate, "HandshakeTimeouts", func(metrics Metrics) uint64 {
		return metrics.HandshakeTimeouts
	}, 1)
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 2)
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections after handshake timeout = %d, want 0", got)
	}

	endpoint := url.URL{Scheme: "wss", Host: gate.Addr().String(), Path: "/ws"}
	dialer := websocket.Dialer{TLSClientConfig: clientTLS, HandshakeTimeout: time.Second}
	dialDeadline := time.Now().Add(time.Second)
	var recovered *websocket.Conn
	for recovered == nil && time.Now().Before(dialDeadline) {
		candidate, response, dialErr := dialer.Dial(endpoint.String(), nil)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		if dialErr == nil {
			recovered = candidate
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if recovered == nil {
		t.Fatal("应用握手超时后，Gate 未归还连接名额")
	}
	defer recovered.Close()
	macKey := authenticateTestClient(t, recovered)
	agent := receiveProductionAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 411})
	if err != nil {
		t.Fatalf("marshal recovered connection request: %v", err)
	}
	if err := recovered.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("recovered connection WriteMessage() error = %v", err)
	}
	_, responseBytes, err := readTestBinaryMessage(t, recovered)
	if err != nil {
		t.Fatalf("recovered connection ReadMessage() error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal recovered connection response: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 411 {
		t.Fatalf("recovered response = msg_id %d, request_id %d; want 1002, 411", response.GetMsgId(), response.GetRequestId())
	}
	if got := gate.Metrics().AcceptedConnections; got != 1 {
		t.Fatalf("AcceptedConnections = %d, want 1", got)
	}
	if err := recovered.Close(); err != nil {
		t.Fatalf("Close(recovered client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("recovered Agent 未收到客户端断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
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
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections after non-binary handshake = %d, want 0", got)
	}
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
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections after invalid handshake = %d, want 0", got)
	}
	select {
	case agent := <-agents:
		t.Fatalf("invalid handshake created Agent %#v", agent)
	case <-time.After(30 * time.Millisecond):
	}
}

// TestProdGateRejectsInvalidX25519KeyAndRecovers 验证非法 X25519 公钥被拒绝并归还唯一连接名额，后续有效连接仍可服务。
func TestProdGateRejectsInvalidX25519KeyAndRecovers(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 1, time.Second)
	invalidConnection := dialProductionTestClient(t, gate, clientTLS)
	defer invalidConnection.Close()
	invalidHandshake := make([]byte, handshakeFrameSize)
	invalidHandshake[0] = handshakeVersion
	if err := invalidConnection.WriteMessage(websocket.BinaryMessage, invalidHandshake); err != nil {
		t.Fatalf("WriteMessage(low-order X25519 key) error = %v", err)
	}
	_, _, err := readTestBinaryMessage(t, invalidConnection)
	if !websocket.IsCloseError(err, websocket.CloseProtocolError) {
		t.Fatalf("ReadMessage(invalid X25519 key) error = %v, want WebSocket close %d", err, websocket.CloseProtocolError)
	}
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections after invalid key = %d, want 0", got)
	}
	select {
	case agent := <-agents:
		t.Fatalf("invalid X25519 key created Agent %#v", agent)
	case <-time.After(30 * time.Millisecond):
	}

	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 97})
	if err != nil {
		t.Fatalf("marshal recovery request Envelope: %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(recovery frame) error = %v", err)
	}
	select {
	case delivered := <-agent.received:
		if !bytes.Equal(delivered, requestBytes) {
			t.Fatalf("recovery Agent 收到 Envelope = %x，want %x", delivered, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("非法握手被拒绝后，Gate 未投递有效请求")
	}
	_, responseBytes, err := readTestBinaryMessage(t, connection)
	if err != nil {
		t.Fatalf("ReadMessage(recovery response) error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal recovery response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 97 {
		t.Fatalf("recovery response = msg_id %d, request_id %d; want 1002, 97", response.GetMsgId(), response.GetRequestId())
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("Close(recovery client) error = %v", err)
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("有效连接关闭后 Agent 未收到通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
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
	waitForGateMetric(t, gate, "RejectedConnections", func(metrics Metrics) uint64 {
		return metrics.RejectedConnections
	}, 1)
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections after oversized handshake = %d, want 0", got)
	}
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

// TestProdGateRejectsMalformedAuthenticatedFrame 验证短于序号和 MAC 最小长度的认证帧以协议错误关闭且不会投递给 Agent。
func TestProdGateRejectsMalformedAuthenticatedFrame(t *testing.T) {
	_, gate, agents, clientTLS := startTestProductionGate(t, 1, time.Second)
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)

	malformedFrame := make([]byte, authenticatedFrameOverhead-1)
	if err := connection.WriteMessage(websocket.BinaryMessage, malformedFrame); err != nil {
		t.Fatalf("WriteMessage(short authenticated frame) error = %v", err)
	}
	_, _, err := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseProtocolError) {
		t.Fatalf("ReadMessage(short authenticated frame) error = %v, want WebSocket close %d", err, websocket.CloseProtocolError)
	}

	select {
	case frame := <-agent.received:
		t.Fatalf("malformed authenticated frame reached Agent as %x", frame)
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("拒绝短认证帧后 Agent 未收到连接断开通知")
	}
}

// TestProdGateRejectsOversizedAuthenticatedFrame 验证认证后的超限消息在投递 Agent 前关闭当前连接，且后续连接可正常服务。
func TestProdGateRejectsOversizedAuthenticatedFrame(t *testing.T) {
	const maxFrameBytes = 256
	agents := make(chan *productionTestAgent, 2)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 2, time.Second, maxFrameBytes, 64<<10, func(client ClientSession) runtime.Service {
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agents <- agent
		return agent
	})
	connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)
	oversizedRequestBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     1001,
		RequestId: 99,
		Payload:   bytes.Repeat([]byte{'x'}, maxFrameBytes),
	})
	if err != nil {
		t.Fatalf("marshal oversized request Envelope: %v", err)
	}
	oversizedFrame := makeAuthenticatedTestFrame(t, macKey[:], 1, oversizedRequestBytes)
	if len(oversizedFrame) <= maxFrameBytes {
		t.Fatalf("authenticated frame length = %d, want greater than %d", len(oversizedFrame), maxFrameBytes)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, oversizedFrame); err != nil {
		t.Fatalf("WriteMessage(oversized authenticated frame) error = %v", err)
	}
	_, _, err = readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("ReadMessage(oversized authenticated frame) error = %v, want WebSocket close %d", err, websocket.CloseMessageTooBig)
	}
	waitForGateMetric(t, gate, "FrameTooLarge", func(metrics Metrics) uint64 {
		return metrics.FrameTooLarge
	}, 1)
	select {
	case delivered := <-agent.received:
		t.Fatalf("oversized authenticated frame reached Agent as %x", delivered)
	case <-time.After(30 * time.Millisecond):
	}
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("超限帧关闭连接后 Agent 未收到断开通知")
	}

	recoveryConnection, recoveryMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer recoveryConnection.Close()
	recoveryAgent := receiveProductionAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 100})
	if err != nil {
		t.Fatalf("marshal recovery request Envelope: %v", err)
	}
	if err := recoveryConnection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, recoveryMACKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(recovery frame) error = %v", err)
	}
	select {
	case delivered := <-recoveryAgent.received:
		if !bytes.Equal(delivered, requestBytes) {
			t.Fatalf("recovery Agent 收到 Envelope = %x，want %x", delivered, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("超限帧拒绝后，Gate 未投递有效请求")
	}
	_, responseBytes, err := readTestBinaryMessage(t, recoveryConnection)
	if err != nil {
		t.Fatalf("ReadMessage(recovery response) error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal recovery response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 100 {
		t.Fatalf("recovery response = msg_id %d, request_id %d; want 1002, 100", response.GetMsgId(), response.GetRequestId())
	}
	if err := recoveryConnection.Close(); err != nil {
		t.Fatalf("Close(recovery client) error = %v", err)
	}
	select {
	case <-recoveryAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("健康连接断开后 Agent 未收到通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateClosesConnectionWhenAgentMailboxIsFull 验证 Mailbox 满载会关闭连接并在超过原重试上限后补交通知。
func TestProdGateClosesConnectionWhenAgentMailboxIsFull(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAgent := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAgent()
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
	select {
	case <-agent.disconnected:
		t.Fatal("Agent 在 Mailbox 仍满载时收到断开通知")
	case <-time.After(defaultAgentStartWait + 100*time.Millisecond):
	}
	releaseAgent()
	select {
	case <-agent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("Mailbox 满载后 Agent 未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateStopNotifiesMappedAgent 验证单独停止 Gate 时仍会通知所有保留映射的 Agent。
func TestProdGateStopNotifiesMappedAgent(t *testing.T) {
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
	agent := &plainTestAgent{disconnected: make(chan struct{})}
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
		t.Fatal("Gate 单独停止后 Agent 未收到断开通知")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateStopsClientMessageAdmissionBeforeClosingConnections 验证 Gate 标记停机后不会再把客户端帧送入 Agent Mailbox。
func TestProdGateStopsClientMessageAdmissionBeforeClosingConnections(t *testing.T) {
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
	agent := &productionTestAgent{received: make(chan []byte, 1), disconnected: make(chan struct{})}
	agentID, err := local.CreateService(context.Background(), agent)
	if err != nil {
		t.Fatalf("CreateService(Agent) error = %v", err)
	}
	gate := &GateService{runtime: local}
	if err := gate.OnStop(context.Background()); err != nil {
		t.Fatalf("Gate.OnStop() error = %v", err)
	}
	frame, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001})
	if err != nil {
		t.Fatalf("marshal Envelope: %v", err)
	}
	if err := gate.deliverClientMessage(agentID, frame); !errors.Is(err, errGateStopping) {
		t.Fatalf("deliverClientMessage() error = %v, want %v", err, errGateStopping)
	}
	select {
	case <-agent.received:
		t.Fatal("Gate 停机后仍有客户端消息进入 Agent")
	default:
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateStopNotifiesHealthyAgentWhileAnotherMailboxIsFull 验证一个满载 Agent 不会挡住其他 Agent 的停机通知。
func TestProdGateStopNotifiesHealthyAgentWhileAnotherMailboxIsFull(t *testing.T) {
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
	blocked := &blockingInboundAgent{release: release, started: make(chan struct{}), disconnected: make(chan struct{})}
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
	healthy := &plainTestAgent{disconnected: make(chan struct{})}
	healthyID, err := local.CreateService(context.Background(), healthy)
	if err != nil {
		t.Fatalf("CreateService(healthy Agent) error = %v", err)
	}
	gate := &GateService{
		runtime:     local,
		connections: make(map[*clientConnection]struct{}),
		agentsByConnection: map[*clientConnection]runtime.ServiceID{
			{connectionID: 1}: blockedID,
			{connectionID: 2}: healthyID,
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

// TestProdGateStopBoundsPendingDisconnectRetry 验证 Agent Mailbox 满载时 Gate 停机仍按自己的期限完成。
func TestProdGateStopBoundsPendingDisconnectRetry(t *testing.T) {
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseAgent := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseAgent()
	local, gate := startConfiguredHTTPProductionGate(t, runtime.Config{
		MailboxCapacity: 1,
		ShutdownTimeout: time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(ClientSession) runtime.Service {
		return &plainTestAgent{disconnected: make(chan struct{})}
	})
	blocked := &blockingInboundAgent{release: release, started: make(chan struct{}), disconnected: make(chan struct{})}
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
	clientConn := &clientConnection{connectionID: 1}
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

// TestProdGateSlowClientDoesNotBlockHealthyConnection 验证慢连接持续发送并触发出站背压时，健康连接仍能完成请求和响应。
func TestProdGateSlowClientDoesNotBlockHealthyConnection(t *testing.T) {
	slowAgents := make(chan *longSlowOutboundAgent, 1)
	healthyAgents := make(chan *productionTestAgent, 1)
	var createdAgents atomic.Uint32
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 2, time.Second, 1<<20, 128<<10, func(client ClientSession) runtime.Service {
		if createdAgents.Add(1) == 1 {
			agent := &longSlowOutboundAgent{
				client:            client,
				duration:          500 * time.Millisecond,
				result:            make(chan longSlowOutboundResult, 1),
				disconnected:      make(chan struct{}),
				queueFullObserved: make(chan struct{}),
			}
			slowAgents <- agent
			return agent
		}
		agent := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		healthyAgents <- agent
		return agent
	})

	slowConnection, slowMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer slowConnection.Close()
	slowTCPConnection, ok := tcpConnectionFromWebSocket(slowConnection)
	if !ok {
		t.Skip("测试客户端不是 TCP 连接")
	}
	if err := slowTCPConnection.SetReadBuffer(1024); err != nil {
		t.Fatalf("SetReadBuffer() error = %v", err)
	}
	slowAgent := receiveLongSlowOutboundAgent(t, slowAgents)

	healthyConnection, healthyMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer healthyConnection.Close()
	healthyAgent := receiveProductionAgent(t, healthyAgents)

	slowRequestBytes, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: 74})
	if err != nil {
		t.Fatalf("marshal slow request Envelope: %v", err)
	}
	if err := slowConnection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, slowMACKey[:], 1, slowRequestBytes)); err != nil {
		t.Fatalf("WriteMessage(slow request frame) error = %v", err)
	}
	select {
	case <-slowAgent.queueFullObserved:
	case <-time.After(2 * time.Second):
		t.Fatal("慢客户端持续发送时未触发有界出站队列拒绝")
	}
	healthyRequest := &commonv1.Envelope{MsgId: 1001, RequestId: 75, Payload: []byte("healthy")}
	healthyRequestBytes, err := proto.Marshal(healthyRequest)
	if err != nil {
		t.Fatalf("marshal healthy request Envelope: %v", err)
	}
	if err := healthyConnection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, healthyMACKey[:], 1, healthyRequestBytes)); err != nil {
		t.Fatalf("WriteMessage(healthy request frame) error = %v", err)
	}
	select {
	case delivered := <-healthyAgent.received:
		if !bytes.Equal(delivered, healthyRequestBytes) {
			t.Fatalf("健康 Agent 收到 Envelope = %x，want %x", delivered, healthyRequestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("慢连接出站背压期间，健康 Agent 未收到请求")
	}
	responseType, responseBytes, err := readTestBinaryMessage(t, healthyConnection)
	if err != nil {
		t.Fatalf("ReadMessage(healthy response) error = %v", err)
	}
	if responseType != websocket.BinaryMessage {
		t.Fatalf("healthy response message type = %d, want binary", responseType)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal healthy response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != healthyRequest.GetRequestId() {
		t.Fatalf("healthy response = msg_id %d, request_id %d; want 1002, %d", response.GetMsgId(), response.GetRequestId(), healthyRequest.GetRequestId())
	}
	if !bytes.Equal(response.GetPayload(), healthyRequest.GetPayload()) {
		t.Fatalf("healthy response payload = %q, want %q", response.GetPayload(), healthyRequest.GetPayload())
	}

	select {
	case result := <-slowAgent.result:
		if result.err != nil {
			t.Fatalf("慢客户端持续发送 error = %v", result.err)
		}
		if result.queueFull == 0 || result.sent == 0 || result.closed {
			t.Fatalf("慢客户端压力结果 = %+v，want sent、queueFull 均大于零且连接未提前关闭", result)
		}
		if got := gate.Metrics().OutboundQueueFull; got == 0 {
			t.Fatal("OutboundQueueFull = 0，want positive count")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("慢客户端压力 Agent 未在预期时间结束")
	}

	if err := slowConnection.Close(); err != nil {
		t.Fatalf("Close(slow client) error = %v", err)
	}
	select {
	case <-slowAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("慢客户端断开后 Agent 未收到通知")
	}
	if err := healthyConnection.Close(); err != nil {
		t.Fatalf("Close(healthy client) error = %v", err)
	}
	select {
	case <-healthyAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("健康客户端断开后 Agent 未收到通知")
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
	stressStartedAt := time.Now()
	select {
	case result := <-agent.result:
		if result.err != nil {
			t.Fatalf("慢客户端持续发送遇到非预期错误: %v", result.err)
		}
		if result.sent == 0 || result.queueFull == 0 {
			t.Fatalf("slow client result = %+v, want sent and queueFull both positive", result)
		}
		t.Logf("slow_client_max_duration=%s elapsed=%s sent=%d queue_full=%d closed=%t", stressDuration, time.Since(stressStartedAt), result.sent, result.queueFull, result.closed)
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

// TestProdGateMultipleSlowClientsDoNotBlockHealthyRequests 验证多条持续背压的连接不会拖住同节点健康连接的请求与回复。
func TestProdGateMultipleSlowClientsDoNotBlockHealthyRequests(t *testing.T) {
	if os.Getenv("GSR_GATE_STRESS") != "1" {
		t.Skip("显式设置 GSR_GATE_STRESS=1 运行多慢连接压力验收")
	}
	stressDuration := 5 * time.Second
	if value := os.Getenv("GSR_GATE_SLOW_CLIENT_SECONDS"); value != "" {
		seconds, err := strconv.Atoi(value)
		if err != nil || seconds <= 0 {
			t.Fatalf("GSR_GATE_SLOW_CLIENT_SECONDS = %q, want positive integer", value)
		}
		stressDuration = time.Duration(seconds) * time.Second
	}

	const slowConnectionCount = 4
	slowAgents := make(chan *longSlowOutboundAgent, slowConnectionCount)
	healthyAgents := make(chan *productionTestAgent, 1)
	var createdAgents atomic.Uint32
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 16,
		ShutdownTimeout: 3 * time.Second,
	}, slowConnectionCount+1, time.Second, 1<<20, 128<<10, func(client ClientSession) runtime.Service {
		if createdAgents.Add(1) <= slowConnectionCount {
			agent := &longSlowOutboundAgent{
				client:            client,
				duration:          stressDuration,
				result:            make(chan longSlowOutboundResult, 1),
				disconnected:      make(chan struct{}),
				queueFullObserved: make(chan struct{}),
			}
			slowAgents <- agent
			return agent
		}
		agent := &productionTestAgent{client: client, disconnected: make(chan struct{})}
		healthyAgents <- agent
		return agent
	})

	slowConnections := make([]*websocket.Conn, 0, slowConnectionCount)
	testSlowAgents := make([]*longSlowOutboundAgent, 0, slowConnectionCount)
	defer func() {
		for _, connection := range slowConnections {
			_ = connection.Close()
		}
	}()
	for index := 0; index < slowConnectionCount; index++ {
		connection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
		slowConnections = append(slowConnections, connection)
		tcpConnection, ok := tcpConnectionFromWebSocket(connection)
		if !ok {
			t.Skip("测试客户端不是 TCP 连接")
		}
		if err := tcpConnection.SetReadBuffer(1024); err != nil {
			t.Fatalf("SetReadBuffer(slow client %d) error = %v", index, err)
		}
		testSlowAgents = append(testSlowAgents, receiveLongSlowOutboundAgent(t, slowAgents))
		request, err := proto.Marshal(&commonv1.Envelope{MsgId: 1001, RequestId: uint32(index + 1)})
		if err != nil {
			t.Fatalf("marshal slow request %d: %v", index, err)
		}
		if err := connection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, request)); err != nil {
			t.Fatalf("WriteMessage(slow request %d) error = %v", index, err)
		}
	}
	stressStartedAt := time.Now()
	for index, agent := range testSlowAgents {
		select {
		case <-agent.queueFullObserved:
		case <-time.After(2 * time.Second):
			t.Fatalf("慢客户端 %d 未触发有界出站队列拒绝", index)
		}
	}

	healthyConnection, healthyMACKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer healthyConnection.Close()
	healthyAgent := receiveProductionAgent(t, healthyAgents)
	deadline := stressStartedAt.Add(stressDuration)
	var healthyResponses uint32
	for time.Now().Before(deadline) {
		healthyResponses++
		request := &commonv1.Envelope{MsgId: 1001, RequestId: healthyResponses, Payload: []byte("healthy")}
		requestBytes, err := proto.Marshal(request)
		if err != nil {
			t.Fatalf("marshal healthy request: %v", err)
		}
		if err := healthyConnection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, healthyMACKey[:], uint64(healthyResponses), requestBytes)); err != nil {
			t.Fatalf("WriteMessage(healthy request %d) error = %v", healthyResponses, err)
		}
		messageType, responseBytes, err := readTestBinaryMessage(t, healthyConnection)
		if err != nil {
			t.Fatalf("ReadMessage(healthy response %d) error = %v", healthyResponses, err)
		}
		var response commonv1.Envelope
		if err := proto.Unmarshal(responseBytes, &response); err != nil {
			t.Fatalf("unmarshal healthy response: %v", err)
		}
		if messageType != websocket.BinaryMessage || response.GetMsgId() != 1002 || response.GetRequestId() != healthyResponses ||
			!bytes.Equal(response.GetPayload(), request.GetPayload()) {
			t.Fatalf("healthy response %d = type %d, msg_id %d, request_id %d, payload %q", healthyResponses,
				messageType, response.GetMsgId(), response.GetRequestId(), response.GetPayload())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if healthyResponses < 2 {
		t.Fatalf("healthy responses = %d, want at least 2 during slow-client backpressure", healthyResponses)
	}

	for index, agent := range testSlowAgents {
		select {
		case result := <-agent.result:
			if result.err != nil || result.sent == 0 || result.queueFull == 0 {
				t.Fatalf("slow client %d result = %+v, want sent and queueFull without unexpected error", index, result)
			}
			t.Logf("slow_client=%d sent=%d queue_full=%d closed=%t", index, result.sent, result.queueFull, result.closed)
		case <-time.After(2 * time.Second):
			t.Fatalf("慢客户端 %d 的压力 Agent 未按时结束", index)
		}
	}
	metrics := gate.Metrics()
	t.Logf("slow_connections=%d duration=%s healthy_responses=%d outbound_queue_full=%d rejected=%d inbound_mailbox_full=%d",
		slowConnectionCount, stressDuration, healthyResponses, metrics.OutboundQueueFull, metrics.RejectedConnections, metrics.InboundMailboxFull)
	if metrics.OutboundQueueFull < slowConnectionCount || metrics.RejectedConnections != 0 || metrics.InboundMailboxFull != 0 {
		t.Fatalf("Gate metrics = %+v, want slow-client queue rejections only", metrics)
	}
	for index, connection := range slowConnections {
		if err := connection.Close(); err != nil {
			t.Fatalf("Close(slow client %d) error = %v", index, err)
		}
	}
	if err := healthyConnection.Close(); err != nil {
		t.Fatalf("Close(healthy client) error = %v", err)
	}
	for index, agent := range testSlowAgents {
		select {
		case <-agent.disconnected:
		case <-time.After(time.Second):
			t.Fatalf("慢客户端 %d 断开后 Agent 未收到通知", index)
		}
	}
	select {
	case <-healthyAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("健康客户端断开后 Agent 未收到通知")
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

// TestProdGateRuntimeCloseReleasesActiveClient 验证 Runtime 关闭时生产 Gate 会收尾客户端与 I/O goroutine，并释放监听地址。
func TestProdGateRuntimeCloseReleasesActiveClient(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 2, time.Second)
	listenerAddress := gate.Addr().String()
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	agent := receiveProductionAgent(t, agents)

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
	if err := agent.client.Send(&commonv1.Envelope{MsgId: 1002, RequestId: 92}); !errors.Is(err, ErrClientSessionClosed) {
		t.Fatalf("ClientSession.Send() after Runtime.Close() error = %v, want %v", err, ErrClientSessionClosed)
	}
	listener, err := net.Listen("tcp", listenerAddress)
	if err != nil {
		t.Fatalf("Gate listener address %q 在 Runtime.Close() 后仍不可用: %v", listenerAddress, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close(rebound listener) error = %v", err)
	}
}

// TestProdGateRuntimeStopClosesAllActiveClients 验证 Runtime.Stop 会关闭所有已认证连接并使对应 ClientSession 失效。
func TestProdGateRuntimeStopClosesAllActiveClients(t *testing.T) {
	const activeConnections = 8
	local, gate, agents, clientTLS := startTestProductionGate(t, activeConnections, time.Second)
	connections := make([]*websocket.Conn, activeConnections)
	clientSessions := make([]*productionTestAgent, activeConnections)
	for index := range connections {
		connections[index], _ = dialAuthenticatedTestClient(t, gate, clientTLS)
		clientSessions[index] = receiveProductionAgent(t, agents)
	}
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()

	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopResult := make(chan error, 1)
	go func() {
		stopResult <- local.Stop(stopContext)
	}()
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("Runtime.Stop() error = %v", err)
		}
	case <-stopContext.Done():
		for _, connection := range connections {
			_ = connection.Close()
		}
		select {
		case <-stopResult:
		case <-time.After(time.Second):
		}
		t.Fatalf("Runtime.Stop() 未关闭全部活动客户端: %v", stopContext.Err())
	}

	for index, connection := range connections {
		_, _, err := readTestBinaryMessage(t, connection)
		if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
			t.Errorf("active connection %d ReadMessage() error = %v, want Gate close %d", index, err, websocket.CloseGoingAway)
		}
		if err := clientSessions[index].client.Send(&commonv1.Envelope{MsgId: 1002, RequestId: uint32(index + 1)}); !errors.Is(err, ErrClientSessionClosed) {
			t.Errorf("ClientSession.Send() after Runtime.Stop() error = %v, want %v", err, ErrClientSessionClosed)
		}
	}
	metrics := gate.Metrics()
	if metrics.AcceptedConnections != activeConnections || metrics.RejectedConnections != 0 {
		t.Fatalf("metrics after Runtime.Stop() = %+v, want %d accepted and no rejected connections", metrics, activeConnections)
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() after Stop() error = %v", err)
	}
}

// TestProdGateAgentAndRuntimeStopCloseSessionIdempotently 验证 Agent.OnStop 与 Gate 停止重复关闭同一 ClientSession 时均正常完成。
func TestProdGateAgentAndRuntimeStopCloseSessionIdempotently(t *testing.T) {
	agents := make(chan *stopClosingProductionTestAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		base := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agent := &stopClosingProductionTestAgent{
			productionTestAgent: base,
			stopResult:          make(chan error, 1),
		}
		agents <- agent
		return agent
	})
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer connection.Close()
	var agent *stopClosingProductionTestAgent
	select {
	case agent = <-agents:
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建连接 Agent")
	}

	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := local.Stop(stopContext); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
	select {
	case err := <-agent.stopResult:
		if err != nil {
			t.Fatalf("Agent.OnStop ClientSession.Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Runtime.Stop() 完成前未调用 Agent.OnStop")
	}
	if _, _, err := readTestBinaryMessage(t, connection); err == nil {
		t.Fatal("Runtime.Stop() 后客户端连接仍可读")
	}
	if err := agent.client.Close(); err != nil {
		t.Fatalf("重复 ClientSession.Close() error = %v", err)
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() after Stop() error = %v", err)
	}
}

// TestProdGateAgentStopAfterClientDisconnectKeepsServing 验证客户端先断开后 Agent.OnStop 关闭会话不会报错或影响后续连接。
func TestProdGateAgentStopAfterClientDisconnectKeepsServing(t *testing.T) {
	agents := make(chan *stopClosingProductionTestAgent, 2)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 2, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		base := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agent := &stopClosingProductionTestAgent{
			productionTestAgent: base,
			stopResult:          make(chan error, 1),
		}
		agents <- agent
		return agent
	})

	firstConnection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	firstAgent := receiveStoppingProductionTestAgent(t, agents)
	if err := firstConnection.Close(); err != nil {
		t.Fatalf("Close(first client) error = %v", err)
	}
	select {
	case <-firstAgent.disconnected:
	case <-time.After(time.Second):
		t.Fatal("第一个 Agent 未收到客户端断开通知")
	}
	select {
	case err := <-firstAgent.stopResult:
		if err != nil {
			t.Fatalf("断开后的 Agent.OnStop ClientSession.Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("客户端断开后 Agent.OnStop 未完成")
	}

	secondConnection, macKey := dialAuthenticatedTestClient(t, gate, clientTLS)
	defer secondConnection.Close()
	secondAgent := receiveStoppingProductionTestAgent(t, agents)
	requestBytes, err := proto.Marshal(&commonv1.Envelope{
		MsgId:     1001,
		RequestId: 93,
		Payload:   []byte("gate remains available"),
	})
	if err != nil {
		t.Fatalf("marshal second request Envelope: %v", err)
	}
	if err := secondConnection.WriteMessage(websocket.BinaryMessage, makeAuthenticatedTestFrame(t, macKey[:], 1, requestBytes)); err != nil {
		t.Fatalf("WriteMessage(second request) error = %v", err)
	}
	select {
	case delivered := <-secondAgent.received:
		if !bytes.Equal(delivered, requestBytes) {
			t.Fatalf("second Agent received Envelope = %x, want %x", delivered, requestBytes)
		}
	case <-time.After(time.Second):
		t.Fatal("第一个客户端断开后 Gate 未向新 Agent 投递请求")
	}
	_, responseBytes, err := readTestBinaryMessage(t, secondConnection)
	if err != nil {
		t.Fatalf("ReadMessage(second response) error = %v", err)
	}
	var response commonv1.Envelope
	if err := proto.Unmarshal(responseBytes, &response); err != nil {
		t.Fatalf("unmarshal second response Envelope: %v", err)
	}
	if response.GetMsgId() != 1002 || response.GetRequestId() != 93 || !bytes.Equal(response.GetPayload(), []byte("gate remains available")) {
		t.Fatalf("second response = %+v, want msg_id 1002, request_id 93 and echoed payload", &response)
	}

	if err := secondConnection.Close(); err != nil {
		t.Fatalf("Close(second client) error = %v", err)
	}
	select {
	case err := <-secondAgent.stopResult:
		if err != nil {
			t.Fatalf("第二个 Agent.OnStop ClientSession.Close() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("第二个 Agent.OnStop 未完成")
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestProdGateClientDisconnectOverlapsRuntimeStop 验证客户端断开与 Runtime.Stop 并发时会话仍关闭且 Agent 只执行一次清理。
func TestProdGateClientDisconnectOverlapsRuntimeStop(t *testing.T) {
	agents := make(chan *stopClosingProductionTestAgent, 1)
	local, gate, clientTLS := startConfiguredProductionGate(t, runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	}, 1, time.Second, 1<<20, 64<<10, func(client ClientSession) runtime.Service {
		base := &productionTestAgent{
			client:       client,
			received:     make(chan []byte, 1),
			disconnected: make(chan struct{}),
		}
		agent := &stopClosingProductionTestAgent{
			productionTestAgent: base,
			stopResult:          make(chan error, 2),
		}
		agents <- agent
		return agent
	})
	connection, _ := dialAuthenticatedTestClient(t, gate, clientTLS)
	agent := receiveStoppingProductionTestAgent(t, agents)

	start := make(chan struct{})
	clientCloseResult := make(chan error, 1)
	stopResult := make(chan error, 1)
	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	go func() {
		<-start
		clientCloseResult <- connection.Close()
	}()
	go func() {
		<-start
		stopResult <- local.Stop(stopContext)
	}()
	close(start)

	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("Runtime.Stop() error = %v", err)
		}
	case <-stopContext.Done():
		t.Fatalf("Runtime.Stop() 未完成: %v", stopContext.Err())
	}
	select {
	case <-clientCloseResult:
	case <-time.After(time.Second):
		t.Fatal("客户端 Close() 未完成")
	}
	select {
	case err := <-agent.stopResult:
		if err != nil {
			t.Fatalf("Agent.OnStop ClientSession.Close() error = %v", err)
		}
	default:
		t.Fatal("Runtime.Stop() 完成前未调用 Agent.OnStop")
	}
	select {
	case err := <-agent.stopResult:
		t.Fatalf("Agent.OnStop 被重复调用，额外结果 = %v", err)
	default:
	}
	if err := agent.client.Send(&commonv1.Envelope{MsgId: 1002, RequestId: 94}); !errors.Is(err, ErrClientSessionClosed) {
		t.Fatalf("ClientSession.Send() after concurrent shutdown error = %v, want %v", err, ErrClientSessionClosed)
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() after Stop() error = %v", err)
	}
}

// TestProdGateStopReportsClientCloseFailure 验证 Gate 停机保留每条连接的 Close 帧和 socket 关闭错误。
func TestProdGateStopReportsClientCloseFailure(t *testing.T) {
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
		connection, err := secureUpgrader.Upgrade(writer, request, nil)
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
		connection := newClientConnection(serverConnection, 1<<20, 64<<10, &gateCounters{}, uint64(index+1))
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

// TestProdGateRuntimeStopReleasesListener 验证 Runtime.Stop 会释放 Gate 监听地址，随后 Close 仍可完成最终释放。
func TestProdGateRuntimeStopReleasesListener(t *testing.T) {
	local, gate, _, _ := startTestProductionGate(t, 1, time.Second)
	listenerAddress := gate.Addr().String()
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
	listener, err := net.Listen("tcp", listenerAddress)
	if err != nil {
		t.Fatalf("Gate listener address %q 在 Runtime.Stop() 后仍不可用: %v", listenerAddress, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close(rebound listener) error = %v", err)
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() after Stop() error = %v", err)
	}
}

// TestProdGateRuntimeStopClosesPendingTLSHandshake 验证停机关闭尚未发送 TLS ClientHello 的原始 TCP 连接。
func TestProdGateRuntimeStopClosesPendingTLSHandshake(t *testing.T) {
	local, gate, _, _ := startTestProductionGate(t, 1, time.Second)
	pending, err := net.DialTimeout("tcp", gate.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("Dial(pending TLS) error = %v", err)
	}
	defer pending.Close()
	if err := pending.SetReadDeadline(time.Now().Add(50 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline(pending TLS) error = %v", err)
	}
	var byteRead [1]byte
	_, err = pending.Read(byteRead[:])
	var timeout net.Error
	if !errors.As(err, &timeout) || !timeout.Timeout() {
		t.Fatalf("Read(pending TLS) error = %v, want pending handshake timeout", err)
	}
	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := local.Stop(stopContext); err != nil {
		t.Fatalf("Runtime.Stop(pending TLS) error = %v", err)
	}
	if err := pending.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline(closed TLS) error = %v", err)
	}
	if _, err := pending.Read(byteRead[:]); !errors.Is(err, io.EOF) {
		t.Fatalf("Read(after Runtime.Stop) error = %v, want EOF", err)
	}
	metrics := gate.Metrics()
	if metrics.AcceptedConnections != 0 || metrics.HandshakeTimeouts != 0 {
		t.Fatalf("Gate metrics after pending TLS stop = %+v, want no Agent or application handshake timeout", metrics)
	}
}

// TestProdGateRuntimeStopClosesPendingHandshake 验证 Runtime.Stop 会关闭仍在等待客户端应用握手的连接，而不等握手超时。
func TestProdGateRuntimeStopClosesPendingHandshake(t *testing.T) {
	local, gate, agents, clientTLS := startTestProductionGate(t, 1, 30*time.Second)
	connection := dialProductionTestClient(t, gate, clientTLS)
	defer connection.Close()

	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopResult := make(chan error, 1)
	go func() {
		stopResult <- local.Stop(stopContext)
	}()
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("Runtime.Stop() error = %v", err)
		}
	case <-stopContext.Done():
		_ = connection.Close()
		select {
		case <-stopResult:
		case <-time.After(time.Second):
		}
		t.Fatalf("Runtime.Stop() 等待应用握手超时后才返回: %v", stopContext.Err())
	}

	_, _, err := readTestBinaryMessage(t, connection)
	if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
		t.Fatalf("client ReadMessage() error = %v, want Gate close %d", err, websocket.CloseGoingAway)
	}
	if got := gate.Metrics().HandshakeTimeouts; got != 0 {
		t.Fatalf("HandshakeTimeouts after Runtime.Stop() = %d, want 0", got)
	}
	if got := gate.Metrics().AcceptedConnections; got != 0 {
		t.Fatalf("AcceptedConnections after Runtime.Stop() = %d, want 0", got)
	}
	select {
	case agent := <-agents:
		t.Fatalf("pending handshake created Agent %#v", agent)
	default:
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() after Stop() error = %v", err)
	}
}

// TestProdGateRuntimeStopClosesAllPendingHandshakes 验证 Runtime.Stop 会关闭全部并发等待应用握手的连接并等待它们退出。
func TestProdGateRuntimeStopClosesAllPendingHandshakes(t *testing.T) {
	const pendingConnections = 8
	local, gate, agents, clientTLS := startTestProductionGate(t, pendingConnections, 30*time.Second)
	connections := make([]*websocket.Conn, pendingConnections)
	for index := range connections {
		connections[index] = dialProductionTestClient(t, gate, clientTLS)
	}
	defer func() {
		for _, connection := range connections {
			_ = connection.Close()
		}
	}()

	stopContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stopResult := make(chan error, 1)
	go func() {
		stopResult <- local.Stop(stopContext)
	}()
	select {
	case err := <-stopResult:
		if err != nil {
			t.Fatalf("Runtime.Stop() error = %v", err)
		}
	case <-stopContext.Done():
		for _, connection := range connections {
			_ = connection.Close()
		}
		select {
		case <-stopResult:
		case <-time.After(time.Second):
		}
		t.Fatalf("Runtime.Stop() 未关闭全部待握手连接: %v", stopContext.Err())
	}

	for index, connection := range connections {
		_, _, err := readTestBinaryMessage(t, connection)
		if !websocket.IsCloseError(err, websocket.CloseGoingAway) {
			t.Errorf("pending connection %d ReadMessage() error = %v, want Gate close %d", index, err, websocket.CloseGoingAway)
		}
	}
	metrics := gate.Metrics()
	if metrics.HandshakeTimeouts != 0 || metrics.AcceptedConnections != 0 {
		t.Fatalf("metrics after Runtime.Stop() = %+v, want no timeout or accepted clients", metrics)
	}
	if metrics.RejectedConnections != pendingConnections {
		t.Fatalf("RejectedConnections after Runtime.Stop() = %d, want %d", metrics.RejectedConnections, pendingConnections)
	}
	select {
	case agent := <-agents:
		t.Fatalf("pending handshakes created Agent %#v", agent)
	default:
	}
	if err := local.Close(); err != nil {
		t.Fatalf("Runtime.Close() after Stop() error = %v", err)
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

// TestNewGateServiceRequiresFrameLimitForHandshake 验证生产 Gate 拒绝小于固定握手帧的上限，同时允许恰好容纳握手的配置。
func TestNewGateServiceRequiresFrameLimitForHandshake(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	config := Config{
		Address:               "127.0.0.1:0",
		MaxConnections:        1,
		MaxFrameBytes:         handshakeFrameSize - 1,
		MaxOutboundQueueBytes: queueEntryOverhead,
		NewAgent:              func(ClientSession) runtime.Service { return &productionTestAgent{} },
	}
	if _, err := NewGateService(local, config); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewGateService(frame limit %d) error = %v, want ErrInvalidConfig", config.MaxFrameBytes, err)
	}
	config.MaxFrameBytes = handshakeFrameSize
	if _, err := NewGateService(local, config); err != nil {
		t.Fatalf("NewGateService(frame limit %d) error = %v, want success", config.MaxFrameBytes, err)
	}
}

// TestProdGateListenFailureLeavesRuntimeAvailable 验证监听地址被占用时 Gate 创建失败不会停止 Runtime，释放地址后可创建新 Gate。
func TestProdGateListenFailureLeavesRuntimeAvailable(t *testing.T) {
	blockedListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen(blocked address) error = %v", err)
	}
	t.Cleanup(func() { _ = blockedListener.Close() })

	local, err := runtime.NewRuntime(runtime.Config{
		MailboxCapacity: 8,
		ShutdownTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Errorf("Runtime.Close() error = %v", err)
		}
	})
	if err := local.Start(); err != nil {
		t.Fatalf("Runtime.Start() error = %v", err)
	}

	config := Config{
		Address:               blockedListener.Addr().String(),
		MaxConnections:        1,
		HandshakeTimeout:      time.Second,
		MaxFrameBytes:         1 << 20,
		MaxOutboundQueueBytes: 64 << 10,
		NewAgent:              func(ClientSession) runtime.Service { return &productionTestAgent{} },
	}
	failedGate, err := NewGateService(local, config)
	if err != nil {
		t.Fatalf("NewGateService(occupied address) error = %v", err)
	}
	if id, err := local.CreateService(context.Background(), failedGate); id != 0 || err == nil {
		t.Fatalf("CreateService(occupied address) = (%d, %v), want zero ID and listen error", id, err)
	}
	if err := blockedListener.Close(); err != nil {
		t.Fatalf("Close(blocked listener) error = %v", err)
	}

	gate, err := NewGateService(local, config)
	if err != nil {
		t.Fatalf("NewGateService(released address) error = %v", err)
	}
	if id, err := local.CreateService(context.Background(), gate); err != nil || id == 0 {
		t.Fatalf("CreateService(released address) = (%d, %v), want nonzero ID and success", id, err)
	}
	if address := gate.Addr(); address == nil || address.String() != config.Address {
		t.Fatalf("Gate.Addr() = %v, want %q", address, config.Address)
	}
	if err := local.Stop(context.Background()); err != nil {
		t.Fatalf("Runtime.Stop() error = %v", err)
	}
}

// TestClientSessionBoundsOutboundQueue 验证生产 ClientSession 按帧字节和固定条目开销限制出站队列。
func TestClientSessionBoundsOutboundQueue(t *testing.T) {
	client := newClientConnection(nil, 128, queueEntryOverhead+2, &gateCounters{}, 0)
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

type startFailingProductionTestAgent struct {
	*productionTestAgent
	stopCalled chan struct{}
}

func (*startFailingProductionTestAgent) OnStart(context.Context, runtime.ServiceContext) error {
	return errors.New("agent startup failed")
}

func (agent *startFailingProductionTestAgent) OnStop(context.Context) error {
	close(agent.stopCalled)
	return nil
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

type stopClosingProductionTestAgent struct {
	*productionTestAgent
	stopResult chan error
}

func (agent *stopClosingProductionTestAgent) OnStop(context.Context) error {
	err := agent.client.Close()
	agent.stopResult <- err
	return err
}

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
	client            ClientSession
	duration          time.Duration
	result            chan longSlowOutboundResult
	disconnected      chan struct{}
	queueFullObserved chan struct{}
	queueFullOnce     sync.Once
	stopOnce          sync.Once
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
				if agent.queueFullObserved != nil {
					agent.queueFullOnce.Do(func() { close(agent.queueFullObserved) })
				}
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

func receiveStoppingProductionTestAgent(t *testing.T, agents <-chan *stopClosingProductionTestAgent) *stopClosingProductionTestAgent {
	t.Helper()
	select {
	case agent := <-agents:
		return agent
	case <-time.After(time.Second):
		t.Fatal("Gate 未创建连接 Agent")
		return nil
	}
}

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
	closeErr              error
	closeCount            *atomic.Int32
	once                  sync.Once
	mu                    sync.Mutex
	writeErr              error
	deadlineErr           error
	readDeadlineErr       error
	readDeadlineErrOnCall int
	readDeadlineCalls     int
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

// SetWriteDeadline 允许测试模拟底层 socket 设置写截止时间失败。
func (connection *closeErrorTestConn) SetWriteDeadline(deadline time.Time) error {
	connection.mu.Lock()
	deadlineErr := connection.deadlineErr
	connection.mu.Unlock()
	if deadlineErr != nil {
		return deadlineErr
	}
	return connection.Conn.SetWriteDeadline(deadline)
}

func (connection *closeErrorTestConn) failWriteDeadline(err error) {
	connection.mu.Lock()
	connection.deadlineErr = err
	connection.mu.Unlock()
}

// SetReadDeadline 允许测试模拟底层 socket 设置读截止时间失败。
func (connection *closeErrorTestConn) SetReadDeadline(deadline time.Time) error {
	connection.mu.Lock()
	connection.readDeadlineCalls++
	shouldFail := connection.readDeadlineCalls == connection.readDeadlineErrOnCall
	readDeadlineErr := connection.readDeadlineErr
	connection.mu.Unlock()
	if shouldFail {
		return readDeadlineErr
	}
	return connection.Conn.SetReadDeadline(deadline)
}

func (connection *closeErrorTestConn) failReadDeadline(err error) {
	connection.failReadDeadlineOnCall(1, err)
}

func (connection *closeErrorTestConn) failReadDeadlineOnCall(call int, err error) {
	connection.mu.Lock()
	connection.readDeadlineErr = err
	connection.readDeadlineErrOnCall = connection.readDeadlineCalls + call
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
