package cluster

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证外层更新名称映射后后续 TCP Call 使用新实例，不缓存故障实例 ID，也不将旧 ID 重定向到新实例。
func TestClusterNameRebindingAfterServiceFailure(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	failed := &tcpFailureProbe{panicOnHandle: true, stopped: make(chan struct{})}
	oldID, err := adapter.runtime.CreateService(ctx, failed)
	if err != nil {
		t.Fatal(err)
	}
	// 注册表由外层拥有；原子值只用于测试中跨 goroutine 发布映射，不让 Cluster 管理注册表。
	var current atomic.Uint64
	current.Store(uint64(oldID))
	server, err := NewCluster(ClusterConfig{Codec: adapter.codec})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, server)
	address, err := server.Open(adapter.runtime, ListenConfig{
		Address: "127.0.0.1:0",
		ResolveService: func(name ServiceName) (runtime.ServiceID, error) {
			if name != "room" {
				return 0, runtime.ErrServiceNotFound
			}
			return runtime.ServiceID(current.Load()), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := NewCluster(ClusterConfig{Codec: adapter.codec, Nodes: map[NodeID]string{"b": address}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, caller)
	client, err := caller.Client(1)
	if err != nil {
		t.Fatal(err)
	}
	named := ServiceTarget{ServiceName: "room"}
	if _, err := client.Call(ctx, "b", named, clusterTestCommand); !errors.Is(err, runtime.ErrServicePanicked) {
		t.Fatalf("original named Call=%v", err)
	}
	select {
	case <-failed.stopped:
	case <-ctx.Done():
		t.Fatal("failed Service did not stop")
	}
	if _, err := client.Call(ctx, "b", named, clusterTestCommand); !errors.Is(err, runtime.ErrServiceClosed) {
		t.Fatalf("unchanged mapping Call=%v", err)
	}
	replacement := &tcpFailureProbe{stopped: make(chan struct{})}
	newID, err := adapter.runtime.CreateService(ctx, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if newID <= oldID {
		t.Fatalf("new ID=%d old ID=%d", newID, oldID)
	}
	current.Store(uint64(newID))
	value, err := client.Call(ctx, "b", named, clusterTestCommand)
	if err != nil || value != "replacement" {
		t.Fatalf("updated mapping Call=%#v error=%v", value, err)
	}
	if _, err := client.Call(ctx, "b", ServiceTarget{ServiceID: oldID}, clusterTestCommand); !errors.Is(err, runtime.ErrServiceClosed) {
		t.Fatalf("old ID Call=%v", err)
	}
	if failed.calls.Load() != 1 || replacement.calls.Load() != 1 {
		t.Fatalf("unexpected calls: failed=%d replacement=%d", failed.calls.Load(), replacement.calls.Load())
	}
	if caller.calls.PendingCount() != 0 || adapter.runtime.Inspect().PendingCallCount != 0 {
		t.Fatal("name rebinding left pending Calls")
	}
}
