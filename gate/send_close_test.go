package gate

import (
	"errors"
	"github.com/cnooljaw/gsr/runtime"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// 验证末帧与此前消息保持 FIFO 和快照，接纳后拒绝新发送，并在写完后关闭。
func TestSendAndCloseFIFO(t *testing.T) {
	server, peer, _ := newProdWriteFailureWebSocketPair(t)
	defer peer.Close()
	client := newClientConnection(server, 1024, 2048, &gateCounters{}, 1)
	if err := client.Send([]byte("old")); err != nil {
		t.Fatal(err)
	}
	last := []byte("last")
	if err := client.SendAndClose(last); err != nil {
		t.Fatal(err)
	}
	last[0] = 'X'
	if err := client.Send(nil); !errors.Is(err, ErrClientSessionClosed) {
		t.Fatal(err)
	}
	if err := client.SendAndClose(nil); !errors.Is(err, ErrClientSessionClosed) {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- client.writeLoop() }()
	peer.SetReadDeadline(time.Now().Add(time.Second))
	for _, want := range []string{"old", "last"} {
		_, frame, err := peer.ReadMessage()
		if err != nil || string(frame) != want {
			t.Fatalf("frame=%q err=%v", frame, err)
		}
	}
	if _, _, err := peer.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("writer did not finish")
	}
}

// 验证末帧超过预算时仍封闭连接，不以普通发送伪装成功。
func TestSendAndCloseAdmissionFailure(t *testing.T) {
	for _, large := range []bool{false, true} {
		server, peer, _ := newProdWriteFailureWebSocketPair(t)
		client := newClientConnection(server, 4, 68, &gateCounters{}, 1)
		frame := []byte("last")
		want := ErrClientQueueFull
		if large {
			frame = []byte("large")
			want = ErrFrameTooLarge
		} else if err := client.Send(frame); err != nil {
			t.Fatal(err)
		}
		if err := client.SendAndClose(frame); !errors.Is(err, want) {
			t.Fatal(err)
		}
		if err := client.Send(nil); !errors.Is(err, ErrClientSessionClosed) {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- client.writeLoop() }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("failed admission did not close")
		}
		peer.Close()
	}
}

// 验证末帧总预算能中断已在写出的旧帧，重复终止不延长期限，显式 Close 可提前中止。
func TestSendAndCloseInterruptsOldWrite(t *testing.T) {
	for _, abort := range []bool{false, true} {
		server, peer, socket := newProdWriteFailureWebSocketPair(t)
		client := newClientConnection(server, 1024, 2048, &gateCounters{}, 1)
		client.sendAndCloseTimeout = 40 * time.Millisecond
		entered, closed := make(chan struct{}), make(chan struct{})
		var once sync.Once
		socket.mu.Lock()
		socket.beforeWrite = func() error { once.Do(func() { close(entered) }); <-closed; return net.ErrClosed }
		socket.afterClose = func() { close(closed) }
		socket.mu.Unlock()
		client.Send([]byte("old"))
		done := make(chan error, 1)
		go func() { done <- client.writeLoop() }()
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("old write did not start")
		}
		if err := client.SendAndClose([]byte("last")); err != nil {
			t.Fatal(err)
		}
		if err := client.SendAndClose(nil); !errors.Is(err, ErrClientSessionClosed) {
			t.Fatal(err)
		}
		if abort {
			client.Close()
		}
		select {
		case err := <-done:
			if !abort && !errors.Is(err, ErrSendAndCloseTimeout) {
				t.Fatal(err)
			}
		case <-time.After(500 * time.Millisecond):
			t.Fatal("old write exceeded total budget")
		}
		peer.Close()
	}
}

// 验证并发 Send 与末帧共享接纳顺序，末帧后没有帧，入站也不再被接纳。
func TestSendAndCloseConcurrentAdmission(t *testing.T) {
	server, peer, _ := newProdWriteFailureWebSocketPair(t)
	defer peer.Close()
	client := newClientConnection(server, 1024, 16384, &gateCounters{}, 1)
	start := make(chan struct{})
	results := make(chan error, 32)
	for i := 0; i < 32; i++ {
		go func() { <-start; results <- client.Send([]byte("before")) }()
	}
	close(start)
	if err := client.SendAndClose([]byte("last")); err != nil {
		t.Fatal(err)
	}
	admitted := 0
	for i := 0; i < 32; i++ {
		err := <-results
		if err == nil {
			admitted++
		} else if !errors.Is(err, ErrClientSessionClosed) {
			t.Fatal(err)
		}
	}
	if err := client.outbound.admitInbound(func() error { t.Error("new inbound admitted"); return nil }); !errors.Is(err, ErrClientSessionClosed) {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- client.writeLoop() }()
	peer.SetReadDeadline(time.Now().Add(time.Second))
	for i := 0; i < admitted+1; i++ {
		_, frame, err := peer.ReadMessage()
		want := "before"
		if i == admitted {
			want = "last"
		}
		if err != nil || string(frame) != want {
			t.Fatal(string(frame), err)
		}
	}
	if _, _, err := peer.ReadMessage(); !websocket.IsCloseError(err, websocket.CloseNormalClosure) {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// 验证关闭预算零值为一秒、可覆盖且负数拒绝，不依赖运行时启动。
func TestSendAndCloseConfig(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer local.Close()
	config := Config{Address: "127.0.0.1:0", MaxConnections: 1, MaxOutboundQueueBytes: 1024, NewAgent: func(ClientSession) runtime.Service { return &productionTestAgent{} }}
	gate, err := NewGateService(local, config)
	if err != nil || gate.config.SendAndCloseTimeout != time.Second {
		t.Fatal(err)
	}
	config.SendAndCloseTimeout = -time.Second
	if _, err := NewGateService(local, config); !errors.Is(err, ErrInvalidConfig) {
		t.Fatal(err)
	}
	config.SendAndCloseTimeout = 20 * time.Millisecond
	gate, err = NewGateService(local, config)
	if err != nil || gate.config.SendAndCloseTimeout != 20*time.Millisecond {
		t.Fatal(err)
	}
}
