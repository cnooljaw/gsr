package cluster

import (
	"errors"
	"fmt"
	"sync"
)

const defaultMaxPayload = 4 << 20

// Receiver 处理投递给已注册节点的消息封包。
// 封包是由 Transport 持有的值快照，接收器可以修改它。
type Receiver func(envelope ClusterEnvelope)

// Transport 在节点间传递 ClusterEnvelope 值，
// 不暴露 Core Service、Mailbox、Session 或连接实现细节。
// 它只要求主动发送能力，to 必须非空；不要求注册接收器或开放监听。
// 具体实现的生命周期由外层 owner 管理，不由 NodeSender 关闭共享 Transport。
type Transport interface {
	Send(to NodeID, envelope ClusterEnvelope) error
}

// InMemoryTransportConfig 配置确定性的进程内传输。
type InMemoryTransportConfig struct {
	// MaxPayload 限制 Send 接受的封包载荷大小，零值使用默认上限。
	MaxPayload int
}

func (config InMemoryTransportConfig) normalized() (InMemoryTransportConfig, error) {
	if config.MaxPayload == 0 {
		config.MaxPayload = defaultMaxPayload
	}
	if config.MaxPayload < 0 {
		return InMemoryTransportConfig{}, fmt.Errorf("%w: max payload is negative", ErrInvalidTransportConfig)
	}
	return config, nil
}

var (
	// ErrInvalidTransportConfig 表示内存传输配置无效。
	ErrInvalidTransportConfig = errors.New("cluster: invalid transport config")
	// ErrInvalidNode 表示节点 ID 或接收器无效。
	ErrInvalidNode = errors.New("cluster: invalid node")
	// ErrDuplicateNode 表示节点已在此 Transport 中注册接收器。
	ErrDuplicateNode = errors.New("cluster: duplicate node")
	// ErrNodeNotFound 表示目标节点尚未注册接收器。
	ErrNodeNotFound = errors.New("cluster: node not found")
	// ErrTransportClosed 表示 Transport 已停止接纳操作。
	ErrTransportClosed = errors.New("cluster: transport is closed")
)

// InMemoryTransport 只按目标同步投递复制后的消息封包，不持有调用方身份。
// 它是用于确定性测试的替换边界，不是生产网络传输实现。
type InMemoryTransport struct {
	mu         sync.RWMutex
	maxPayload int
	closed     bool
	receivers  map[NodeID]Receiver
}

// NewInMemoryTransport 构造确定性的进程内传输。
func NewInMemoryTransport(config InMemoryTransportConfig) (*InMemoryTransport, error) {
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	return &InMemoryTransport{
		maxPayload: normalized.MaxPayload,
		receivers:  make(map[NodeID]Receiver),
	}, nil
}

// Register 为测试目标开放接收能力；主动 Send 不要求调用方先注册。
func (transport *InMemoryTransport) Register(node NodeID, receiver Receiver) error {
	if transport == nil || node == "" || receiver == nil {
		return ErrInvalidNode
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.closed {
		return ErrTransportClosed
	}
	if _, exists := transport.receivers[node]; exists {
		return ErrDuplicateNode
	}
	transport.receivers[node] = receiver
	return nil
}

// Unregister 移除节点接收器；对尚未关闭的 Transport，此操作幂等。
func (transport *InMemoryTransport) Unregister(node NodeID) error {
	if transport == nil || node == "" {
		return ErrInvalidNode
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.closed {
		return ErrTransportClosed
	}
	delete(transport.receivers, node)
	return nil
}

// Send 校验消息封包，并将其副本同步投递给目标节点。
func (transport *InMemoryTransport) Send(to NodeID, envelope ClusterEnvelope) error {
	if transport == nil {
		return ErrTransportClosed
	}
	if to == "" {
		return ErrInvalidNode
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	if len(envelope.Payload) > transport.maxPayload {
		return fmt.Errorf("%w: got %d, limit %d", ErrPayloadTooLarge, len(envelope.Payload), transport.maxPayload)
	}

	transport.mu.RLock()
	if transport.closed {
		transport.mu.RUnlock()
		return ErrTransportClosed
	}
	receiver, exists := transport.receivers[to]
	transport.mu.RUnlock()
	if !exists {
		return ErrNodeNotFound
	}
	receiver(envelope.Clone())
	return nil
}

// Close 阻止后续注册与投递，此操作幂等。
func (transport *InMemoryTransport) Close() error {
	if transport == nil {
		return nil
	}
	transport.mu.Lock()
	transport.closed = true
	transport.receivers = nil
	transport.mu.Unlock()
	return nil
}
