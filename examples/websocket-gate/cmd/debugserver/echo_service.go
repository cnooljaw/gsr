package main

import (
	"context"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	echoServiceName                     = "echo"
	echoCommand       runtime.CommandID = 1
	notifyEchoCommand runtime.CommandID = 2
	pushEchoCommand   runtime.CommandID = 3
)

// echoResult 是本地 Call 的独占值结果，不依赖客户端 Protobuf 协议。
type echoResult struct {
	Text     string
	Accepted bool
}

// echoService 处理 Echo 业务；只通过消息与 Agent 通信，不持有 Agent 实例或连接。
type echoService struct {
	dispatcher *gsr.Dispatcher
}

func newEchoService() *echoService {
	service := &echoService{}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		echoCommand:       service.handleEcho,
		notifyEchoCommand: service.handleNotifyEcho,
	})
	return service
}

func (service *echoService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *echoService) Handle(self gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(self, command, args...)
}

func (service *echoService) handleEcho(_ gsr.CommandContext, text string) (echoResult, error) {
	return echoResult{Text: text, Accepted: text != ""}, nil
}

func (service *echoService) handleNotifyEcho(self gsr.CommandContext, text string) error {
	// Source 是发来通知的 AgentID；Send 只表示接纳，客户端写回仍由 Agent 负责。
	return self.Send(self.Source(), pushEchoCommand, text)
}

func (service *echoService) OnStop(_ context.Context) error {
	return nil
}
