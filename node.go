package gsr

import (
	"errors"
	"net"
	"sync"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/httpserver"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// Node 保存节点运行状态；Run 创建唯一对象并管理其生命周期，业务借用公开组装能力。
// 同一 Run 的 Service 共用此对象，不同 Run 相互隔离。
// 仅在取得它的当前生命周期回调或 Handler 中使用，不保存或借给异步任务。
// Node 不提供节点关闭、业务通信或内部组件访问能力。
type Node struct {
	local         *runtime.Runtime
	cluster       *cluster.Cluster
	logger        *logging.Logger
	nodeAddresses map[cluster.NodeID]string
	mu            sync.RWMutex
	names         map[cluster.ServiceName]runtime.ServiceID
	namesByID     map[runtime.ServiceID][]cluster.ServiceName
	active        map[runtime.ServiceID]struct{}
	listenAddress string
	httpMu        sync.Mutex
	httpServer    *httpserver.Server
	httpClosed    bool
}

// ErrServiceNameRegistered 表示本节点已有同名发布，不会静默替换。
var ErrServiceNameRegistered = errors.New("gsr: service name already registered")

// RegisterService 发布目标 Service 的节点内名称；注册者退出不撤销名称。
// 目标停止时由框架自动清理，重复名称不会静默改绑。
func (node *Node) RegisterService(name cluster.ServiceName, id runtime.ServiceID) error {
	if name == "" || id == 0 {
		return runtime.ErrServiceNotFound
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	if _, exists := node.active[id]; !exists {
		return runtime.ErrServiceNotFound
	}
	if _, exists := node.names[name]; exists {
		return ErrServiceNameRegistered
	}
	node.names[name] = id
	node.namesByID[id] = append(node.namesByID[id], name)
	return nil
}

// OpenCluster 从共同节点表取端口并绑定 0.0.0.0；仅开放入站，不改变调用者身份。
// 每节点最多一个监听；不 Open 仍可主动 Send/Call。失败由调用者处理。
func (node *Node) OpenCluster(name cluster.NodeID) error {
	address, exists := node.nodeAddresses[name]
	if !exists {
		return cluster.ErrNodeNotFound
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	actual, err := node.cluster.Open(node.local, cluster.ListenConfig{
		Address: net.JoinHostPort("0.0.0.0", port), ResolveService: node.resolve,
	})
	if err != nil {
		return err
	}
	node.mu.Lock()
	node.listenAddress = actual
	node.mu.Unlock()
	return nil
}

// ClusterListenAddress 返回实际绑定地址供运维观测；它不一定是其它节点可拨号的发布地址。
func (node *Node) ClusterListenAddress() string {
	node.mu.RLock()
	defer node.mu.RUnlock()
	return node.listenAddress
}

func (node *Node) activateID(id runtime.ServiceID) {
	node.mu.Lock()
	node.active[id] = struct{}{}
	node.mu.Unlock()
}

func (node *Node) resolve(name cluster.ServiceName) (runtime.ServiceID, error) {
	node.mu.RLock()
	defer node.mu.RUnlock()
	id, exists := node.names[name]
	if !exists {
		return 0, runtime.ErrServiceNotFound
	}
	return id, nil
}

func (node *Node) unregisterID(id runtime.ServiceID) {
	node.mu.Lock()
	defer node.mu.Unlock()
	delete(node.active, id)
	for _, name := range node.namesByID[id] {
		delete(node.names, name)
	}
	delete(node.namesByID, id)
}
