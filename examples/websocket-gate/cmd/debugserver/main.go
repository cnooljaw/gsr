package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cnooljaw/gsr/examples/websocket-gate/debuggate"
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

	address := "127.0.0.1:7888"
	if len(os.Args) == 3 && os.Args[1] == "--addr" {
		address = os.Args[2]
	} else if len(os.Args) != 1 {
		return errors.New("usage: debugserver [--addr 127.0.0.1:7888]")
	}

	gate, err := debuggate.NewGateService(local, debuggate.Config{
		Address:               address,
		MaxConnections:        128,
		MaxOutboundQueueBytes: 1 << 20,
		NewAgent: func(client debuggate.ClientSession) runtime.Service {
			return &echoAgent{client: client, registry: registry}
		},
	})
	if err != nil {
		return err
	}
	if err := local.Start(); err != nil {
		return err
	}
	if _, err := local.CreateService(context.Background(), gate); err != nil {
		return err
	}

	fmt.Printf("DebugGate listening at ws://%s/ws; press Ctrl-C to stop\n", gate.Addr())
	<-ctx.Done()
	return nil
}
