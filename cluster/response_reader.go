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
// 返回前只清理此连接的等待，不影响排队请求或其它连接。
// diagnostics 由调用方管理，必须保持打开直到本函数返回；建议使用有界缓冲。
// nil 或满载通道不会阻塞读取，返回值记录丢弃的诊断数；终止错误直接返回。
func (connection *streamConnection) receiveResponses(calls *connectionCalls, diagnostics chan<- responseReadFailure) (uint64, error) {
	return connection.receiveResponsesTo(calls, responseDiagnostics{events: diagnostics})
}

// receiveResponsesTo 通过借用的诊断出口直接上报，不创建队列或转发任务。
func (connection *streamConnection) receiveResponsesTo(calls *connectionCalls, diagnostics responseDiagnostics) (uint64, error) {
	if connection == nil {
		return 0, net.ErrClosed
	}
	if calls == nil || calls.registry == nil {
		return 0, ErrInvalidClusterCallRegistry
	}
	var dropped uint64
	for {
		envelope, err := connection.read()
		if err != nil {
			var complete *completeFrameError
			if !errors.As(err, &complete) {
				calls.fail(err)
				return dropped, err
			}
		} else {
			err = calls.complete(envelope)
		}
		if err == nil {
			continue
		}
		dropped += diagnostics.report(Diagnostic{Kind: DiagnosticResponseRejected, RemoteNode: calls.node,
			Session: envelope.Session, Target: envelope.Target, Command: envelope.Command, Err: err})
	}
}
