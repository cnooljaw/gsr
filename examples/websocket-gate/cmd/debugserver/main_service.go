package main

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/runtime"
)

// mainService 创建命名业务 Service 并打开 Gate，连接 Agent 由 Gate 按需创建。
type mainService struct {
	options serverOptions
}

func (service *mainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	registry, err := newEchoRegistry()
	if err != nil {
		return err
	}
	echoID, err := self.CreateService(ctx, newEchoService())
	if err != nil {
		return err
	}
	if err := self.Register(echoServiceName, echoID); err != nil {
		return err
	}
	newAgent := func(client gate.ClientSession) gsr.Service {
		return newEchoAgent(client, registry)
	}
	config := gsr.GateConfig{
		Protocol:              gate.ProtocolPlain,
		Address:               service.options.address,
		MaxConnections:        128,
		MaxOutboundQueueBytes: 1 << 20,
		NewAgent:              newAgent,
	}
	label, scheme := "Plain Gate", "ws"
	if service.options.production {
		config.Protocol = gate.ProtocolMAC
		if service.options.certificateFile != "" {
			certificate, err := tls.LoadX509KeyPair(service.options.certificateFile, service.options.privateKeyFile)
			if err != nil {
				return fmt.Errorf("load server TLS certificate: %w", err)
			}
			config.TLSConfig = &tls.Config{Certificates: []tls.Certificate{certificate}}
			scheme = "wss"
		}
		label = "MAC Gate"
	}
	address, err := self.OpenGate(ctx, config)
	if err != nil {
		return err
	}
	fmt.Printf("%s listening at %s://%s/ws; press Ctrl-C to stop\n", label, scheme, address)
	self.Exit()
	return nil
}

func (service *mainService) Handle(gsr.CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *mainService) OnStop(context.Context) error {
	return nil
}
