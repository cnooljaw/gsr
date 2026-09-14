package cluster

import (
	"github.com/cnooljaw/gsr/runtime"
	"sync/atomic"
)

// inboundFailure 只携带连接地址、可信请求字段和错误，不持有业务载荷或来源 NodeID。
// 远端地址是诊断标签，不是身份认证凭据。
type inboundFailure struct {
	remoteAddress string
	session       ClusterSessionID
	target        ServiceTarget
	command       runtime.CommandID
	err           error
	suppressed    uint64
}

// inboundDiagnostics 是 listener 与全部入站连接共享的有界、非阻塞诊断出口。
// 不执行外部 callback，也不创建 goroutine；满载丢弃由所有生产者统一计数。
// 创建方必须等全部生产者退出后才能关闭 failures；单连接不能关闭共享出口。
type inboundDiagnostics struct {
	failures chan inboundFailure
	dropped  atomic.Uint64
	shared   *clusterDiagnostics
}

func newInboundDiagnostics() *inboundDiagnostics {
	return &inboundDiagnostics{failures: make(chan inboundFailure, defaultFailureQueueCapacity)}
}

func (diagnostics *inboundDiagnostics) report(failure inboundFailure) {
	if diagnostics.shared != nil {
		kind := DiagnosticInboundFailure
		if failure.suppressed > 0 {
			kind = DiagnosticBadFramesSuppressed
		}
		diagnostics.shared.report(Diagnostic{Kind: kind, RemoteAddress: failure.remoteAddress,
			Session: failure.session, Target: failure.target, Command: failure.command, Err: failure.err, Suppressed: failure.suppressed})
		return
	}
	select {
	case diagnostics.failures <- failure:
	default:
		diagnostics.dropped.Add(1)
	}
}
