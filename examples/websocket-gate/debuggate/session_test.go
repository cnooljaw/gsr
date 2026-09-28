package debuggate

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestDebugClientSessionCloseAggregatesCloseFailures 验证 Close 帧写入与 socket 关闭同时失败时都能被识别。
func TestDebugClientSessionCloseAggregatesCloseFailures(t *testing.T) {
	serverConnection, clientConnection, serverSocket := newDebugWriteFailureWebSocketPair(t)
	closeFrameFailure := errors.New("test close frame write failure")
	socketCloseFailure := errors.New("test socket close failure")
	serverSocket.failWrites(closeFrameFailure)
	serverSocket.closeErr = socketCloseFailure

	client := newClientConnection(serverConnection, 1024, 2048, &gateCounters{})
	closeErr := client.Close()
	for _, expected := range []error{closeFrameFailure, socketCloseFailure} {
		if !errors.Is(closeErr, expected) {
			t.Errorf("ClientSession.Close() error = %v, want to preserve %v", closeErr, expected)
		}
	}
	if err := clientConnection.Close(); err != nil {
		t.Fatalf("Close(client WebSocket) error = %v", err)
	}
}

// TestDebugClientWriterClosesAfterDeadlineFailure 验证底层 deadline 设置失败时 DebugGate 会关闭连接并结束写循环。
func TestDebugClientWriterClosesAfterDeadlineFailure(t *testing.T) {
	serverConnection, clientConnection, serverSocket := newDebugWriteFailureWebSocketPair(t)
	deadlineFailure := errors.New("test WebSocket write deadline failure")
	serverSocket.failWriteDeadline(deadlineFailure)
	client := newClientConnection(serverConnection, 1024, 2048, &gateCounters{})
	if err := client.Send(&Envelope{MsgId: 1002}); err != nil {
		t.Fatalf("ClientSession.Send() error = %v", err)
	}

	writerDone := make(chan error, 1)
	go func() {
		writerDone <- client.writeLoop()
	}()
	select {
	case err := <-writerDone:
		if !errors.Is(err, deadlineFailure) {
			t.Fatalf("writeLoop() error = %v, want %v", err, deadlineFailure)
		}
	case <-time.After(time.Second):
		t.Fatal("DebugGate 写循环未在 deadline 设置失败后退出")
	}
	if err := client.Close(); !errors.Is(err, deadlineFailure) {
		t.Fatalf("ClientSession.Close() error = %v, want to preserve %v", err, deadlineFailure)
	}
	if err := clientConnection.Close(); err != nil {
		t.Fatalf("Close(client WebSocket) error = %v", err)
	}
}

func newDebugWriteFailureWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, *closeErrorTestConn) {
	t.Helper()

	serverConnections := make(chan *websocket.Conn, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := debugUpgrader.Upgrade(writer, request, nil)
		if err != nil {
			t.Errorf("Upgrade() error = %v", err)
			return
		}
		serverConnections <- connection
	}))
	listener := &closeErrorTestListener{Listener: server.Listener, accepted: make(chan *closeErrorTestConn, 1)}
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)

	endpoint := url.URL{Scheme: "ws", Host: server.Listener.Addr().String(), Path: "/"}
	clientConnection, _, err := websocket.DefaultDialer.Dial(endpoint.String(), nil)
	if err != nil {
		t.Fatalf("Dial(test WebSocket) error = %v", err)
	}
	t.Cleanup(func() { _ = clientConnection.Close() })

	var serverConnection *websocket.Conn
	select {
	case serverConnection = <-serverConnections:
	case <-time.After(time.Second):
		t.Fatal("测试服务器未建立 WebSocket 连接")
	}
	var serverSocket *closeErrorTestConn
	select {
	case serverSocket = <-listener.accepted:
	case <-time.After(time.Second):
		t.Fatal("测试服务器未接纳底层 socket")
	}
	return serverConnection, clientConnection, serverSocket
}
