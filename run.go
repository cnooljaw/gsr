package gsr

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// Run 启动一个独立节点和普通入口 Service，等待 ctx 取消后统一收尾。
// 入口 Exit 不结束节点。信号处理归 main；运行中的 ctx 取消视为正常退出。
func Run(ctx context.Context, config Config, entry Service) (result error) {
	if ctx == nil {
		return errors.New("gsr: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	node := &Node{
		names:     make(map[cluster.ServiceName]runtime.ServiceID),
		namesByID: make(map[runtime.ServiceID][]cluster.ServiceName),
		active:    make(map[runtime.ServiceID]struct{}),
	}
	defer func() { result = errors.Join(result, node.close()) }()
	var err error
	node.nodeAddresses, config.Cluster.Nodes, err = splitNodeAddresses(config.Cluster.Nodes)
	if err != nil {
		return err
	}
	node.logger, err = logging.New(config.Logging)
	if err != nil {
		return err
	}
	node.local, err = runtime.NewRuntime(config.Runtime.core(node.logger))
	if err != nil {
		return err
	}
	node.cluster, err = cluster.NewCluster(config.Cluster.outbound(node.logger))
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

func (node *Node) close() error {
	var result error
	node.httpMu.Lock()
	node.httpClosed = true
	server := node.httpServer
	node.httpMu.Unlock()
	if server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result = errors.Join(result, server.Close(ctx))
		cancel()
	}
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
	addresses, routes := make(map[cluster.NodeID]string), make(map[cluster.NodeID]string)
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
		addresses[name] = address
		if number != 0 {
			routes[name] = address
		}
	}
	return addresses, routes, nil
}
