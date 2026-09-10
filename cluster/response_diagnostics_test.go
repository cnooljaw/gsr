package cluster

import (
	"errors"
	"net"
	"testing"
)

// 验证响应诊断只写入选定出口，共享模式保留完整关联字段且独立队列不被双写。
func TestResponseDiagnosticsSelectsOneDestination(t *testing.T) {
	standalone := make(chan responseReadFailure, 1)
	shared := newClusterDiagnostics()
	reporter := responseDiagnostics{events: standalone, shared: shared}
	event := Diagnostic{Kind: DiagnosticResponseRejected, RemoteNode: "node-b", Session: 7, Target: ServiceTarget{ServiceID: 9}, Command: 10, Err: ErrClusterSessionNotFound}
	if reporter.report(event) != 0 {
		t.Fatal("shared event counted as standalone drop")
	}
	got := <-shared.events
	if got != event {
		t.Fatalf("shared event=%+v", got)
	}
	if len(standalone) != 0 {
		t.Fatal("event was duplicated")
	}
	reporter.shared = nil
	if reporter.report(event) != 0 {
		t.Fatal("standalone event dropped")
	}
	local := <-standalone
	if local.node != event.RemoteNode || local.session != event.Session || !errors.Is(local.err, event.Err) {
		t.Fatalf("standalone event=%+v", local)
	}
}

// 验证满载及 nil 出口不阻塞，丢弃次数只归属实际使用的出口。
func TestResponseDiagnosticsDropOwnership(t *testing.T) {
	event := Diagnostic{Kind: DiagnosticResponseRejected, Err: ErrClusterSessionNotFound}
	standalone := make(chan responseReadFailure, 1)
	reporter := responseDiagnostics{events: standalone}
	if reporter.report(event) != 0 || reporter.report(event) != 1 {
		t.Fatal("standalone drop count")
	}
	if (responseDiagnostics{}).report(event) != 1 {
		t.Fatal("nil stream drop count")
	}
	shared := newClusterDiagnostics()
	reporter.shared = shared
	for index := 0; index < defaultFailureQueueCapacity+3; index++ {
		if reporter.report(event) != 0 {
			t.Fatal("shared drop leaked to standalone count")
		}
	}
	if shared.dropped.Load() != 3 || len(standalone) != 1 {
		t.Fatal("wrong drop owner or duplicate writes")
	}
}

// 验证共享出口过滤主动关闭，独立出口保留原终止事件，异常关闭仍上报。
func TestResponseDiagnosticsTerminalPolicy(t *testing.T) {
	standalone := make(chan responseReadFailure, 2)
	shared := newClusterDiagnostics()
	reporter := responseDiagnostics{events: standalone, shared: shared}
	if reporter.terminal("node-b", net.ErrClosed, true) != 0 || len(shared.events) != 0 {
		t.Fatal("expected close reported")
	}
	reporter.terminal("node-b", net.ErrClosed, false)
	event := <-shared.events
	if event.Kind != DiagnosticConnectionFailure || event.RemoteNode != "node-b" || !errors.Is(event.Err, net.ErrClosed) {
		t.Fatalf("terminal=%+v", event)
	}
	reporter.shared = nil
	reporter.terminal("node-b", net.ErrClosed, true)
	if len(standalone) != 1 {
		t.Fatal("standalone terminal was filtered")
	}
	reporter.terminal("node-b", nil, false)
	if len(standalone) != 1 {
		t.Fatal("nil terminal reported")
	}
}
