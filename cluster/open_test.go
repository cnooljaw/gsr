package cluster

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"
)

// 验证公开监听可被匿名出站节点调用，重复打开被拒绝，关闭不接管 Runtime。
func TestClusterOpenCallAndClose(t *testing.T) {
	adapter, id, _ := newLocalAdapterTest(t)
	server, err := NewCluster(ClusterConfig{Codec: adapter.codec})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, server)
	address, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0", ResolveService: adapter.resolve})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Open(adapter.runtime, ListenConfig{Address: address}); !errors.Is(err, ErrClusterAlreadyOpen) {
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
	value, err := client.Call(context.Background(), "b", ServiceTarget{ServiceID: id}, clusterTestCommand, "hello")
	if err != nil || !reflect.DeepEqual(value, []any{"hello"}) {
		t.Fatalf("value=%v err=%v", value, err)
	}
	server.Quiesce()
	if socket, err := net.Dial("tcp", address); err == nil {
		socket.Close()
		t.Fatal("listener still open")
	}
	closeTestCluster(t, server)
	if _, err := adapter.runtime.Call(context.Background(), id, clusterTestCommand, "local"); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Open(adapter.runtime, ListenConfig{Address: address}); !errors.Is(err, ErrClusterQuiescing) {
		t.Fatal(err)
	}
}

// 验证停止入站接纳后已进入 Handler 的 Call 仍能沿原连接回复。
func TestClusterOpenQuiesceKeepsReply(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	blocked := &inboundRecordingService{entered: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(blocked.release) }) }
	defer release()
	id, err := adapter.runtime.CreateService(context.Background(), blocked)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewCluster(ClusterConfig{Codec: adapter.codec})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, server)
	address, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0"})
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Call(ctx, "b", ServiceTarget{ServiceID: id}, clusterTestCommand, "block")
		result <- err
	}()
	select {
	case <-blocked.entered:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	server.Quiesce()
	release()
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if len(server.diagnostics.events) != 0 {
		t.Fatal("normal quiesce reported failure")
	}
}

// 验证 Open 与 Close 竞争后不会残留监听，也不会提前关闭诊断导致后台 panic。
func TestClusterOpenCloseRace(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	for i := 0; i < 20; i++ {
		server, err := NewCluster(ClusterConfig{Codec: adapter.codec})
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan struct{})
		var address string
		var openErr error
		go func() {
			defer close(done)
			address, openErr = server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0"})
		}()
		closeTestCluster(t, server)
		<-done
		if openErr != nil && !errors.Is(openErr, ErrClusterQuiescing) {
			t.Fatal(openErr)
		}
		if address != "" {
			socket, err := net.DialTimeout("tcp", address, time.Second)
			if err == nil {
				socket.Close()
				t.Fatal("listener leaked")
			}
		}
		if _, ok := <-server.Diagnostics(); ok {
			t.Fatal("unexpected close diagnostic")
		}
	}
}

// 验证坏载荷在 B 侧共享诊断中保留可信请求字段，不构造来源 NodeID。
func TestClusterOpenInboundDiagnostic(t *testing.T) {
	adapter, id, _ := newLocalAdapterTest(t)
	server, err := NewCluster(ClusterConfig{Codec: adapter.codec})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, server)
	address, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	socket, err := net.Dial("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	defer socket.Close()
	stream, err := newStreamConnection(socket, server.frames, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	request := ClusterEnvelope{Kind: FrameRequest, Session: 77, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand + 1}
	if err := stream.write(request); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-server.Diagnostics():
		if event.Kind != DiagnosticInboundFailure || event.RemoteNode != "" || event.RemoteAddress != socket.LocalAddr().String() || event.Session != 77 || event.Target != request.Target || event.Command != request.Command || !errors.Is(event.Err, ErrMalformedPayload) {
			t.Fatalf("event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing diagnostic")
	}
	if server.inbound.listener.diagnostics.failures != nil {
		t.Fatal("extra inbound queue")
	}
}

// 验证监听失败不占用打开名额、不重复诊断，并发打开最多成功一次。
func TestClusterOpenRetryAndConcurrency(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	server, err := NewCluster(ClusterConfig{Codec: adapter.codec})
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, server)
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer busy.Close()
	if _, err := server.Open(adapter.runtime, ListenConfig{Address: busy.Addr().String()}); err == nil {
		t.Fatal("occupied port accepted")
	}
	if len(server.diagnostics.events) != 0 {
		t.Fatal("Open failure duplicated")
	}
	var workers sync.WaitGroup
	results := make(chan error, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0"})
			results <- err
		}()
	}
	workers.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, ErrClusterAlreadyOpen) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes=%d", successes)
	}
}

// 验证成功打开后监听异常退出会报告，且不会关闭仍供出站使用的共享出口。
func TestClusterOpenListenerFailure(t *testing.T) {
	adapter, _, _ := newLocalAdapterTest(t)
	server, err := NewCluster(ClusterConfig{Codec: adapter.codec})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0"}); err != nil {
		t.Fatal(err)
	}
	if err := server.inbound.listener.config.listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case event, ok := <-server.Diagnostics():
		if !ok || event.Kind != DiagnosticInboundFailure || event.RemoteAddress != "" || !errors.Is(event.Err, net.ErrClosed) {
			t.Fatalf("event=%+v ok=%v", event, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("missing listener failure")
	}
	select {
	case _, ok := <-server.Diagnostics():
		if !ok {
			t.Fatal("shared stream closed early")
		}
	default:
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Close(ctx); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("close=%v", err)
	}
}
