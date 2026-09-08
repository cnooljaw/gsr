package cluster

import "sync/atomic"

// inboundFailure 只携带连接地址、Session 和错误，不持有业务载荷或来源 NodeID。
// 远端地址是诊断标签，不是身份认证凭据。
type inboundFailure struct {
	remoteAddress string
	session       ClusterSessionID
	err           error
}

// inboundDiagnostics 是 listener 与全部入站连接共享的有界、非阻塞诊断出口。
// 不执行外部 callback，也不创建 goroutine；满载丢弃由所有生产者统一计数。
// 创建方必须等全部生产者退出后才能关闭 failures；单连接不能关闭共享出口。
type inboundDiagnostics struct {
	failures chan inboundFailure
	dropped  atomic.Uint64
}

func newInboundDiagnostics() *inboundDiagnostics {
	return &inboundDiagnostics{failures: make(chan inboundFailure, defaultFailureQueueCapacity)}
}

func (diagnostics *inboundDiagnostics) report(failure inboundFailure) {
	select {
	case diagnostics.failures <- failure:
	default:
		diagnostics.dropped.Add(1)
	}
}
