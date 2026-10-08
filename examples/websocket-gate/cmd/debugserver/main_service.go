package main

import (
	"context"
	"crypto/tls"
	"fmt"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/runtime"
)

// mainService 打开所选 Gate，连接 Agent 由 Gate 按需创建。
type mainService struct {
	options serverOptions
}

func (service *mainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	registry, err := newEchoRegistry()
	if err != nil {
		return err
	}
	newAgent := func(client gate.ClientSession) gsr.Service {
		return &echoAgent{client: client, registry: registry}
	}
	var address string
	label, scheme := "DebugGate", "ws"
	if service.options.production {
		var tlsConfig *tls.Config
		if service.options.certificateFile != "" {
			certificate, err := tls.LoadX509KeyPair(service.options.certificateFile, service.options.privateKeyFile)
			if err != nil {
				return fmt.Errorf("load server TLS certificate: %w", err)
			}
			tlsConfig = &tls.Config{Certificates: []tls.Certificate{certificate}}
			scheme = "wss"
		}
		address, err = self.OpenProdGate(ctx, gsr.ProdGateConfig{
			Address:               service.options.address,
			TLSConfig:             tlsConfig,
			MaxConnections:        128,
			MaxOutboundQueueBytes: 1 << 20,
			NewAgent:              newAgent,
		})
		label = "ProdGate"
	} else {
		address, err = self.OpenDebugGate(ctx, gsr.DebugGateConfig{
			Address:               service.options.address,
			MaxConnections:        128,
			MaxOutboundQueueBytes: 1 << 20,
			NewAgent:              newAgent,
		})
	}
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
