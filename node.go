package gsr

import (
	"net"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/runtime"
)

// Node 提供所属节点的名称与接入组装能力，资源仍由运行环境或独立 Service 管理。
// 仅在取得它的当前生命周期回调或 Handler 中使用，不保存或借给异步任务。
// Node 不提供节点关闭、业务通信或内部组件访问能力。
type Node struct {
	node *application
}

// RegisterService 发布目标 Service 的节点内名称；注册者退出不撤销名称。
// 目标停止时由框架自动清理，重复名称不会静默改绑。
func (node *Node) RegisterService(name cluster.ServiceName, id runtime.ServiceID) error {
	return node.node.register(name, id)
}

// OpenCluster 从共同节点表取端口并绑定 0.0.0.0；仅开放入站，不改变调用者身份。
// 每节点最多一个监听；不 Open 仍可主动 Send/Call。失败由调用者处理。
func (node *Node) OpenCluster(name cluster.NodeID) error {
	address, exists := node.node.nodes[name]
	if !exists {
		return cluster.ErrNodeNotFound
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	actual, err := node.node.cluster.Open(node.node.local, cluster.ListenConfig{
		Address: net.JoinHostPort("0.0.0.0", port), ResolveService: node.node.resolve,
	})
	if err != nil {
		return err
	}
	node.node.mu.Lock()
	node.node.listenAddress = actual
	node.node.mu.Unlock()
	return nil
}

// ClusterListenAddress 返回实际绑定地址供运维观测；它不一定是其它节点可拨号的发布地址。
func (node *Node) ClusterListenAddress() string {
	node.node.mu.RLock()
	defer node.node.mu.RUnlock()
	return node.node.listenAddress
}
