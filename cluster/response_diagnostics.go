package cluster

// responseDiagnostics 只借用出口，不创建队列、执行回调或负责关闭。
// 将独立/共享出口的适配集中在此处，响应读循环只报告事件。
type responseDiagnostics struct {
	events chan<- responseReadFailure
	shared *clusterDiagnostics
}

// report 返回独立出口的丢弃数；共享出口自行累计，不重复计数。
func (diagnostics responseDiagnostics) report(event Diagnostic) uint64 {
	if diagnostics.shared != nil {
		diagnostics.shared.report(event)
		return 0
	}
	failure := responseReadFailure{node: event.RemoteNode, session: event.Session, err: event.Err}
	select {
	case diagnostics.events <- failure:
		return 0
	default:
		return 1
	}
}

// terminal 保留两种出口已有的语义：共享出口过滤预期关闭，独立出口报告全部终止错误。
// expectedClose 由连接 owner 根据关闭意图和原因判断，不从错误文本推测。
func (diagnostics responseDiagnostics) terminal(node NodeID, err error, expectedClose bool) uint64 {
	if err == nil || (expectedClose && diagnostics.shared != nil) {
		return 0
	}
	return diagnostics.report(Diagnostic{Kind: DiagnosticConnectionFailure, RemoteNode: node, Err: err})
}
