package cluster

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"

	"github.com/cnooljaw/gsr/runtime"
)

var (
	// ErrInvalidClusterConfig 表示公开出站组装配置无效。
	ErrInvalidClusterConfig = errors.New("cluster: invalid cluster config")
	// ErrClusterQuiescing 表示 Cluster 已停止接纳，不允许新调用或来源绑定。
	ErrClusterQuiescing = errors.New("cluster: admission stopped")
	// ErrInvalidClusterCloseContext 表示 Close 缺少等待 context。
	ErrInvalidClusterCloseContext = errors.New("cluster: close context is nil")
)

// Cluster 统一管理出站路由、共享等待表和每目标发送组件。
// 必须通过 NewCluster 构造；可选管理入站监听，不管理 Runtime 或业务 Service 生命周期。
type Cluster struct {
	config         ClusterConfig
	frames         *FrameCodec
	calls          *ClusterCallRegistry
	diagnostics    *clusterDiagnostics
	mu             sync.Mutex
	quiescing      bool
	opening        chan struct{} // 当前打开流程退出后关闭，Close 等待其回滚或移交。
	inbound        *clusterInbound
	listenCloseErr error
	targets        map[NodeID]*clusterTarget
	closeOnce      sync.Once
	done           chan struct{}
	closeErr       error // 只在 done 关闭后读取。
	// dial 仅为包内测试替换标准拨号器，不是公开配置能力。
	dial func(context.Context, string, string) (net.Conn, error)
}

type clusterTarget struct {
	cluster *Cluster
	sender  *NodeSender
	peer    *outboundPeer
}

// NewCluster 校验并复制配置，不拨号、监听或启动发送任务。
func NewCluster(config ClusterConfig) (*Cluster, error) {
	config, err := config.normalized()
	if err != nil {
		return nil, err
	}
	frames, err := NewFrameCodec(FrameCodecConfig{MaxFrame: config.MaxFrame})
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidClusterConfig, err)
	}
	diagnostics := newClusterDiagnostics()
	diagnostics.logger = config.Logger
	return &Cluster{config: config, frames: frames, calls: NewClusterCallRegistry(),
		diagnostics: diagnostics, targets: make(map[NodeID]*clusterTarget), done: make(chan struct{})}, nil
}

// Client 是受信任组装入口，只绑定非零来源值，不校验 Service 存活。
// 客户端共享资源，无需独立关闭；Quiesce 后拒绝创建。
func (cluster *Cluster) Client(source runtime.ServiceID) (*ClusterClient, error) {
	if cluster == nil || cluster.calls == nil {
		return nil, ErrInvalidClusterConfig
	}
	cluster.mu.Lock()
	defer cluster.mu.Unlock()
	if cluster.quiescing {
		return nil, ErrClusterQuiescing
	}
	return NewClusterClient(ClusterClientConfig{Source: source, Senders: clusterResolver{cluster},
		Codec: cluster.config.Codec, PendingCalls: cluster.calls, MaxCallWait: cluster.config.MaxCallWait})
}

// clusterResolver 隐藏 sender 管理能力，不额外导出 Cluster 的解析接口。
type clusterResolver struct{ cluster *Cluster }

func (resolver clusterResolver) ResolveNodeSender(node NodeID) (OutboundSender, error) {
	cluster := resolver.cluster
	cluster.mu.Lock()
	defer cluster.mu.Unlock()
	if cluster.quiescing {
		return nil, ErrClusterQuiescing
	}
	address, exists := cluster.config.Nodes[node]
	if !exists {
		return nil, ErrNodeNotFound
	}
	if target := cluster.targets[node]; target != nil {
		return target, nil
	}
	peer, err := newOutboundPeer(outboundPeerConfig{node: node, address: address,
		codec: cluster.frames, calls: cluster.calls, dialTimeout: cluster.config.DialTimeout,
		writeTimeout: cluster.config.WriteTimeout, dial: cluster.dial, sharedDiagnostics: cluster.diagnostics})
	if err != nil {
		return nil, err
	}
	sender, err := newNodeSender(NodeSenderConfig{RemoteNode: node, Transport: peer,
		QueueCapacity: cluster.config.OutboundQueueCapacity}, cluster.diagnostics)
	if err != nil {
		return nil, errors.Join(err, peer.close())
	}
	target := &clusterTarget{cluster: cluster, sender: sender, peer: peer}
	cluster.targets[node] = target
	return target, nil
}

func (target *clusterTarget) Send(request ClusterEnvelope) error {
	// 解析和 admission 之间可能发生 Quiesce，实际入队仍需同一个门禁。
	target.cluster.mu.Lock()
	defer target.cluster.mu.Unlock()
	if target.cluster.quiescing {
		return ErrClusterQuiescing
	}
	return target.sender.Send(request)
}

// Quiesce 同步停止入站与出站接纳、关闭监听、取消未绑定 Call 并阻止新拨号。
// 不等待任务或主动关闭已建立连接；监听关闭错误由 Close 合并返回。
// 已绑定 Call 保留原等待上限，不能通过此方法恢复接纳。
func (cluster *Cluster) Quiesce() {
	if cluster == nil || cluster.calls == nil {
		return
	}
	cluster.mu.Lock()
	defer cluster.mu.Unlock()
	if cluster.quiescing {
		return
	}
	cluster.quiescing = true
	if cluster.inbound != nil {
		cluster.listenCloseErr = cluster.inbound.listener.quiesce()
	}
	cluster.calls.pending.quiesce()
	for _, target := range cluster.targets {
		target.sender.stopAdmission()
		target.peer.quiesce()
	}
}

// Close 发起唯一最终清理并等待；ctx 只控制本次等待，取消不停止清理。
// 所有目标先收到关闭信号再等待，诊断出口最后关闭；不等待日志消费者。
func (cluster *Cluster) Close(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidClusterCloseContext
	}
	if cluster == nil || cluster.calls == nil {
		return ErrInvalidClusterConfig
	}
	cluster.Quiesce()
	cluster.closeOnce.Do(func() { go cluster.finishClose() })
	select {
	case <-cluster.done:
		return cluster.closeErr
	default:
	}
	select {
	case <-cluster.done:
		return cluster.closeErr
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (cluster *Cluster) finishClose() {
	// Quiesce 后 targets 不再增删；发送与读循环不需要 Cluster.mu。
	cluster.calls.pending.close(ErrClusterQuiescing)
	var failures []error
	cluster.mu.Lock()
	opening := cluster.opening
	inbound := cluster.inbound
	failures = append(failures, cluster.listenCloseErr)
	cluster.mu.Unlock()
	if inbound != nil {
		inbound.cancel()
	}
	for _, target := range cluster.targets {
		failures = append(failures, target.peer.signalClose())
	}
	for _, target := range cluster.targets {
		<-target.sender.done
		target.peer.waitClosed()
	}
	if opening != nil {
		<-opening
	}
	if inbound != nil {
		<-inbound.done
		failures = append(failures, inbound.result)
	}
	cluster.closeErr = errors.Join(failures...)
	close(cluster.diagnostics.events)
	close(cluster.done)
}

// Diagnostics 返回共享单消费流，不是广播；关闭后缓冲事件仍可读取。
func (cluster *Cluster) Diagnostics() <-chan Diagnostic {
	if cluster == nil || cluster.diagnostics == nil {
		return nil
	}
	return cluster.diagnostics.events
}

// SuppressedDiagnosticCount 返回重复坏帧被源头抑制的累计次数，关闭后仍可读取。
func (cluster *Cluster) SuppressedDiagnosticCount() uint64 {
	if cluster == nil || cluster.diagnostics == nil {
		return 0
	}
	return cluster.diagnostics.suppressed.Load()
}

// DroppedDiagnosticCount 返回队列满导致的累计丢弃数，关闭后仍可读取。
func (cluster *Cluster) DroppedDiagnosticCount() uint64 {
	if cluster == nil || cluster.diagnostics == nil {
		return 0
	}
	return cluster.diagnostics.dropped.Load()
}
