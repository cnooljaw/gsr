package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/cnooljaw/gsr"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	options, err := parseServerOptions(os.Args[1:])
	if err != nil {
		return err
	}

	return gsr.Run(ctx, gsr.Config{}, &mainService{options: options})
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
