package gsr

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/protocol"
	commonv1 "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// 验证应用 Agent 的强类型协议 Handler 能按名称调用业务 Service，保持 Agent 来源并回传客户端请求编号。
func TestOpenGateConnectsApplicationAgent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addresses := make(chan string, 1)
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		backendID, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			return current.Source(), nil
		}})
		if err != nil {
			return err
		}
		if err := self.Register("gate-test-backend", backendID); err != nil {
			return err
		}
		address, err := self.OpenGate(ctx, GateConfig{Protocol: gate.ProtocolPlain,
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
	case err := <-done:
		done <- err // 留给统一清理路径读取，避免再次等待已结束的 Run。
		t.Fatalf("Run() 在 Gate 打开前结束: %v", err)
	case <-ctx.Done():
		t.Fatal("等待 Gate 打开超时")
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws://"+address+"/ws", nil)
	if err != nil {
		t.Fatalf("Dial() error = %v", err)
	}
	defer connection.Close()
	payload, err := proto.Marshal(&echopb.EchoRequest{Text: "application-call"})
	if err != nil {
		t.Fatalf("Marshal(payload) error = %v", err)
	}
	request, err := proto.Marshal(&commonv1.Envelope{
		MsgId: uint32(echopb.MsgId_C2S_ECHO_REQUEST), RequestId: 7, Payload: payload,
	})
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
	response := new(commonv1.Envelope)
	if err := proto.Unmarshal(responseBytes, response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if response.MsgId != uint32(echopb.MsgId_S2C_ECHO_RESPONSE) || response.RequestId != 7 {
		t.Fatalf("response = %+v, want msg_id=1002 request_id=7", response)
	}
	var reply echopb.EchoResponse
	if err := proto.Unmarshal(response.Payload, &reply); err != nil {
		t.Fatalf("Unmarshal(payload) error = %v", err)
	}
	if reply.Text != "application-call" {
		t.Fatalf("response text = %q, want application-call", reply.Text)
	}
}

type applicationGateAgent struct {
	client     gate.ClientSession
	dispatcher *protocol.Dispatcher[CommandContext]
}

func (agent *applicationGateAgent) OnStart(context.Context, ServiceContext) error {
	registry, err := protocol.NewRegistry(commonv1.MessageCatalogGsrCommonV1Common, echopb.MessageCatalogGsrExampleEchoV1Echo)
	if err != nil {
		return err
	}
	dispatcher, err := protocol.NewDispatcher[CommandContext](registry, agent.client)
	if err != nil {
		return err
	}
	if err := protocol.Register(dispatcher, agent.onEcho); err != nil {
		return err
	}
	agent.dispatcher = dispatcher
	return nil
}

func (agent *applicationGateAgent) Handle(CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, nil
}

func (agent *applicationGateAgent) HandleClientMessage(self CommandContext, frame []byte) error {
	return agent.dispatcher.HandleFrame(self, frame)
}

func (agent *applicationGateAgent) onEcho(self CommandContext, ctx protocol.Context, request *echopb.EchoRequest) error {
	if self.Self() == 0 || self.Source() != 0 {
		return fmt.Errorf("unexpected Agent identity: self=%d source=%d", self.Self(), self.Source())
	}
	result, err := self.CallName(context.Background(), "gate-test-backend", 1)
	if err != nil {
		return err
	}
	if source, ok := result.(runtime.ServiceID); !ok || source != self.Self() {
		return fmt.Errorf("backend source = %v, want Agent %d", result, self.Self())
	}
	return ctx.Response(uint32(echopb.MsgId_S2C_ECHO_RESPONSE), &echopb.EchoResponse{Text: request.Text})
}

func (agent *applicationGateAgent) OnStop(context.Context) error {
	return nil
}
