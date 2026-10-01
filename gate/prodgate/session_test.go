package prodgate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/gorilla/websocket"
)

// TestProdClientSessionCloseAggregatesCloseFailures 验证 Close 帧写入与 socket 关闭同时失败时都能被识别。
func TestProdClientSessionCloseAggregatesCloseFailures(t *testing.T) {
	serverConnection, clientConnection, serverSocket := newProdWriteFailureWebSocketPair(t)
	closeFrameFailure := errors.New("test close frame write failure")
	socketCloseFailure := errors.New("test socket close failure")
	serverSocket.failWrites(closeFrameFailure)
	serverSocket.closeErr = socketCloseFailure

	client := newClientConnection(serverConnection, 1024, 2048, &gateCounters{}, 1)
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

// TestProdGateLogsClientWriterFailures 验证底层 deadline 和消息写入失败都会由 Gate 记录连接故障。
func TestProdGateLogsClientWriterFailures(t *testing.T) {
	for _, testCase := range []struct {
		name              string
		failWriteDeadline bool
	}{
		{name: "deadline", failWriteDeadline: true},
		{name: "message write"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			logger, err := logging.New(logging.Config{Writer: &output})
			if err != nil {
				t.Fatalf("logging.New() error = %v", err)
			}
			t.Cleanup(func() { _ = logger.Close(context.Background()) })

			serverConnection, clientConnection, serverSocket := newProdWriteFailureWebSocketPair(t)
			writeFailure := errors.New("test WebSocket " + testCase.name + " failure")
			if testCase.failWriteDeadline {
				serverSocket.failWriteDeadline(writeFailure)
			} else {
				serverSocket.failWrites(writeFailure)
			}
			client := newClientConnection(serverConnection, 1024, 2048, &gateCounters{}, 7)
			if err := client.Send(&Envelope{MsgId: 1002}); err != nil {
				t.Fatalf("ClientSession.Send() error = %v", err)
			}

			gate := &GateService{logger: logger}
			gate.startClientWriter(client)
			writerDone := make(chan struct{})
			go func() {
				gate.ioWorkers.Wait()
				close(writerDone)
			}()
			select {
			case <-writerDone:
			case <-time.After(time.Second):
				t.Fatal("Gate 写循环未在失败后退出")
			}
			if err := logger.Close(context.Background()); err != nil {
				t.Fatalf("Logger.Close() error = %v", err)
			}
			if err := clientConnection.Close(); err != nil {
				t.Fatalf("Close(client WebSocket) error = %v", err)
			}

			decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
			for decoder.More() {
				var entry struct {
					Message string         `json:"message"`
					Fields  map[string]any `json:"fields"`
				}
				if err := decoder.Decode(&entry); err != nil {
					t.Fatalf("decode log entry: %v", err)
				}
				if entry.Message != "Gate connection failure" {
					continue
				}
				if entry.Fields["category"] != "write" {
					t.Fatalf("Gate failure category = %v, want write", entry.Fields["category"])
				}
				loggedError, ok := entry.Fields["error"].(string)
				if !ok || !strings.Contains(loggedError, writeFailure.Error()) {
					t.Fatalf("Gate write error = %v, want %q", entry.Fields["error"], writeFailure)
				}
				return
			}
			t.Fatal("missing Gate client write failure log")
		})
	}
}

func newProdWriteFailureWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, *closeErrorTestConn) {
	t.Helper()

	serverConnections := make(chan *websocket.Conn, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		connection, err := secureUpgrader.Upgrade(writer, request, nil)
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
