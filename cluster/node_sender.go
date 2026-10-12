package cluster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const (
	defaultOutboundQueueCapacity = 1024
	defaultFailureQueueCapacity  = 256
)

// DeliveryFailure 描述本地接纳后的基础设施失败。
// 仅包含路由元数据，不携带业务载荷。
type DeliveryFailure struct {
	RemoteNode NodeID
	Kind       FrameKind
	Session    ClusterSessionID
	Target     ServiceTarget
	Command    runtime.CommandID
	Err        error
}

// NodeSenderConfig 配置对应单个远端 NodeID 的出站发送器。
type NodeSenderConfig struct {
	RemoteNode    NodeID
	Transport     Transport
	QueueCapacity int
}

var (
	// ErrInvalidNodeSenderConfig 表示 NodeSender 配置无效。
	ErrInvalidNodeSenderConfig = errors.New("cluster: invalid node sender config")
	// ErrNodeSenderClosed 表示 NodeSender 已停止接纳新消息。
	ErrNodeSenderClosed = errors.New("cluster: node sender is closed")
	// ErrOutboundQueueFull 表示有界发送队列拒绝消息入队。
	ErrOutboundQueueFull = errors.New("cluster: outbound queue is full")
	// ErrTransportPanicked 表示 Transport 在发送消息封包时发生 panic。
	ErrTransportPanicked = errors.New("cluster: transport panicked")
)

// NodeSender 是本地节点到某个远端节点的唯一出站管理组件。
// 它将复制后的消息封包接纳到有界队列，并串行投递。
type NodeSender struct {
	remoteNode        NodeID
	transport         Transport
	queue             chan ClusterEnvelope
	failures          chan DeliveryFailure
	stop              chan struct{}
	done              chan struct{}
	closeOnce         sync.Once
	mu                sync.Mutex
	closed            bool
	droppedFailures   atomic.Uint64
	sharedDiagnostics *clusterDiagnostics
}

var _ OutboundSender = (*NodeSender)(nil)

// NewNodeSender 构造并启动一个使用有界队列的出站发送 worker。
func NewNodeSender(config NodeSenderConfig) (*NodeSender, error) {
	return newNodeSender(config, nil)
}

func newNodeSender(config NodeSenderConfig, diagnostics *clusterDiagnostics) (*NodeSender, error) {
	if config.RemoteNode == "" || isNilTransport(config.Transport) {
		return nil, fmt.Errorf("%w: remote node and Transport are required", ErrInvalidNodeSenderConfig)
	}
	if config.QueueCapacity == 0 {
		config.QueueCapacity = defaultOutboundQueueCapacity
	}
	if config.QueueCapacity < 1 {
		return nil, fmt.Errorf("%w: QueueCapacity must be greater than zero", ErrInvalidNodeSenderConfig)
	}
	sender := &NodeSender{
		remoteNode:        config.RemoteNode,
		transport:         config.Transport,
		queue:             make(chan ClusterEnvelope, config.QueueCapacity),
		sharedDiagnostics: diagnostics,
		stop:              make(chan struct{}),
		done:              make(chan struct{}),
	}
	if diagnostics == nil {
		sender.failures = make(chan DeliveryFailure, defaultFailureQueueCapacity)
	}
	go sender.run()
	return sender, nil
}

// Send 将复制后的消息封包接纳到有界出站队列。
// 返回 nil 仅表示本地接纳成功；后续基础设施失败通过 Failures 发布，
// 不调用业务代码。
func (sender *NodeSender) Send(envelope ClusterEnvelope) error {
	if sender == nil {
		return ErrNodeSenderClosed
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	ownedEnvelope := envelope.Clone()

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.closed {
		return ErrNodeSenderClosed
	}
	select {
	case sender.queue <- ownedEnvelope:
		return nil
	default:
		return ErrOutboundQueueFull
	}
}

// Failures 返回接纳后基础设施失败的有界事件流。
// 慢消费者不会阻塞发送器，超出容量的事件会被丢弃。
func (sender *NodeSender) Failures() <-chan DeliveryFailure {
	if sender == nil {
		return nil
	}
	return sender.failures
}

// DroppedFailureCount 返回因有界失败事件流已满而丢弃的诊断事件数。
// nil 发送器返回零。
func (sender *NodeSender) DroppedFailureCount() uint64 {
	if sender == nil {
		return 0
	}
	return sender.droppedFailures.Load()
}

// Close 停止接纳，将队列中的消息报告为失败，并等待当前 Transport 调用结束，
// 最多等到 ctx 到期。取消 ctx 不会取消 worker；后续 Close 调用可继续等待它退出。
func (sender *NodeSender) Close(ctx context.Context) error {
	if sender == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sender.stopAdmission()
	select {
	case <-sender.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// stopAdmission 只关闭接纳和发布停止意图，不等待 Transport。
func (sender *NodeSender) stopAdmission() {
	sender.closeOnce.Do(func() {
		sender.mu.Lock()
		sender.closed = true
		close(sender.stop)
		sender.mu.Unlock()
	})
}

func (sender *NodeSender) run() {
	defer close(sender.done)
	if sender.failures != nil {
		defer close(sender.failures)
	}
	for {
		select {
		case <-sender.stop:
			sender.failQueued()
			return
		case envelope := <-sender.queue:
			select {
			case <-sender.stop:
				sender.tryReportFailure(envelope, ErrNodeSenderClosed)
				sender.failQueued()
				return
			default:
			}
			if err := sender.sendTransport(envelope); err != nil {
				sender.tryReportFailure(envelope, err)
			}
		}
	}
}

func (sender *NodeSender) sendTransport(envelope ClusterEnvelope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", ErrTransportPanicked, recovered)
		}
	}()
	return sender.transport.Send(sender.remoteNode, envelope)
}

func (sender *NodeSender) tryReportFailure(envelope ClusterEnvelope, err error) {
	if sender.sharedDiagnostics != nil {
		sender.sharedDiagnostics.report(Diagnostic{Kind: DiagnosticDeliveryFailure,
			RemoteNode: sender.remoteNode, Session: envelope.Session, Target: envelope.Target,
			Command: envelope.Command, Err: err})
		return
	}
	failure := DeliveryFailure{
		RemoteNode: sender.remoteNode,
		Kind:       envelope.Kind,
		Session:    envelope.Session,
		Target:     envelope.Target,
		Command:    envelope.Command,
		Err:        err,
	}
	select {
	case sender.failures <- failure:
	default:
		sender.droppedFailures.Add(1)
	}
}

func (sender *NodeSender) failQueued() {
	for {
		select {
		case envelope := <-sender.queue:
			sender.tryReportFailure(envelope, ErrNodeSenderClosed)
		default:
			return
		}
	}
}

func isNilTransport(transport Transport) bool {
	if transport == nil {
		return true
	}
	value := reflect.ValueOf(transport)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
