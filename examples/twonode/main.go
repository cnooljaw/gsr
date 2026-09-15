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

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	notifyCommand runtime.CommandID = 0x04100306
	echoCommand   runtime.CommandID = 0x04100307
	tickCommand   runtime.CommandID = 1 // 仅本地使用，不注册 wire codec。
)

type config struct {
	Role         string                    `json:"role"`
	LogDirectory string                    `json:"log_directory"`
	Listen       string                    `json:"listen"`
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
		if cfg.Listen == "" {
			return cfg, errors.New("server requires listen")
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

func run(ctx context.Context, cfg config) (result error) {
	logger, err := logging.New(logging.Config{Directory: cfg.LogDirectory})
	if err != nil {
		return err
	}
	defer func() {
		wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result = errors.Join(result, logger.Close(wait))
	}()
	local, err := runtime.NewRuntime(runtime.Config{Logger: logger})
	if err != nil {
		return err
	}
	codec, err := cluster.NewValueCodec(cluster.ValueCodecConfig{})
	if err != nil {
		return errors.Join(err, local.Close())
	}
	codecs := cluster.MustCommandCodecRegistry([]cluster.CommandCodecEntry{
		{CommandID: notifyCommand, Codec: codec},
		{CommandID: echoCommand, Codec: codec},
	})
	network, err := cluster.NewCluster(cluster.ClusterConfig{Nodes: cfg.Nodes, Codecs: codecs, Logger: logger})
	if err != nil {
		return errors.Join(err, local.Close())
	}
	defer func() {
		network.Quiesce()
		// 每阶段均尝试收尾；某阶段错误不能跳过后续资源释放。
		result = errors.Join(result, local.Close())
		wait, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		result = errors.Join(result, network.Close(wait))
		result = errors.Join(result, logger.Info("process stopped", "role", cfg.Role))
	}()
	if err := local.Start(); err != nil {
		return err
	}
	var listenAddress string
	if cfg.Role == "server" {
		id, err := local.CreateService(ctx, &echoService{log: logger})
		if err != nil {
			return err
		}
		listenAddress, err = network.Open(local, cluster.ListenConfig{Address: cfg.Listen,
			ResolveService: func(name cluster.ServiceName) (runtime.ServiceID, error) {
				if name == "echo" {
					return id, nil
				}
				return 0, runtime.ErrServiceNotFound
			}})
		if err != nil {
			return err
		}
	} else {
		if _, err := local.CreateService(ctx, &callerService{network: network, target: cfg.Target, log: logger}); err != nil {
			return err
		}
	}
	if err := logger.Info("process ready", "role", cfg.Role, "listen_address", listenAddress); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

type echoService struct{ log *logging.Logger }

func (service *echoService) OnStart(_ context.Context, ctx runtime.ServiceContext) error {
	service.log = service.log.WithService(uint64(ctx.Self()))
	return nil
}

func (service *echoService) Handle(_ runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if command != notifyCommand && command != echoCommand {
		return nil, runtime.ErrUnknownCommand
	}
	// 日志输出故障不替换业务结果；Logger.Err/Close 负责保留输出故障。
	_ = service.log.Info("received", "command", command, "args", args)
	return args, nil
}

func (service *echoService) OnStop(context.Context) error {
	return nil
}

type callerService struct {
	network   *cluster.Cluster
	client    *cluster.ClusterClient
	target    cluster.NodeID
	log       *logging.Logger
	operation uint64
}

func (service *callerService) OnStart(_ context.Context, ctx runtime.ServiceContext) error {
	client, err := service.network.Client(ctx.Self())
	if err != nil {
		return err
	}
	service.client = client
	service.log = service.log.WithService(uint64(ctx.Self()))
	_, err = ctx.After(0, tickCommand)
	return err
}

func (service *callerService) Handle(ctx runtime.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != tickCommand {
		return nil, runtime.ErrUnknownCommand
	}
	service.operation++
	target := cluster.ServiceTarget{ServiceName: "echo"}
	// 每轮是新的示例操作，不重试上一轮失败或结果不确定的业务请求。
	err := service.client.Send(service.target, target, notifyCommand, service.operation)
	_ = service.log.Info("Send admission", "operation", service.operation, "error", err)
	wait, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	value, callErr := service.client.Call(wait, service.target, target, echoCommand, service.operation)
	cancel()
	_ = service.log.Info("Call result", "operation", service.operation, "result", value, "error", callErr)
	_, err = ctx.After(3*time.Second, tickCommand)
	return nil, err
}

func (service *callerService) OnStop(context.Context) error {
	return nil
}
