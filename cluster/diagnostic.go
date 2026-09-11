package cluster

import (
	"sync/atomic"

	"github.com/cnooljaw/gsr/logging"

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
	// DiagnosticInboundFailure 表示入站接收、适配、回复或监听失败。
	DiagnosticInboundFailure
)

// Diagnostic 只保存出站目标或入站对端地址、可信关联字段和错误链，不保留业务载荷。
// 无单个请求归属时关联字段为零；Err 文本不承诺自动脱敏。
type Diagnostic struct {
	Kind       DiagnosticKind
	RemoteNode NodeID
	// RemoteAddress 是入站对端地址，不是 NodeID 或认证身份；监听故障时可为空。
	RemoteAddress string
	Session       ClusterSessionID
	Target        ServiceTarget
	Command       runtime.CommandID
	Err           error
}

// clusterDiagnostics 由 Cluster 持有；所有生产者退出后才允许关闭。
type clusterDiagnostics struct {
	logger  *logging.Logger
	events  chan Diagnostic
	dropped atomic.Uint64
}

func newClusterDiagnostics() *clusterDiagnostics {
	return &clusterDiagnostics{events: make(chan Diagnostic, defaultFailureQueueCapacity)}
}

func (diagnostics *clusterDiagnostics) report(event Diagnostic) {
	// 直接在诊断产生路径入主日志队列，保留 Diagnostics 作为独立观测流。
	// 主日志满载由 Logger 计数，不回流为新的诊断或改变网络结果。
	_ = diagnostics.logger.Error("Cluster failure", "kind", event.Kind,
		"remote_node", event.RemoteNode, "remote_address", event.RemoteAddress,
		"session", event.Session, "target", event.Target, "command", event.Command, "error", event.Err)
	select {
	case diagnostics.events <- event:
	default:
		diagnostics.dropped.Add(1)
	}
}
