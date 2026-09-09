package cluster

import "net"

// connectionCalls 标记一次出站连接的等待归属，不拥有 socket 或 goroutine。
// failure 与 PendingCall 的绑定均由 registry.mu 保护；对象不复用、不进入 wire。
type connectionCalls struct {
	registry *clusterPendingCalls
	node     NodeID
	failure  error
}

func newConnectionCalls(registry *ClusterCallRegistry, node NodeID) (*connectionCalls, error) {
	if !registry.valid() {
		return nil, ErrInvalidClusterCallRegistry
	}
	if node == "" {
		return nil, ErrInvalidNode
	}
	return &connectionCalls{registry: registry.pending, node: node}, nil
}

// bind 在获得写入权后执行；成功绑定是发送尝试的线性化点。
// false,nil 表示 Call 已取消或完成，调用方应跳过发送；Send 不创建等待记录。
func (owner *connectionCalls) bind(request ClusterEnvelope) (bool, error) {
	if request.Kind != FrameRequest {
		return false, ErrInvalidEnvelope
	}
	if err := request.Validate(); err != nil {
		return false, err
	}
	registry := owner.registry
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return false, ErrClusterCallClosed
	}
	if registry.quiescing {
		return false, ErrClusterQuiescing
	}
	if owner.failure != nil {
		return false, owner.failure
	}
	if request.Session == 0 {
		return true, nil
	}
	call, exists := registry.pending[request.Session]
	if !exists {
		return false, nil
	}
	if call.owner != nil || call.node != owner.node || call.target != request.Target || call.command != request.Command {
		return false, ErrClusterResponseMismatch
	}
	call.owner = owner
	return true, nil
}

func (owner *connectionCalls) complete(response ClusterEnvelope) error {
	return owner.registry.completeOwned(owner.node, response, owner)
}

// pending 在拨号前检查未绑定等待，不改变连接归属；实际写入仍须再次 bind。
func (owner *connectionCalls) pending(request ClusterEnvelope) (bool, error) {
	if request.Kind != FrameRequest {
		return false, ErrInvalidEnvelope
	}
	if err := request.Validate(); err != nil {
		return false, err
	}
	registry := owner.registry
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return false, ErrClusterCallClosed
	}
	if registry.quiescing {
		return false, ErrClusterQuiescing
	}
	if request.Session == 0 {
		return true, nil
	}
	call := registry.pending[request.Session]
	if call == nil {
		return false, nil
	}
	if call.owner != nil || call.node != owner.node || call.target != request.Target || call.command != request.Command {
		return false, ErrClusterResponseMismatch
	}
	return true, nil
}

// reject 只结束当前尚未绑定的请求，不把一次发送接纳失败扩散到同节点其它 Call。
func (owner *connectionCalls) reject(request ClusterEnvelope, cause error) {
	registry := owner.registry
	registry.mu.Lock()
	call := registry.pending[request.Session]
	if call == nil || call.owner != nil || call.node != owner.node || call.target != request.Target || call.command != request.Command {
		registry.mu.Unlock()
		return
	}
	delete(registry.pending, request.Session)
	registry.mu.Unlock()
	finishClusterPendingCall(call, clusterCallResult{err: cause})
}

// fail 与新增绑定互斥，只取走当前连接的等待；完成通知在锁外发送。
func (owner *connectionCalls) fail(cause error) int {
	if cause == nil {
		cause = net.ErrClosed
	}
	registry := owner.registry
	registry.mu.Lock()
	if owner.failure != nil {
		registry.mu.Unlock()
		return 0
	}
	owner.failure = cause
	var failed []*clusterPendingCall
	for session, call := range registry.pending {
		if call.owner == owner {
			delete(registry.pending, session)
			failed = append(failed, call)
		}
	}
	registry.mu.Unlock()
	for _, call := range failed {
		finishClusterPendingCall(call, clusterCallResult{err: cause})
	}
	return len(failed)
}
