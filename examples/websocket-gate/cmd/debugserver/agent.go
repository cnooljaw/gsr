package main

import (
	"context"
	"errors"

	"github.com/cnooljaw/gsr/gate"
	commonv1 "github.com/cnooljaw/gsr/gate/gen/go/gsr/common/v1"
	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/gate/protocol"
	"github.com/cnooljaw/gsr/runtime"
)

type echoAgent struct {
	client     gate.ClientSession
	registry   *protocol.Registry
	dispatcher *protocol.Dispatcher
}

func newEchoRegistry() (*protocol.Registry, error) {
	return protocol.NewRegistry(
		commonv1.MessageCatalogGsrCommonV1Common,
		echopb.MessageCatalogGsrExampleEchoV1Echo,
	)
}

func (agent *echoAgent) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	if agent.registry == nil {
		return errors.New("debugserver: protocol registry is required")
	}
	session, err := protocol.NewSession(agent.registry, agent.client)
	if err != nil {
		return err
	}
	dispatcher, err := protocol.NewDispatcher(agent.registry, session)
	if err != nil {
		return err
	}
	if err := protocol.Register(dispatcher, agent.OnEcho); err != nil {
		return err
	}
	if err := protocol.Register(dispatcher, agent.OnEchoNotify); err != nil {
		return err
	}
	agent.dispatcher = dispatcher
	return nil
}

func (*echoAgent) Handle(
	commandContext runtime.CommandContext,
	command runtime.CommandID,
	_ ...any,
) (any, error) {
	if command != gate.ClientDisconnectedCommand {
		return nil, runtime.ErrUnknownCommand
	}
	commandContext.Exit()
	return nil, nil
}

func (agent *echoAgent) HandleClientMessage(_ runtime.CommandContext, frame []byte) error {
	if agent.dispatcher == nil {
		return errors.New("debugserver: protocol dispatcher is not initialized")
	}
	return agent.dispatcher.HandleFrame(frame)
}

func (*echoAgent) OnEcho(context protocol.Context, request *echopb.EchoRequest) error {
	if request.GetText() == "" {
		return context.Response(
			uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
			&echopb.EchoResponse{
				Code:    echopb.EchoResultCode_ECHO_RESULT_TEXT_REQUIRED,
				Message: "text is required",
			},
		)
	}
	return context.Response(
		uint32(echopb.MsgId_S2C_ECHO_RESPONSE),
		&echopb.EchoResponse{
			Code: echopb.EchoResultCode_ECHO_RESULT_OK,
			Text: request.GetText(),
		},
	)
}

// OnEchoNotify 将不要求响应的客户端通知转换为一次服务端主动推送。
func (*echoAgent) OnEchoNotify(context protocol.Context, request *echopb.EchoNotify) error {
	return context.Push(
		uint32(echopb.MsgId_S2C_ECHO_PUSH),
		&echopb.EchoPush{Text: request.GetText()},
	)
}

func (agent *echoAgent) OnStop(context.Context) error {
	return agent.client.Close()
}
