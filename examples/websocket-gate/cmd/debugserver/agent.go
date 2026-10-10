package main

import (
	"context"
	"fmt"
	"time"

	"github.com/cnooljaw/gsr"
	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/protocol"
	"github.com/cnooljaw/gsr/runtime"
)

type echoAgent struct {
	client             gate.ClientSession
	protocolDispatcher *protocol.Dispatcher[gsr.CommandContext]
	commandDispatcher  *gsr.Dispatcher
}

func newEchoAgent(client gate.ClientSession, registry *protocol.Registry) *echoAgent {
	agent := &echoAgent{client: client}
	agent.commandDispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		pushEchoCommand: agent.handlePushEcho,
	})
	agent.protocolDispatcher = protocol.MustDispatcher[gsr.CommandContext](
		registry,
		client,
		map[uint32]any{
			uint32(echopb.MsgId_C2S_ECHO_REQUEST): agent.OnEcho,
			uint32(echopb.MsgId_C2S_ECHO_NOTIFY):  agent.OnEchoNotify,
		},
	)
	return agent
}

func newEchoRegistry() (*protocol.Registry, error) {
	return protocol.NewRegistry(
		echopb.MessageCatalogGsrExampleEchoV1Echo,
	)
}

func (agent *echoAgent) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

// Handle 接收 Service 发来的普通 Command；断线通知也走这个入口。
func (agent *echoAgent) Handle(
	self gsr.CommandContext,
	command runtime.CommandID,
	args ...any,
) (any, error) {
	if command == gate.ClientDisconnectedCommand {
		self.Exit()
		return nil, nil
	}
	return agent.commandDispatcher.Handle(self, command, args...)
}

// HandleClientMessage 接收客户端 Envelope，不把 msg_id 当作 Core CommandID。
func (agent *echoAgent) HandleClientMessage(self gsr.CommandContext, frame []byte) error {
	return agent.protocolDispatcher.HandleFrame(self, frame)
}

func (agent *echoAgent) OnEcho(self gsr.CommandContext, ctx protocol.Context, request *echopb.EchoRequest) error {
	wait, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result, err := self.CallName(wait, echoServiceName, echoCommand, request.GetText())
	if err != nil {
		return err
	}
	reply, ok := result.(echoResult)
	if !ok {
		return fmt.Errorf("unexpected Echo result: %T", result)
	}
	if !reply.Accepted {
		return ctx.Response(
			uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
			&echopb.EchoResponse{
				Code:    echopb.EchoResultCode_ECHO_RESULT_TEXT_REQUIRED,
				Message: "text is required",
			},
		)
	}
	return ctx.Response(
		uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
		&echopb.EchoResponse{
			Code: echopb.EchoResultCode_ECHO_RESULT_OK,
			Text: reply.Text,
		},
	)
}

// OnEchoNotify 只投递业务通知，不持有或等待客户端请求上下文。
func (agent *echoAgent) OnEchoNotify(self gsr.CommandContext, _ protocol.Context, request *echopb.EchoNotify) error {
	return self.SendName(echoServiceName, notifyEchoCommand, request.GetText())
}

// handlePushEcho 没有当前客户端请求，使用 Dispatcher 的独立 Push 能力写回。
func (agent *echoAgent) handlePushEcho(_ gsr.CommandContext, text string) error {
	return agent.protocolDispatcher.Push(
		uint32(echopb.MsgId_S2C_ECHO_PUSH),
		&echopb.EchoPush{Text: text},
	)
}

func (agent *echoAgent) OnStop(_ context.Context) error {
	return agent.client.Close()
}
