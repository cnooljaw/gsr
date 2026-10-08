package gsr

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
)

// 验证 ProdGate 完成握手后才创建应用 Agent，并为它绑定业务上下文、随 Runtime 停止。
func TestOpenProdGateCreatesApplicationAgentAfterHandshake(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addresses := make(chan string, 1)
	started := make(chan runtime.ServiceID, 1)
	stopped := make(chan struct{}, 1)
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		address, err := self.OpenProdGate(ctx, ProdGateConfig{
			Address:               "127.0.0.1:0",
			MaxConnections:        1,
			MaxOutboundQueueBytes: 64 << 10,
			NewAgent: func(client gate.ClientSession) Service {
				return &productionApplicationAgent{client: client, started: started, stopped: stopped}
			},
		})
		if err == nil {
			addresses <- address
		}
		return err
	}}
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run() error = %v", err)
			}
		case <-time.After(time.Second):
			t.Error("Runtime 停止超时")
		}
	})
	var address string
	select {
	case address = <-addresses:
	case <-ctx.Done():
		t.Fatal("等待 Gate 打开超时")
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws://"+address+"/ws", nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer connection.Close()
	select {
	case <-started:
		t.Fatal("握手前不应创建 Agent")
	default:
	}
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	handshake := append([]byte{1}, privateKey.PublicKey().Bytes()...)
	if err := connection.WriteMessage(websocket.BinaryMessage, handshake); err != nil {
		t.Fatalf("WriteMessage(handshake) error = %v", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	kind, response, err := connection.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage(handshake) error = %v", err)
	}
	if kind != websocket.BinaryMessage || len(response) != 33 || response[0] != 1 {
		t.Fatalf("unexpected handshake: kind=%d length=%d", kind, len(response))
	}
	select {
	case id := <-started:
		if id == 0 {
			t.Fatal("Agent 身份未绑定")
		}
	case <-ctx.Done():
		t.Fatal("握手后未启动应用 Agent")
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Runtime 停止未调用 Agent.OnStop")
	}
}

type productionApplicationAgent struct {
	client  gate.ClientSession
	started chan<- runtime.ServiceID
	stopped chan<- struct{}
}

func (agent *productionApplicationAgent) OnStart(_ context.Context, self ServiceContext) error {
	if self.Cluster() == nil || self.Logger() == nil {
		return fmt.Errorf("Agent 缺少应用上下文能力")
	}
	agent.started <- self.Self()
	return nil
}

func (agent *productionApplicationAgent) Handle(CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, nil
}

func (agent *productionApplicationAgent) HandleClientMessage(CommandContext, []byte) error {
	return nil
}

func (agent *productionApplicationAgent) OnStop(context.Context) error {
	agent.stopped <- struct{}{}
	return agent.client.Close()
}
