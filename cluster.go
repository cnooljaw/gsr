package gsr

import (
	"context"
	"net"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/runtime"
)

// Cluster 是来源绑定的使用入口，共享节点的唯一 Cluster，不拥有独立连接或关闭责任。
// 只能通过当前 ServiceContext.Cluster 取得并在当前回调内使用。
type Cluster struct {
	node   *application
	client *cluster.ClusterClient
}

// Open 从共同节点表取端口并绑定 0.0.0.0；仅开放入站，不改变调用者身份。
// 每节点最多一个监听；不 Open 仍可主动 Send/Call。失败由调用者处理。
func (bound *Cluster) Open(name cluster.NodeID) error {
	address, exists := bound.node.nodes[name]
	if !exists {
		return cluster.ErrNodeNotFound
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	actual, err := bound.node.cluster.Open(bound.node.local, cluster.ListenConfig{
		Address: net.JoinHostPort("0.0.0.0", port), ResolveService: bound.node.resolve,
	})
	if err != nil {
		return err
	}
	bound.node.mu.Lock()
	bound.node.listenAddress = actual
	bound.node.mu.Unlock()
	return nil
}

// ListenAddress 返回实际绑定地址供运维观测；它不一定是其它节点可拨号的发布地址。
func (bound *Cluster) ListenAddress() string {
	bound.node.mu.RLock()
	defer bound.node.mu.RUnlock()
	return bound.node.listenAddress
}

// Send 发送到目标节点的命名 Service；nil 仅表示本地接纳。
func (bound *Cluster) Send(node cluster.NodeID, name cluster.ServiceName, command runtime.CommandID, args ...any) error {
	return bound.client.Send(node, cluster.ServiceTarget{ServiceName: name}, command, args...)
}

// Call 使用当前 Service 来源等待远端结果，等待期间当前 Service 不处理下一条消息。
func (bound *Cluster) Call(ctx context.Context, node cluster.NodeID, name cluster.ServiceName, command runtime.CommandID, args ...any) (any, error) {
	return bound.client.Call(ctx, node, cluster.ServiceTarget{ServiceName: name}, command, args...)
}
