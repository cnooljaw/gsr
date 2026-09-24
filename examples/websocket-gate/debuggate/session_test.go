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

// TestDebugClientSessionCloseReturnsCloseFrameWriteFailure 验证 Close 帧写失败不会被底层连接关闭成功掩盖。
func TestDebugClientSessionCloseReturnsCloseFrameWriteFailure(t *testing.T) {
	serverConnection, clientConnection, serverSocket := newDebugWriteFailureWebSocketPair(t)
	closeFrameFailure := errors.New("test close frame write failure")
	serverSocket.failWrites(closeFrameFailure)

	client := newClientConnection(serverConnection, 1024, 2048, &gateCounters{})
	if err := client.Close(); !errors.Is(err, closeFrameFailure) {
		t.Fatalf("ClientSession.Close() error = %v, want to preserve %v", err, closeFrameFailure)
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
