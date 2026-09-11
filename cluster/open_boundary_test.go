package cluster

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证公开 Send 的业务失败只归 B 的 Runtime，Call 返回领域错误但不重复产生网络诊断。
func TestClusterOpenHandlerErrorOwnership(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	domainErr := runtime.NewStatusError(runtime.ErrorClassDomain, "room.full", "room is full")
	id, err := adapter.runtime.CreateService(context.Background(), &tcpErrorService{err: domainErr})
	if err != nil {
		t.Fatal(err)
	}
	server, client, caller := newOpenBoundaryLink(t, adapter, nil)
	target := ServiceTarget{ServiceID: id}
	if err := client.Send("b", target, clusterTestCommand); err != nil {
		t.Fatal(err)
	}
	for _, call := range []bool{false, true} {
		if call {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, err := client.Call(ctx, "b", target, clusterTestCommand)
			cancel()
			if !errors.Is(err, domainErr) {
				t.Fatalf("Call error=%v", err)
			}
		}
		select {
		case event := <-adapter.runtime.Diagnostics():
			if event.ServiceID != id || event.Source != 0 || event.Class != runtime.ErrorClassDomain || event.Code != "room.full" || (event.Session != 0) != call {
				t.Fatalf("event=%+v call=%v", event, call)
			}
		case <-time.After(time.Second):
			t.Fatal("missing Runtime diagnostic")
		}
	}
	// 等所有生产者退出后检查完整事件流，不用短暂空队列推断未来没有重复诊断。
	closeTestCluster(t, caller)
	closeTestCluster(t, server)
	for event := range server.Diagnostics() {
		t.Errorf("unexpected server diagnostic: %+v", event)
	}
	for event := range caller.Diagnostics() {
		t.Errorf("unexpected caller diagnostic: %+v", event)
	}
}

// 验证外层解析暂未返回时 Close 超时只结束本次等待，诊断保持打开，释放后可继续完成关闭。
func TestClusterOpenCloseTimeoutKeepsDiagnostics(t *testing.T) {
	adapter, id, _ := newLocalAdapterTest(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	server, client, caller := newOpenBoundaryLink(t, adapter, func(ServiceName) (runtime.ServiceID, error) {
		close(entered)
		<-release
		return id, nil
	})
	// 后注册的 cleanup 先执行，保证断言失败也会解除扩展代码等待。
	t.Cleanup(unblock)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Call(ctx, "b", ServiceTarget{ServiceName: "room"}, clusterTestCommand)
		result <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	wait, stopWait := context.WithTimeout(context.Background(), 20*time.Millisecond)
	err := server.Close(wait)
	stopWait()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close=%v", err)
	}
	select {
	case _, ok := <-server.Diagnostics():
		if !ok {
			t.Fatal("diagnostics closed before producer exited")
		}
	default:
	}
	unblock()
	closeTestCluster(t, server)
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Call succeeded after connection closed")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	closeTestCluster(t, caller)
	for range server.Diagnostics() {
		// 允许读取最终收尾事件；通道必须最终关闭，不能滞留生产者。
	}
}

func newOpenBoundaryLink(t *testing.T, adapter *localRequestAdapter, resolve func(ServiceName) (runtime.ServiceID, error)) (*Cluster, *ClusterClient, *Cluster) {
	t.Helper()
	server, err := NewCluster(ClusterConfig{Codecs: adapter.codecs})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestCluster(t, server) })
	address, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0", ResolveService: resolve})
	if err != nil {
		t.Fatal(err)
	}
	caller, err := NewCluster(ClusterConfig{Codecs: adapter.codecs, Nodes: map[NodeID]string{"b": address}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestCluster(t, caller) })
	client, err := caller.Client(1)
	if err != nil {
		t.Fatal(err)
	}
	return server, client, caller
}
