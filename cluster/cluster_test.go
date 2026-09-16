package cluster

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

func testClusterConfig(t *testing.T) ClusterConfig {
	t.Helper()
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	return ClusterConfig{Codec: codec}
}

func closeTestCluster(t *testing.T, cluster *Cluster) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := cluster.Close(ctx); err != nil {
		t.Error(err)
	}
}

// 验证构造与来源绑定不创建发送任务，默认值有限，路由表采用独占副本。
func TestClusterConfigAndBinding(t *testing.T) {
	config := testClusterConfig(t)
	config.Nodes = map[NodeID]string{"node-b": "localhost:1234"}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, cluster)
	config.Nodes["node-b"] = "localhost:4321"
	if cluster.config.Nodes["node-b"] != "localhost:1234" || len(cluster.targets) != 0 {
		t.Fatal("construction shared routes or started a sender")
	}
	if cluster.config.MaxCallWait != 5*time.Second || cluster.config.DialTimeout != 3*time.Second ||
		cluster.config.WriteTimeout != 3*time.Second || cluster.config.OutboundQueueCapacity != 1024 ||
		cluster.config.MaxFrame != 4<<20 {
		t.Fatalf("defaults = %+v", cluster.config)
	}
	for _, id := range []runtime.ServiceID{11, 12, 11} {
		client, err := cluster.Client(id)
		if err != nil || client.Source() != id {
			t.Fatalf("Client(%d) = %v", id, err)
		}
	}
	if _, err := cluster.Client(0); !errors.Is(err, ErrInvalidClusterClientConfig) {
		t.Fatal(err)
	}
	if len(cluster.targets) != 0 {
		t.Fatal("binding created a sender")
	}
}

// 验证非法静态路由和负数上限在拨号前被拒绝，不接受无限配置。
func TestClusterRejectsInvalidConfig(t *testing.T) {
	for _, address := range []string{"", ":1234", "localhost:0", "localhost:65536", "localhost:http", "tcp://localhost:1234"} {
		config := testClusterConfig(t)
		config.Nodes = map[NodeID]string{"node-b": address}
		if _, err := NewCluster(config); !errors.Is(err, ErrInvalidClusterConfig) {
			t.Fatalf("address %q: %v", address, err)
		}
	}
	for _, change := range []func(*ClusterConfig){
		func(config *ClusterConfig) { config.Codec = (*ValueCodec)(nil) },
		func(config *ClusterConfig) { config.Nodes = map[NodeID]string{"": "localhost:1"} },
		func(config *ClusterConfig) { config.MaxCallWait = -1 },
		func(config *ClusterConfig) { config.DialTimeout = -1 },
		func(config *ClusterConfig) { config.WriteTimeout = -1 },
		func(config *ClusterConfig) { config.OutboundQueueCapacity = -1 },
		func(config *ClusterConfig) { config.MaxFrame = -1 },
	} {
		config := testClusterConfig(t)
		change(&config)
		if _, err := NewCluster(config); !errors.Is(err, ErrInvalidClusterConfig) {
			t.Fatal(err)
		}
	}
}

// 验证未知目标在 admission 前失败，不创建发送器或重复诊断；Quiesce 后旧 client 也拒绝请求。
func TestClusterUnknownRouteAndQuiesce(t *testing.T) {
	cluster, err := NewCluster(testClusterConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, cluster)
	client, err := cluster.Client(11)
	if err != nil {
		t.Fatal(err)
	}
	target := ServiceTarget{ServiceID: 1}
	if err := client.Send("absent", target, clusterTestCommand); !errors.Is(err, ErrNodeNotFound) {
		t.Fatal(err)
	}
	if _, err := client.Call(context.Background(), "absent", target, clusterTestCommand); !errors.Is(err, ErrNodeNotFound) {
		t.Fatal(err)
	}
	if len(cluster.targets) != 0 || cluster.calls.PendingCount() != 0 || len(cluster.Diagnostics()) != 0 {
		t.Fatal("unknown route created resources or diagnostics")
	}
	cluster.Quiesce()
	cluster.Quiesce()
	if _, err := cluster.Client(11); !errors.Is(err, ErrClusterQuiescing) {
		t.Fatal(err)
	}
	if err := client.Send("absent", target, clusterTestCommand); !errors.Is(err, ErrClusterQuiescing) {
		t.Fatal(err)
	}
}

// 验证公开组装可连接真实目标 Runtime，来源 client 共享唯一发送器且主动关闭不报告连接故障。
func TestClusterPublicTCPRoundTrip(t *testing.T) {
	link := newTCPClusterTestLink(t)
	config := testClusterConfig(t)
	config.Nodes = map[NodeID]string{"node-b": link.listener.config.listener.Addr().String()}
	cluster, err := NewCluster(config)
	if err != nil {
		t.Fatal(err)
	}
	defer closeTestCluster(t, cluster)
	for _, source := range []runtime.ServiceID{11, 12} {
		client, err := cluster.Client(source)
		if err != nil {
			t.Fatal(err)
		}
		value, err := client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: link.target}, clusterTestCommand, "hello")
		if err != nil || !reflect.DeepEqual(value, []any{"hello"}) {
			t.Fatalf("Call = %#v, %v", value, err)
		}
	}
	if len(cluster.targets) != 1 || cluster.calls.PendingCount() != 0 {
		t.Fatal("sender or calls were not shared")
	}
	closeTestCluster(t, cluster)
	for failure := range cluster.Diagnostics() {
		t.Fatalf("unexpected diagnostic: %+v", failure)
	}
}

// 验证未知响应与真实断线通过统一出口报告，下一 Call 可恢复且不残留等待。
func TestClusterPublicTCPDiagnosticsAndRecovery(t *testing.T) {
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
	if err := connection.write(ClusterEnvelope{Kind: FrameResponse, Session: 999,
		Target: target, Command: clusterTestCommand}); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-cluster.Diagnostics():
		if event.Kind != DiagnosticResponseRejected || event.Session != 999 ||
			event.Target != target || event.Command != clusterTestCommand || !errors.Is(event.Err, ErrClusterSessionNotFound) {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing response diagnostic")
	}
	if err := connection.close(); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-cluster.Diagnostics():
		if event.Kind != DiagnosticConnectionFailure || event.RemoteNode != "node-b" || !errors.Is(event.Err, io.EOF) {
			t.Fatalf("event = %+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("missing connection diagnostic")
	}
	if _, err := client.Call(context.Background(), "node-b", target, clusterTestCommand); err != nil {
		t.Fatal(err)
	}
	if cluster.calls.PendingCount() != 0 {
		t.Fatal("Call leaked after reconnect")
	}
}
