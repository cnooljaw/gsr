package cluster

import (
	"sync/atomic"

	"github.com/cnooljaw/gsr/runtime"
)

// DiagnosticKind 区分基础设施诊断边界，不代替错误链分类。
type DiagnosticKind uint8

const (
	// DiagnosticDeliveryFailure 表示本地接纳后的投递失败。
	DiagnosticDeliveryFailure DiagnosticKind = iota + 1
	// DiagnosticConnectionFailure 表示已建立连接异常终止。
	DiagnosticConnectionFailure
	// DiagnosticResponseRejected 表示完整响应或帧被拒绝。
	DiagnosticResponseRejected
)

// Diagnostic 只保存出站目标、可信关联字段和错误链，不保留业务载荷。
// 无单个请求归属时关联字段为零；Err 文本不承诺自动脱敏。
type Diagnostic struct {
	Kind       DiagnosticKind
	RemoteNode NodeID
	Session    ClusterSessionID
	Target     ServiceTarget
	Command    runtime.CommandID
	Err        error
}

// clusterDiagnostics 由 Cluster 持有；所有生产者退出后才允许关闭。
type clusterDiagnostics struct {
	events  chan Diagnostic
	dropped atomic.Uint64
}

func newClusterDiagnostics() *clusterDiagnostics {
	return &clusterDiagnostics{events: make(chan Diagnostic, defaultFailureQueueCapacity)}
}

func (diagnostics *clusterDiagnostics) report(event Diagnostic) {
	select {
	case diagnostics.events <- event:
	default:
		diagnostics.dropped.Add(1)
	}
}
