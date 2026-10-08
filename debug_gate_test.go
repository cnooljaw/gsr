package gsr

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// 验证应用层 Agent 经 DebugGate 创建后，能使用业务上下文接收客户端消息并写回响应。
func TestOpenDebugGateConnectsApplicationAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addresses := make(chan string, 1)
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		address, err := self.OpenDebugGate(ctx, DebugGateConfig{
			Address:               "127.0.0.1:0",
			MaxConnections:        1,
			MaxOutboundQueueBytes: 64 << 10,
			NewAgent: func(client gate.ClientSession) Service {
				return &applicationGateAgent{client: client}
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
	var address string
	select {
	case address = <-addresses:
	case err := <-done:
		t.Fatalf("Run() 在 Gate 打开前结束: %v", err)
	case <-ctx.Done():
		t.Fatal("等待 Gate 打开超时")
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws://"+address+"/ws", nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer connection.Close()
	request, err := proto.Marshal(&gate.Envelope{MsgId: 1001, RequestId: 7})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, request); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}
	if err := connection.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	_, responseBytes, err := connection.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	response := new(gate.Envelope)
	if err := proto.Unmarshal(responseBytes, response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if response.MsgId != 1002 || response.RequestId != 7 {
		t.Fatalf("response = %+v, want msg_id=1002 request_id=7", response)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run() error = %v", err)
	}
}

type applicationGateAgent struct {
	client gate.ClientSession
}

func (agent *applicationGateAgent) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (agent *applicationGateAgent) Handle(CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, nil
}

func (agent *applicationGateAgent) HandleClientMessage(self CommandContext, frame []byte) error {
	request := new(gate.Envelope)
	if err := proto.Unmarshal(frame, request); err != nil {
		return err
	}
	if self.Self() == 0 || self.Source() != 0 {
		return fmt.Errorf("unexpected Agent identity: self=%d source=%d", self.Self(), self.Source())
	}
	return agent.client.Send(&gate.Envelope{MsgId: 1002, RequestId: request.RequestId})
}

func (agent *applicationGateAgent) OnStop(context.Context) error {
	return nil
}
