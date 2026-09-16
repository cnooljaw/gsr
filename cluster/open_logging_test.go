package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
)

// 验证真实 TCP 每连接保留首条坏帧，日志与诊断同时去重，关闭后汇总且计数仍可查询。
func TestClusterOpenBadFrameLoggingPerConnection(t *testing.T) {
	var output bytes.Buffer
	logger, err := logging.New(logging.Config{Writer: &output})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { logger.Close(context.Background()) })
	adapter, id, codec := newLocalAdapterTest(t)
	server, err := NewCluster(ClusterConfig{Codec: adapter.codec, Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeTestCluster(t, server) })
	address, err := server.Open(adapter.runtime, ListenConfig{Address: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	addresses := make(map[string]bool)
	for i := 0; i < 2; i++ {
		socket, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { socket.Close() })
		addresses[socket.LocalAddr().String()] = true
		if err := socket.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
			t.Fatal(err)
		}
		for j := 0; j < 5; j++ {
			if err := writeFrameBytes(socket, []byte{0, 0, 0, 1, 255}); err != nil {
				t.Fatal(err)
			}
		}
		payload, err := codec.Encode([]any{"alive"})
		if err != nil {
			t.Fatal(err)
		}
		request := ClusterEnvelope{Kind: FrameRequest, Session: 1, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload}
		if err := server.frames.writeFrame(socket, request); err != nil {
			t.Fatal(err)
		}
		// 合法回复同时证明此前完整坏帧已被读循环处理，不通过 sleep 猜测进度。
		reply, err := server.frames.readFrame(socket)
		if err != nil || reply.Kind != FrameResponse || reply.Session != request.Session {
			t.Fatalf("reply=%+v err=%v", reply, err)
		}
	}
	if server.SuppressedDiagnosticCount() != 8 || server.DroppedDiagnosticCount() != 0 {
		t.Fatal("live counters incorrect")
	}
	closeTestCluster(t, server)
	firsts, summaries := make(map[string]int), make(map[string]int)
	for event := range server.Diagnostics() {
		if !addresses[event.RemoteAddress] || event.Session != 0 || event.Command != 0 || event.Target != (ServiceTarget{}) {
			t.Fatalf("unexpected attribution: %+v", event)
		}
		switch event.Kind {
		case DiagnosticInboundFailure:
			if !errors.Is(event.Err, ErrMalformedFrame) || event.Suppressed != 0 {
				t.Fatalf("detail=%+v", event)
			}
			firsts[event.RemoteAddress]++
		case DiagnosticBadFramesSuppressed:
			if event.Suppressed != 4 {
				t.Fatalf("summary=%+v", event)
			}
			summaries[event.RemoteAddress]++
		default:
			t.Fatalf("unexpected event=%+v", event)
		}
	}
	for address := range addresses {
		if firsts[address] != 1 || summaries[address] != 1 {
			t.Fatalf("address=%s firsts=%v summaries=%v", address, firsts, summaries)
		}
	}
	if server.SuppressedDiagnosticCount() != 8 {
		t.Fatal("close lost suppressed count")
	}
	if err := logger.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	// 关闭唯一写入者后才读取 Buffer，避免测试本身与日志输出发生数据竞争。
	decoder := json.NewDecoder(&output)
	loggedFirsts, loggedSummaries := 0, 0
	for sequence := 1; ; sequence++ {
		var entry struct {
			Sequence int `json:"seq"`
			Fields   struct {
				Kind       DiagnosticKind `json:"kind"`
				Suppressed uint64         `json:"suppressed"`
			} `json:"fields"`
		}
		if err := decoder.Decode(&entry); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if entry.Sequence != sequence {
			t.Fatal("log sequence discontinuity")
		}
		switch entry.Fields.Kind {
		case DiagnosticInboundFailure:
			loggedFirsts++
		case DiagnosticBadFramesSuppressed:
			if entry.Fields.Suppressed != 4 {
				t.Fatal("log summary count incorrect")
			}
			loggedSummaries++
		default:
			t.Fatal("unexpected log record")
		}
	}
	if loggedFirsts != 2 || loggedSummaries != 2 || logger.DroppedCount() != 0 {
		t.Fatalf("logged details=%d summaries=%d", loggedFirsts, loggedSummaries)
	}
}
