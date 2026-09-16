package gsr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// Config 声明应用运行环境；组件中的 Logger 必须为空，由 Logging 统一创建。
type Config struct {
	Logging logging.Config
	Runtime runtime.Config
	// Nodes 是共同节点表。端口 0 仅供本机测试 Open 分配端口，不作为出站路由。
	Cluster cluster.ClusterConfig
}

// Run 启动一个独立节点和普通入口 Service，等待 ctx 取消后统一收尾。
// 入口 Exit 不结束节点。信号处理归 main；运行中的 ctx 取消视为正常退出。
func Run(ctx context.Context, config Config, entry Service) (result error) {
	if ctx == nil {
		return errors.New("gsr: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if config.Runtime.Logger != nil || config.Cluster.Logger != nil {
		return errors.New("gsr: configure shared logger through Logging")
	}
	node := &application{names: make(map[cluster.ServiceName]runtime.ServiceID)}
	defer func() { result = errors.Join(result, node.close()) }()
	var err error
	node.nodes, config.Cluster.Nodes, err = splitNodeAddresses(config.Cluster.Nodes)
	if err != nil {
		return err
	}
	node.logger, err = logging.New(config.Logging)
	if err != nil {
		return err
	}
	config.Runtime.Logger = node.logger
	node.local, err = runtime.NewRuntime(config.Runtime)
	if err != nil {
		return err
	}
	config.Cluster.Logger = node.logger
	node.cluster, err = cluster.NewCluster(config.Cluster)
	if err != nil {
		return err
	}
	if err := node.local.Start(); err != nil {
		return err
	}
	wrapped, err := node.wrap(entry)
	if err != nil {
		return err
	}
	if _, err := node.local.CreateService(ctx, wrapped); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

// application 是固定组件 owner，仅存在于 Run 内部，不是公开的组装对象。
type application struct {
	local         *runtime.Runtime
	cluster       *cluster.Cluster
	logger        *logging.Logger
	nodes         map[cluster.NodeID]string
	mu            sync.RWMutex
	names         map[cluster.ServiceName]runtime.ServiceID
	listenAddress string
}

func (node *application) close() error {
	var result error
	if node.cluster != nil {
		node.cluster.Quiesce()
	}
	if node.local != nil {
		result = errors.Join(result, node.local.Close())
	}
	if node.cluster != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result = errors.Join(result, node.cluster.Close(ctx))
		cancel()
	}
	if node.logger != nil {
		result = errors.Join(result, node.logger.Info("process stopped"))
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result = errors.Join(result, node.logger.Close(ctx))
		cancel()
	}
	return result
}

// 端口零只供本节点 Open 动态监听，不能成为出站路由；正常部署使用固定非零端口。
func splitNodeAddresses(input map[cluster.NodeID]string) (map[cluster.NodeID]string, map[cluster.NodeID]string, error) {
	nodes, routes := make(map[cluster.NodeID]string), make(map[cluster.NodeID]string)
	for name, address := range input {
		host, port, err := net.SplitHostPort(address)
		number, parseErr := strconv.Atoi(port)
		if err != nil || parseErr != nil || name == "" || host == "" || strings.ContainsAny(host, "/\\ \t\r\n") || number < 0 || number > 65535 {
			return nil, nil, fmt.Errorf("%w: node %q", cluster.ErrInvalidClusterConfig, name)
		}
		for _, digit := range port {
			if digit < '0' || digit > '9' {
				return nil, nil, fmt.Errorf("%w: node %q port", cluster.ErrInvalidClusterConfig, name)
			}
		}
		nodes[name] = address
		if number != 0 {
			routes[name] = address
		}
	}
	return nodes, routes, nil
}
