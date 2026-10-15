package main

import (
	"context"
	"fmt"
	"time"

	codec "github.com/cnooljaw/gosproto"
	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/examples/websocket-gate/gen/sproto/c2s"
	"github.com/cnooljaw/gsr/examples/websocket-gate/gen/sproto/s2c"
	"github.com/cnooljaw/gsr/gate"
	sprotocol "github.com/cnooljaw/gsr/protocol/sproto"
	"github.com/cnooljaw/gsr/runtime"
)

// sprotoAgent 组合连接与原生 Sproto 分发，业务复用现有 EchoService。
type sprotoAgent struct {
	client             gate.ClientSession
	protocolDispatcher *sprotocol.Dispatcher[gsr.CommandContext]
	commandDispatcher  *gsr.Dispatcher
}

var (
	_ gsr.Service              = (*sprotoAgent)(nil)
	_ gsr.ClientMessageHandler = (*sprotoAgent)(nil)
)

func newSprotoAgent(client gate.ClientSession) *sprotoAgent {
	agent := &sprotoAgent{client: client}
	agent.commandDispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		pushEchoCommand: agent.handlePush,
	})
	agent.protocolDispatcher = sprotocol.MustDispatcher[gsr.CommandContext](
		sprotocol.Config{C2S: c2s.Protocols, S2C: s2c.Protocols},
		client,
		map[string]any{"echo": agent.handleEcho, "notify": agent.handleNotify},
	)
	return agent
}

func (agent *sprotoAgent) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (agent *sprotoAgent) Handle(self gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if command == gate.ClientDisconnectedCommand {
		self.Exit()
		return nil, nil
	}
	return agent.commandDispatcher.Handle(self, command, args...)
}

// HandleClientMessage 只解释 Sproto；错误关闭连接，客户端的等待随断线结束。
func (agent *sprotoAgent) HandleClientMessage(self gsr.CommandContext, frame []byte) error {
	if err := agent.protocolDispatcher.HandleFrame(self, frame); err != nil {
		_ = agent.client.Close()
		return err
	}
	return nil
}

func (agent *sprotoAgent) handleEcho(self gsr.CommandContext, request sprotocol.Context, payload *c2s.EchoRequest) error {
	text := ""
	if payload.Text != nil {
		text = *payload.Text
	}
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := self.CallName(wait, echoServiceName, echoCommand, text)
	if err != nil {
		return err
	}
	response, ok := result.(echoResult)
	if !ok {
		return fmt.Errorf("unexpected Echo result: %T", result)
	}
	if !response.Accepted {
		return request.Response(&c2s.EchoResponse{Code: codec.Int64(2), Message: codec.String("text is required")})
	}
	return request.Response(&c2s.EchoResponse{Text: codec.String(response.Text), Code: codec.Int64(1)})
}

func (agent *sprotoAgent) handleNotify(self gsr.CommandContext, _ sprotocol.Context, payload *c2s.NotifyRequest) error {
	text := ""
	if payload.Text != nil {
		text = *payload.Text
	}
	return self.SendName(echoServiceName, notifyEchoCommand, text)
}

func (agent *sprotoAgent) handlePush(_ gsr.CommandContext, text string) error {
	return agent.protocolDispatcher.Push("push", &s2c.PushRequest{Text: codec.String(text)})
}

func (agent *sprotoAgent) OnStop(_ context.Context) error {
	return agent.client.Close()
}
