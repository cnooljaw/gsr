package cluster

import (
	"errors"
	"net"
)

// responseReadFailure 只记录出站目标节点、Session 和失败原因，不保留业务载荷。
type responseReadFailure struct {
	node    NodeID
	session ClusterSessionID
	err     error
}

// receiveResponses 在绑定目标 NodeID 的出站连接上同步读取响应，直到连接结束。
// node 来自本地目标路由，不表示已通过握手认证；不要求本地拥有 NodeID。
// 外层 Transport 负责启动且仅启动一个读循环，通过 close 取消，并等待返回；
// 返回前完成该节点旧连接的等待清理，之后才能开放替代连接上的请求。
// diagnostics 由调用方管理，必须保持打开直到本函数返回；建议使用有界缓冲。
// nil 或满载通道不会阻塞读取，返回值记录丢弃的诊断数；终止错误直接返回。
func (connection *streamConnection) receiveResponses(node NodeID, calls *ClusterCallRegistry, diagnostics chan<- responseReadFailure) (uint64, error) {
	if connection == nil {
		return 0, net.ErrClosed
	}
	if node == "" {
		return 0, ErrInvalidNode
	}
	if !calls.valid() {
		return 0, ErrInvalidClusterCallRegistry
	}
	var dropped uint64
	for {
		envelope, err := connection.read()
		if err != nil {
			var complete *completeFrameError
			if !errors.As(err, &complete) {
				_, cleanupErr := calls.failNode(node, err)
				return dropped, errors.Join(err, cleanupErr)
			}
		} else {
			err = calls.Complete(node, envelope)
		}
		if err == nil {
			continue
		}
		failure := responseReadFailure{node: node, session: envelope.Session, err: err}
		select {
		case diagnostics <- failure:
		default:
			dropped++
		}
	}
}
