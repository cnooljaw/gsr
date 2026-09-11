package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证业务、Runtime 与 Cluster 直接共用一个日志序列，不依赖诊断消费者转发。
func TestUnifiedLoggingOrder(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logger.Close(context.Background()) })
	local, err := runtime.NewRuntime(runtime.Config{Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { local.Close() })
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	id, err := local.CreateService(context.Background(), &tcpErrorService{err: errors.New("domain failure")})
	if err != nil {
		t.Fatal(err)
	}
	config := testClusterConfig(t)
	config.Logger = logger
	network, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestCluster(t, network) })
	business := logger.WithService(uint64(id))
	if err := business.Info("before"); err != nil {
		t.Fatal(err)
	}
	if _, err := local.Call(context.Background(), id, clusterTestCommand); err == nil {
		t.Fatal("missing Handler failure")
	}
	// 仅作测试同步；不把事件写回 Logger。
	select {
	case <-local.Diagnostics():
	case <-time.After(time.Second):
		t.Fatal("missing diagnostic")
	}
	network.diagnostics.report(Diagnostic{Kind: DiagnosticConnectionFailure, RemoteNode: "b", Err: errors.New("disconnected")})
	if err := business.Info("after"); err != nil {
		t.Fatal(err)
	}
	closeTestCluster(t, network)
	if err := local.Close(); err != nil {
		t.Fatal(err)
	}
	// 生产者都关闭后 Logger 仍可输出应用退出记录。
	if err := logger.Info("shutdown"); err != nil {
		t.Fatal(err)
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	for i, expected := range []string{"before", "Handler failed", "Cluster failure", "after", "shutdown"} {
		var entry struct {
			Sequence int    `json:"seq"`
			Message  string `json:"message"`
			File     string `json:"file"`
		}
		if err := decoder.Decode(&entry); err != nil {
			t.Fatal(err)
		}
		if entry.Sequence != i+1 || entry.Message != expected || entry.File == "" {
			t.Fatalf("entry=%+v", entry)
		}
	}
}
