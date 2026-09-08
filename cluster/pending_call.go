package cluster

import (
	"errors"
	"fmt"
	"sync"

	runtime "github.com/cnooljaw/gsr/runtime"
)

var (
	// ErrClusterCallClosed 表示 Cluster 的待完成调用管理组件已停止。
	ErrClusterCallClosed = errors.New("cluster: pending calls are closed")
	// ErrClusterSessionNotFound 表示响应未匹配到待完成调用。
	ErrClusterSessionNotFound = errors.New("cluster: session not found")
	// ErrClusterResponseMismatch 表示响应与请求路由不匹配。
	ErrClusterResponseMismatch = errors.New("cluster: response does not match request")
	// ErrInvalidClusterCall 表示待完成调用的注册信息无效。
	ErrInvalidClusterCall = errors.New("cluster: invalid call")
	// ErrInvalidClusterResponse 表示用于完成调用的帧不是有效响应。
	ErrInvalidClusterResponse = errors.New("cluster: invalid response")
)

type clusterCallResult struct {
	payload []byte
	err     error
}

// ClusterCallRegistry 是由 Cluster 管理的调用完成边界，
// 供同一 Cluster 中所有绑定来源身份的 ClusterClient 实例共享。
//
// Session 分配和待完成调用由此统一管理，不归各 Service 客户端所有。
// Transport 或 ConnectionAgent 通过 Complete 完成响应，业务代码不直接访问等待记录。
// 必须通过 NewClusterCallRegistry 构造；零值不是可用的注册表。
type ClusterCallRegistry struct {
	pending *clusterPendingCalls
}

var (
	// ErrInvalidClusterCallRegistry 表示调用注册表接收者无效。
	ErrInvalidClusterCallRegistry = errors.New("cluster: invalid call registry")
)

// NewClusterCallRegistry 创建可用的 Cluster 级待完成调用管理组件。
func NewClusterCallRegistry() *ClusterCallRegistry {
	return &ClusterCallRegistry{pending: newClusterPendingCalls()}
}

func (registry *ClusterCallRegistry) valid() bool {
	return registry != nil && registry.pending != nil
}

// Complete 将一个远端 Response 或 Error 匹配到对应的待完成调用。
//
// remoteNode 来自本地出站连接绑定的目标路由，不从消息封包读取，也不是入站来源身份。
// 此逻辑适配入口只完成未绑定网络连接的等待；网络读循环必须通过连接 owner 完成。
// 不匹配或未知的 Session 会被拒绝，不会完成其它调用。
func (registry *ClusterCallRegistry) Complete(remoteNode NodeID, envelope ClusterEnvelope) error {
	if !registry.valid() {
		return ErrInvalidClusterCallRegistry
	}
	return registry.pending.complete(remoteNode, envelope)
}

// Close 使用 ErrClusterCallClosed 完成所有剩余调用，并阻止注册新 Session。
func (registry *ClusterCallRegistry) Close() error {
	if !registry.valid() {
		return ErrInvalidClusterCallRegistry
	}
	return registry.pending.close(nil)
}

// PendingCount 返回仍在等待响应的调用数，用于生命周期检查与诊断。
func (registry *ClusterCallRegistry) PendingCount() int {
	if !registry.valid() {
		return 0
	}
	return registry.pending.pendingCount()
}

func (registry *ClusterCallRegistry) begin(source runtime.ServiceID, node NodeID, target ServiceTarget, command runtime.CommandID) (*clusterPendingCall, error) {
	if !registry.valid() {
		return nil, ErrInvalidClusterCallRegistry
	}
	return registry.pending.begin(source, node, target, command)
}

func (registry *ClusterCallRegistry) cancel(session ClusterSessionID, err error) bool {
	if !registry.valid() {
		return false
	}
	return registry.pending.cancel(session, err)
}

type clusterPendingCall struct {
	owner   *connectionCalls
	session ClusterSessionID
	source  runtime.ServiceID
	node    NodeID
	target  ServiceTarget
	command runtime.CommandID
	done    chan clusterCallResult
}

// clusterPendingCalls 管理单个 Cluster 实例的 ClusterSessionID 分配与响应完成。
// 它不执行网络 I/O，也不直接唤醒 Runtime runner；这些接线由调用方负责。
type clusterPendingCalls struct {
	mu      sync.Mutex
	next    ClusterSessionID
	pending map[ClusterSessionID]*clusterPendingCall
	closed  bool
}

func newClusterPendingCalls() *clusterPendingCalls {
	return &clusterPendingCalls{
		next:    1,
		pending: make(map[ClusterSessionID]*clusterPendingCall),
	}
}

func (registry *clusterPendingCalls) begin(source runtime.ServiceID, node NodeID, target ServiceTarget, command runtime.CommandID) (*clusterPendingCall, error) {
	if registry == nil || source == 0 || node == "" || target.validate() != nil || command == 0 {
		return nil, fmt.Errorf("%w: source, node, target, and command are required", ErrInvalidClusterCall)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return nil, ErrClusterCallClosed
	}
	if registry.next == 0 {
		return nil, runtime.ErrIDExhausted
	}
	session := registry.next
	if session == ^ClusterSessionID(0) {
		registry.next = 0
	} else {
		registry.next++
	}
	pending := &clusterPendingCall{
		session: session,
		source:  source,
		node:    node,
		target:  target,
		command: command,
		done:    make(chan clusterCallResult, 1),
	}
	registry.pending[session] = pending
	return pending, nil
}

func (registry *clusterPendingCalls) complete(remoteNode NodeID, envelope ClusterEnvelope) error {
	return registry.completeOwned(remoteNode, envelope, nil)
}

func (registry *clusterPendingCalls) completeOwned(remoteNode NodeID, envelope ClusterEnvelope, owner *connectionCalls) error {
	if registry == nil {
		return ErrClusterSessionNotFound
	}
	if remoteNode == "" {
		return fmt.Errorf("%w: remote node is empty", ErrInvalidClusterResponse)
	}
	if envelope.Kind != FrameResponse && envelope.Kind != FrameError {
		return fmt.Errorf("%w: frame kind %s cannot complete a call", ErrInvalidClusterResponse, envelope.Kind)
	}
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidClusterResponse, err)
	}

	registry.mu.Lock()
	pending, exists := registry.pending[envelope.Session]
	if !exists {
		registry.mu.Unlock()
		return fmt.Errorf("%w: session %d", ErrClusterSessionNotFound, envelope.Session)
	}
	if pending.owner != owner || pending.node != remoteNode || pending.target != envelope.Target || pending.command != envelope.Command {
		registry.mu.Unlock()
		return fmt.Errorf("%w: session %d", ErrClusterResponseMismatch, envelope.Session)
	}
	delete(registry.pending, envelope.Session)
	registry.mu.Unlock()

	result := clusterCallResult{}
	if envelope.Kind == FrameResponse {
		result.payload = append([]byte(nil), envelope.Payload...)
	} else {
		status := *envelope.Error
		result.err = &status
	}
	finishClusterPendingCall(pending, result)
	return nil
}

func (registry *clusterPendingCalls) cancel(session ClusterSessionID, err error) bool {
	if registry == nil || session == 0 {
		return false
	}
	registry.mu.Lock()
	pending, exists := registry.pending[session]
	if exists {
		delete(registry.pending, session)
	}
	registry.mu.Unlock()
	if !exists {
		return false
	}
	finishClusterPendingCall(pending, clusterCallResult{err: err})
	return true
}

func (registry *clusterPendingCalls) close(err error) error {
	if registry == nil {
		return nil
	}
	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		return nil
	}
	registry.closed = true
	pending := make([]*clusterPendingCall, 0, len(registry.pending))
	for _, call := range registry.pending {
		pending = append(pending, call)
	}
	registry.pending = make(map[ClusterSessionID]*clusterPendingCall)
	registry.mu.Unlock()
	if err == nil {
		err = ErrClusterCallClosed
	}
	for _, call := range pending {
		finishClusterPendingCall(call, clusterCallResult{err: err})
	}
	return nil
}

func (registry *clusterPendingCalls) pendingCount() int {
	if registry == nil {
		return 0
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return len(registry.pending)
}

func finishClusterPendingCall(pending *clusterPendingCall, result clusterCallResult) {
	pending.done <- result
	close(pending.done)
}
