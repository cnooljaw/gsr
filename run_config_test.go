package gsr

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
)

// 验证非法节点配置在业务入口执行前被拒绝，并提供稳定的配置错误。
func TestRunRejectsInvalidNodesBeforeStartup(t *testing.T) {
	tests := []struct {
		name    string
		node    cluster.NodeID
		address string
	}{
		{"empty_node", "", "127.0.0.1:9000"},
		{"empty_host", "battle", ":9000"},
		{"missing_port", "battle", "127.0.0.1"},
		{"empty_port", "battle", "127.0.0.1:"},
		{"named_port", "battle", "localhost:http"},
		{"negative_port", "battle", "localhost:-1"},
		{"signed_port", "battle", "localhost:+80"},
		{"port_overflow", "battle", "localhost:65536"},
		{"port_integer_overflow", "battle", "localhost:99999999999999999999"},
		{"host_space", "battle", "bad host:9000"},
		{"host_path", "battle", "host/path:9000"},
		{"host_backslash", "battle", "host\\path:9000"},
		{"unbracketed_ipv6", "battle", "::1:9000"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			started := false
			entry := &probeService{start: func(_ context.Context, _ ServiceContext) error {
				started = true
				return errors.New("unexpected startup")
			}}
			err := Run(context.Background(), Config{
				Logging: logging.Config{Writer: io.Discard},
				Cluster: ClusterConfig{Nodes: map[cluster.NodeID]string{test.node: test.address}},
			}, entry)
			if !errors.Is(err, cluster.ErrInvalidClusterConfig) || started {
				t.Fatalf("error=%v, entry started=%v", err, started)
			}
		})
	}
}

// 验证共同节点表保留测试监听端口零，但出站路由只包含非零端口，且两个结果均独立于输入。
func TestSplitNodeAddressesCopiesAndFiltersRoutes(t *testing.T) {
	input := map[cluster.NodeID]string{
		"test":   "127.0.0.1:0",
		"battle": "battle.internal:9000",
		"ipv6":   "[::1]:65535",
		"min":    "127.0.0.1:1",
	}
	nodes, routes, err := splitNodeAddresses(input)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(nodes, input) {
		t.Fatalf("node table changed: %v", nodes)
	}
	wantRoutes := map[cluster.NodeID]string{
		"battle": "battle.internal:9000",
		"ipv6":   "[::1]:65535",
		"min":    "127.0.0.1:1",
	}
	if !reflect.DeepEqual(routes, wantRoutes) {
		t.Fatalf("routes=%v, want %v", routes, wantRoutes)
	}
	input["battle"] = "changed:9001"
	delete(input, "test")
	if nodes["battle"] != wantRoutes["battle"] || nodes["test"] != "127.0.0.1:0" || !reflect.DeepEqual(routes, wantRoutes) {
		t.Fatal("input mutation changed runtime configuration")
	}
	nodes["battle"] = "other:9002"
	if !reflect.DeepEqual(routes, wantRoutes) {
		t.Fatal("listen table and outbound routes share a map")
	}
}
