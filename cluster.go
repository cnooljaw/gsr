package gsr

import (
	"context"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/runtime"
)

// Cluster 是来源绑定的使用入口，共享节点的唯一 Cluster，不拥有独立连接或关闭责任。
// 只能通过当前 ServiceContext.Cluster 取得并在当前回调内使用。
type Cluster struct {
	client *cluster.ClusterClient
}

// Send 发送到目标节点的命名 Service；nil 仅表示本地接纳。
func (bound *Cluster) Send(node cluster.NodeID, name cluster.ServiceName, command runtime.CommandID, args ...any) error {
	return bound.client.Send(node, cluster.ServiceTarget{ServiceName: name}, command, args...)
}

// Call 使用当前 Service 来源等待远端结果，等待期间当前 Service 不处理下一条消息。
func (bound *Cluster) Call(ctx context.Context, node cluster.NodeID, name cluster.ServiceName, command runtime.CommandID, args ...any) (any, error) {
	return bound.client.Call(ctx, node, cluster.ServiceTarget{ServiceName: name}, command, args...)
}
