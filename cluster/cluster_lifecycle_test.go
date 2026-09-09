package cluster

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// 验证已绑定 Call 在 Quiesce 后仍可收到结果，未绑定等待立即失败且不能再绑定。
func TestClusterQuiescePreservesBoundCall(t *testing.T) {
	link := newTCPClusterTestLink(t)
	config := testClusterConfig(t)
	config.Nodes = map[NodeID]string{"node-b": link.listener.config.listener.Addr().String()}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, cluster)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(blocked.release) }) }
	defer release()
	id, err := link.adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	client, err := cluster.Client(11)
	if err != nil {
		t.Fatal(err)
	}
	outcome := make(chan error, 1)
	go func() {
		_, err := client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: id}, clusterTestCommand, "block")
		outcome <- err
	}()
	select {
	case <-blocked.entered:
	case <-time.After(time.Second):
		t.Fatal("Handler did not start")
	}
	unbound := beginResponseReaderCall(t, cluster.calls, "node-b")
	cluster.Quiesce()
	if result := awaitPeerCall(t, unbound); !errors.Is(result.err, ErrClusterQuiescing) {
		t.Fatal(result.err)
	}
	select {
	case err := <-outcome:
		t.Fatalf("bound Call ended before Handler: %v", err)
	default:
	}
	release()
	select {
	case err := <-outcome:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("bound response was lost")
	}
	if cluster.calls.PendingCount() != 0 {
		t.Fatal("waiting calls leaked")
	}
}

// 验证绑定与 Quiesce 竞争有唯一先后顺序，不会在停止后留下新绑定。
func TestClusterQuiesceBindingRace(t *testing.T) {
	for iteration := 0; iteration < 64; iteration++ {
		cluster, err := NewCluster(testClusterConfig(t))
		if err != nil {
			t.Fatal(err)
		}
		call := beginResponseReaderCall(t, cluster.calls, "node-b")
		owner := newConnectionCallsTest(t, cluster.calls, "node-b")
		start := make(chan struct{})
		bound := make(chan bool, 1)
		go func() {
			<-start
			ready, _ := owner.bind(connectionCallRequest(call))
			bound <- ready
		}()
		close(start)
		cluster.Quiesce()
		if <-bound {
			response := connectionCallRequest(call)
			response.Kind = FrameResponse
			if err := owner.complete(response); err != nil {
				t.Fatal(err)
			}
		} else if result := awaitPeerCall(t, call); !errors.Is(result.err, ErrClusterQuiescing) {
			t.Fatal(result.err)
		}
		if ready, err := owner.bind(connectionCallRequest(call)); ready || !errors.Is(err, ErrClusterQuiescing) {
			t.Fatalf("binding after Quiesce = %v, %v", ready, err)
		}
		closeTestCluster(t, cluster)
	}
}

// 验证 Close 等待超时不停止清理，所有目标先取消，诊断出口直到生产者退出才关闭。
func TestClusterCloseTimeoutAndAllTargets(t *testing.T) {
	config := testClusterConfig(t)
	config.Nodes = map[NodeID]string{"node-a": "localhost:1234", "node-b": "localhost:1235"}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer closeTestCluster(t, cluster)
	defer unblock()
	entered := make(chan struct{}, 2)
	cancelled := make(chan struct{}, 2)
	var attempts atomic.Int32
	cluster.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		attempts.Add(1)
		entered <- struct{}{}
		<-ctx.Done()
		cancelled <- struct{}{}
		// 故意延迟协作退出，验证调用方不必无限等测试扩展。
		<-release
		return nil, ctx.Err()
	}
	client, err := cluster.Client(11)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range []NodeID{"node-a", "node-b"} {
		if err := client.Send(node, ServiceTarget{ServiceID: 1}, clusterTestCommand); err != nil {
			t.Fatal(err)
		}
		select {
		case <-entered:
		case <-time.After(time.Second):
			t.Fatal("Dial did not start")
		}
		if err := client.Send(node, ServiceTarget{ServiceID: 1}, clusterTestCommand); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := cluster.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close = %v", err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("close waited for one target before cancelling the other")
		}
	}
	select {
	case _, ok := <-cluster.Diagnostics():
		if !ok {
			t.Fatal("diagnostics closed while Dial producers remain")
		}
	default:
	}
	if attempts.Load() != 2 {
		t.Fatal("queued Send triggered extra Dial")
	}
	unblock()
	closeTestCluster(t, cluster)
	count := 0
	for event := range cluster.Diagnostics() {
		if event.Kind != DiagnosticDeliveryFailure {
			t.Fatalf("event = %+v", event)
		}
		count++
	}
	if count != 4 {
		t.Fatalf("delivery failures = %d, want active and queued Send for both targets", count)
	}
}

// 验证响应诊断满载时读循环继续处理正常回复，不依赖日志消费者腾出空间。
func TestClusterResponseDiagnosticOverflow(t *testing.T) {
	link := newTCPClusterTestLink(t)
	config := testClusterConfig(t)
	config.Nodes = map[NodeID]string{"node-b": link.listener.config.listener.Addr().String()}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, cluster)
	client, err := cluster.Client(11)
	if err != nil {
		t.Fatal(err)
	}
	target := ServiceTarget{ServiceID: link.target}
	if _, err := client.Call(context.Background(), "node-b", target, clusterTestCommand); err != nil {
		t.Fatal(err)
	}
	connection := link.connection(t)
	for i := 0; i < 300; i++ {
		if err := connection.write(ClusterEnvelope{Kind: FrameResponse, Session: 999,
			Target: target, Command: clusterTestCommand}); err != nil {
			t.Fatal(err)
		}
	}
	// 同一 TCP 响应流上的后续正常回复确认此前 300 条均已经过读循环。
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Call(ctx, "node-b", target, clusterTestCommand); err != nil {
		t.Fatal(err)
	}
	if cluster.DroppedDiagnosticCount() != 44 || len(cluster.Diagnostics()) != 256 {
		t.Fatalf("dropped=%d buffered=%d", cluster.DroppedDiagnosticCount(), len(cluster.Diagnostics()))
	}
}

// 验证日志无人消费和队列溢出不阻塞投递，失败只进入共享出口并正确计数。
func TestClusterDiagnosticOverflow(t *testing.T) {
	config := testClusterConfig(t)
	config.Nodes = map[NodeID]string{"node-b": "localhost:1234"}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, cluster)
	cluster.dial = func(context.Context, string, string) (net.Conn, error) { return nil, io.ErrUnexpectedEOF }
	client, err := cluster.Client(11)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 300; i++ {
		_, err := client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: 1}, clusterTestCommand)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatal(err)
		}
	}
	closeTestCluster(t, cluster)
	if cluster.DroppedDiagnosticCount() != 44 {
		t.Fatalf("dropped = %d", cluster.DroppedDiagnosticCount())
	}
	count := 0
	for event := range cluster.Diagnostics() {
		count++
		if event.Kind != DiagnosticDeliveryFailure || event.RemoteNode != "node-b" ||
			event.Session == 0 || event.Command != clusterTestCommand || !errors.Is(event.Err, io.ErrUnexpectedEOF) {
			t.Fatalf("event = %+v", event)
		}
	}
	if count != 256 {
		t.Fatalf("diagnostics = %d", count)
	}
	target := cluster.targets["node-b"]
	if target.sender.failures != nil || target.peer.diagnostics != nil {
		t.Fatal("assembly allocated duplicate per-target diagnostic queues")
	}
}

// 验证并发来源解析只创建一个目标发送器，Close 可被重复并发等待。
func TestClusterConcurrentClientsAndClose(t *testing.T) {
	config := testClusterConfig(t)
	config.Nodes = map[NodeID]string{"node-b": "localhost:1234"}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, cluster)
	var workers sync.WaitGroup
	results := make(chan OutboundSender, 32)
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			sender, err := (clusterResolver{cluster}).ResolveNodeSender("node-b")
			if err != nil {
				t.Error(err)
			}
			results <- sender
		}()
	}
	workers.Wait()
	close(results)
	var first OutboundSender
	for sender := range results {
		if first == nil {
			first = sender
		} else if sender != first {
			t.Fatal("multiple senders for one target")
		}
	}
	if err := cluster.Close(nil); !errors.Is(err, ErrInvalidClusterCloseContext) {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			if err := cluster.Close(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	select {
	case _, ok := <-cluster.Diagnostics():
		if ok {
			t.Fatal("idle close produced an event")
		}
	default:
		t.Fatal("diagnostics did not close")
	}
}
