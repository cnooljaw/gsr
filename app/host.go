package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// Config 声明应用的组件配置；Logger 统一由 Logging 创建，不接受组件中的外部 Logger。
type Config struct {
	Logging logging.Config
	Runtime runtime.Config
	Cluster cluster.ClusterConfig
	// Commands 使用默认 ValueCodec；与 Cluster.Codecs 互斥，重复注册属于编程错误。
	Commands []runtime.CommandID
}

// Host 拥有固定的三个组件，不支持插件注册或运行后重新启动。
// 创建后应调用 Run，或在放弃启动时调用 Close；不要与 Run 并发调用 Close。
type Host struct {
	local     *runtime.Runtime
	network   *cluster.Cluster
	logger    *logging.Logger
	run       atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

// New 完成组装，失败时回滚已经创建的资源；不启动 Runtime、不监听或拨号。
func New(config Config) (_ *Host, result error) {
	if config.Runtime.Logger != nil || config.Cluster.Logger != nil {
		return nil, errors.New("app: configure shared logger through Logging")
	}
	if config.Cluster.Codecs != nil && len(config.Commands) != 0 {
		return nil, errors.New("app: Commands and Cluster.Codecs are mutually exclusive")
	}
	if config.Cluster.Codecs == nil {
		codec, err := cluster.NewValueCodec(cluster.ValueCodecConfig{})
		if err != nil {
			return nil, err
		}
		entries := make([]cluster.CommandCodecEntry, len(config.Commands))
		for index, command := range config.Commands {
			entries[index] = cluster.CommandCodecEntry{CommandID: command, Codec: codec}
		}
		// 静态协议声明先校验；panic 前尚未分配任何运行资源。
		config.Cluster.Codecs = cluster.MustCommandCodecRegistry(entries)
	}
	host := &Host{}
	defer func() {
		if result != nil {
			result = errors.Join(result, host.Close())
		}
	}()
	var err error
	host.logger, err = logging.New(config.Logging)
	if err != nil {
		return nil, err
	}
	config.Runtime.Logger = host.logger
	host.local, err = runtime.NewRuntime(config.Runtime)
	if err != nil {
		return nil, err
	}
	config.Cluster.Logger = host.logger
	host.network, err = cluster.NewCluster(config.Cluster)
	if err != nil {
		return nil, err
	}
	return host, nil
}

// Logger 返回共享日志入口，仅供写日志，不由业务关闭。
func (host *Host) Logger() *logging.Logger {
	return host.logger
}

// Cluster 返回共享通信组件；宿主保留其最终关闭责任。
func (host *Host) Cluster() *cluster.Cluster {
	return host.network
}

// Open 绑定宿主 Runtime 打开监听，不要求业务持有 Runtime。
func (host *Host) Open(config cluster.ListenConfig) (string, error) {
	return host.network.Open(host.local, config)
}

// Run 启动 Runtime 和普通入口 Service，等待 ctx 取消，然后按固定顺序收尾。
// ctx 取消是正常进程退出信号；入口退出不等同于应用退出。只能调用一次。
func (host *Host) Run(ctx context.Context, entry runtime.Service) (result error) {
	if !host.run.CompareAndSwap(false, true) {
		return errors.New("app: Run already called")
	}
	defer func() { result = errors.Join(result, host.Close()) }()
	if ctx == nil {
		return errors.New("app: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := host.local.Start(); err != nil {
		return err
	}
	if _, err := host.local.CreateService(ctx, entry); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

// Close 幂等地停止接纳、关闭 Runtime、Cluster，最后排空日志；错误不跳过后续步骤。
// Runtime 使用自身关闭预算，Cluster 和 Logger 每阶段使用独立的五秒预算。
func (host *Host) Close() error {
	host.closeOnce.Do(func() {
		if host.network != nil {
			host.network.Quiesce()
		}
		if host.local != nil {
			host.closeErr = errors.Join(host.closeErr, host.local.Close())
		}
		if host.network != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			host.closeErr = errors.Join(host.closeErr, host.network.Close(ctx))
			cancel()
		}
		if host.logger != nil {
			host.closeErr = errors.Join(host.closeErr, host.logger.Info("process stopped"))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			host.closeErr = errors.Join(host.closeErr, host.logger.Close(ctx))
			cancel()
		}
	})
	return host.closeErr
}
