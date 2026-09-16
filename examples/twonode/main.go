package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	notifyCommand runtime.CommandID = 0x04100306
	echoCommand   runtime.CommandID = 0x04100307
	tickCommand   runtime.CommandID = 1
)

type config struct {
	Role         string                    `json:"role"`
	LogDirectory string                    `json:"log_directory"`
	Listen       cluster.NodeID            `json:"listen"`
	Target       cluster.NodeID            `json:"target"`
	Nodes        map[cluster.NodeID]string `json:"nodes"`
}

func decodeConfig(reader io.Reader) (config, error) {
	var cfg config
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return cfg, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return cfg, errors.New("config must contain exactly one JSON object")
	}
	if cfg.LogDirectory == "" {
		return cfg, errors.New("log_directory is required")
	}
	switch cfg.Role {
	case "server":
		if cfg.Listen == "" || cfg.Nodes[cfg.Listen] == "" {
			return cfg, errors.New("server requires a listen node in nodes")
		}
	case "caller":
		if cfg.Listen != "" || cfg.Target == "" || cfg.Nodes[cfg.Target] == "" {
			return cfg, errors.New("caller requires a target route and must not listen")
		}
	default:
		return cfg, errors.New("role must be server or caller")
	}
	return cfg, nil
}

func main() {
	path := flag.String("config", "", "进程 JSON 配置文件")
	flag.Parse()
	if err := runFile(*path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	cfg, decodeErr := decodeConfig(file)
	if err := errors.Join(decodeErr, file.Close()); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, cfg)
}

func run(ctx context.Context, cfg config) error {
	return gsr.Run(ctx, gsr.Config{
		Logging: logging.Config{Directory: cfg.LogDirectory},
		Cluster: cluster.ClusterConfig{Nodes: cfg.Nodes},
	}, &mainService{cfg: cfg})
}

// mainService 是一次性应用入口，不持有 Runtime，也不拥有新建 Service 的生命周期。
type mainService struct {
	cfg config
}

func (service *mainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	cluster := self.Cluster()
	if service.cfg.Role == "server" {
		id, err := self.CreateService(ctx, &echoService{})
		if err != nil {
			return err
		}
		if err := cluster.Register("echo", id); err != nil {
			return err
		}
		if err := cluster.Open(service.cfg.Listen); err != nil {
			return err
		}
	} else {
		if _, err := self.CreateService(ctx, &callerService{
			target: service.cfg.Target,
		}); err != nil {
			return err
		}
	}
	if err := self.Logger().Info("process ready", "role", service.cfg.Role, "listen_address", cluster.ListenAddress()); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (service *mainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *mainService) OnStop(_ context.Context) error {
	return nil
}

type echoService struct{}

func (service *echoService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *echoService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if command != notifyCommand && command != echoCommand {
		return nil, runtime.ErrUnknownCommand
	}
	// 日志输出故障不替换业务结果；Logger.Err/Close 负责保留输出故障。
	_ = ctx.Logger().Info("received", "command", command, "args", args)
	return args, nil
}

func (service *echoService) OnStop(context.Context) error {
	return nil
}

type callerService struct {
	target    cluster.NodeID
	operation uint64
}

func (service *callerService) OnStart(_ context.Context, self gsr.ServiceContext) error {
	_, err := self.After(0, tickCommand)
	return err
}

func (service *callerService) Handle(ctx gsr.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != tickCommand {
		return nil, runtime.ErrUnknownCommand
	}
	service.operation++
	cluster := ctx.Cluster()
	// 每轮是新的示例操作，不重试上一轮失败或结果不确定的业务请求。
	err := cluster.Send(service.target, "echo", notifyCommand, service.operation)
	_ = ctx.Logger().Info("Send admission", "operation", service.operation, "error", err)
	wait, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	value, callErr := cluster.Call(wait, service.target, "echo", echoCommand, service.operation)
	cancel()
	_ = ctx.Logger().Info("Call result", "operation", service.operation, "result", value, "error", callErr)
	_, err = ctx.After(3*time.Second, tickCommand)
	return nil, err
}

func (service *callerService) OnStop(context.Context) error {
	return nil
}
