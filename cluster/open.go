package cluster

import (
	"context"
	"errors"
	"net"

	"github.com/cnooljaw/gsr/runtime"
)

// ErrClusterAlreadyOpen 表示已有监听或正在执行一次打开流程。
var ErrClusterAlreadyOpen = errors.New("cluster: listener already open or opening")

// clusterInbound 持有唯一监听任务；done 关闭后才可读取 result。
type clusterInbound struct {
	listener *tcpListener
	cancel   context.CancelFunc
	done     chan struct{}
	result   error
}

// Open 打开唯一监听，返回实际绑定地址，不注册节点或管理目标 Runtime 生命周期。
// 失败可由应用重试；成功后不允许再次打开。停止后的 Cluster 拒绝 Open。
func (cluster *Cluster) Open(target *runtime.Runtime, config ListenConfig) (string, error) {
	if cluster == nil || cluster.calls == nil {
		return "", ErrInvalidClusterConfig
	}
	if target == nil {
		return "", ErrInvalidListenConfig
	}
	config, err := config.normalized()
	if err != nil {
		return "", err
	}
	cluster.mu.Lock()
	if cluster.quiescing {
		cluster.mu.Unlock()
		return "", ErrClusterQuiescing
	}
	if cluster.inbound != nil || cluster.opening != nil {
		cluster.mu.Unlock()
		return "", ErrClusterAlreadyOpen
	}
	opening := make(chan struct{})
	cluster.opening = opening
	cluster.mu.Unlock()
	// 解析与绑定不持有 Cluster 锁，不能阻塞其它目标的 admission。
	defer func() {
		cluster.mu.Lock()
		cluster.opening = nil
		close(opening)
		cluster.mu.Unlock()
	}()
	address, err := net.ResolveTCPAddr("tcp", config.Address)
	if err != nil {
		return "", err
	}
	socket, err := net.ListenTCP("tcp", address)
	if err != nil {
		return "", err
	}
	listener, err := newTCPListener(tcpListenerConfig{
		listener: socket, codec: cluster.frames,
		adapter:        &localRequestAdapter{runtime: target, codecs: cluster.config.Codecs, resolve: config.ResolveService},
		maxConnections: config.MaxConnections, maxCalls: config.MaxConcurrentCalls,
		writeTimeout: cluster.config.WriteTimeout, sharedDiagnostics: cluster.diagnostics,
	})
	if err != nil {
		return "", errors.Join(err, socket.Close())
	}
	cluster.mu.Lock()
	defer cluster.mu.Unlock()
	if cluster.quiescing {
		return "", errors.Join(ErrClusterQuiescing, socket.Close())
	}
	ctx, cancel := context.WithCancel(context.Background())
	inbound := &clusterInbound{listener: listener, cancel: cancel, done: make(chan struct{})}
	cluster.inbound = inbound
	go inbound.serve(ctx)
	return socket.Addr().String(), nil
}

func (inbound *clusterInbound) serve(ctx context.Context) {
	defer close(inbound.done)
	inbound.result = inbound.listener.serve(ctx)
	if errors.Is(inbound.result, context.Canceled) {
		inbound.result = nil
	}
	if inbound.result != nil {
		inbound.listener.diagnostics.report(inboundFailure{err: inbound.result})
	}
}
