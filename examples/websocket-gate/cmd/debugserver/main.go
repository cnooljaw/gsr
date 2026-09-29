package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cnooljaw/gsr/examples/websocket-gate/debuggate"
	"github.com/cnooljaw/gsr/examples/websocket-gate/gate"
	"github.com/cnooljaw/gsr/examples/websocket-gate/prodgate"
	"github.com/cnooljaw/gsr/runtime"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (result error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	registry, err := newEchoRegistry()
	if err != nil {
		return err
	}
	local, err := runtime.NewRuntime(runtime.Config{})
	if err != nil {
		return err
	}
	defer func() {
		result = errors.Join(result, local.Close())
	}()

	options, err := parseServerOptions(os.Args[1:])
	if err != nil {
		return err
	}

	var service runtime.Service
	var label string
	scheme := "ws"
	if options.production {
		var tlsConfig *tls.Config
		if options.certificateFile != "" {
			certificate, err := tls.LoadX509KeyPair(options.certificateFile, options.privateKeyFile)
			if err != nil {
				return fmt.Errorf("load server TLS certificate: %w", err)
			}
			tlsConfig = &tls.Config{Certificates: []tls.Certificate{certificate}}
			scheme = "wss"
		}
		service, err = prodgate.NewGateService(local, prodgate.Config{
			Address:               options.address,
			TLSConfig:             tlsConfig,
			MaxConnections:        128,
			MaxOutboundQueueBytes: 1 << 20,
			NewAgent: func(client gate.ClientSession) runtime.Service {
				return &echoAgent{client: client, registry: registry}
			},
		})
		label = "ProdGate"
	} else {
		service, err = debuggate.NewGateService(local, debuggate.Config{
			Address:               options.address,
			MaxConnections:        128,
			MaxOutboundQueueBytes: 1 << 20,
			NewAgent: func(client gate.ClientSession) runtime.Service {
				return &echoAgent{client: client, registry: registry}
			},
		})
		label = "DebugGate"
	}
	if err != nil {
		return err
	}
	if err := local.Start(); err != nil {
		return err
	}
	if _, err := local.CreateService(context.Background(), service); err != nil {
		return err
	}

	fmt.Printf("%s listening at %s://%s/ws; press Ctrl-C to stop\n", label, scheme, serviceAddress(service))
	<-ctx.Done()
	return nil
}

type serverOptions struct {
	address         string
	production      bool
	certificateFile string
	privateKeyFile  string
}

const serverUsage = "usage: debugserver [--prod] [--addr 127.0.0.1:7888] [--tls-cert FILE --tls-key FILE]"

// parseServerOptions 解析示例服务的监听模式和地址，不引入通用配置框架。
func parseServerOptions(arguments []string) (serverOptions, error) {
	options := serverOptions{address: "127.0.0.1:7888"}
	addressSet := false
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--prod":
			options.production = true
		case "--addr":
			if index+1 >= len(arguments) || arguments[index+1] == "" {
				return serverOptions{}, errors.New(serverUsage)
			}
			options.address = arguments[index+1]
			addressSet = true
			index++
		case "--tls-cert":
			if index+1 >= len(arguments) || arguments[index+1] == "" {
				return serverOptions{}, errors.New(serverUsage)
			}
			options.certificateFile = arguments[index+1]
			index++
		case "--tls-key":
			if index+1 >= len(arguments) || arguments[index+1] == "" {
				return serverOptions{}, errors.New(serverUsage)
			}
			options.privateKeyFile = arguments[index+1]
			index++
		default:
			return serverOptions{}, errors.New(serverUsage)
		}
	}
	if (options.certificateFile == "") != (options.privateKeyFile == "") ||
		(!options.production && options.certificateFile != "") {
		return serverOptions{}, errors.New(serverUsage)
	}
	if options.production && !addressSet {
		options.address = "127.0.0.1:7889"
	}
	return options, nil
}

func serviceAddress(service runtime.Service) string {
	switch service := service.(type) {
	case *debuggate.GateService:
		return service.Addr().String()
	case *prodgate.GateService:
		return service.Addr().String()
	default:
		return "unknown"
	}
}
