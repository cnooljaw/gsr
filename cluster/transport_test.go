package cluster

import (
	"errors"
	"testing"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// 验证 InMemoryTransport 传递来源并在接收边界复制可变 Envelope 值。
func TestInMemoryTransportDeliversClonedEnvelope(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{MaxPayload: 32})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}

	var gotFrom NodeID
	var gotPayload string
	if err := transport.Register("node-b", func(from NodeID, envelope ClusterEnvelope) {
		gotFrom = from
		gotPayload = string(envelope.Payload)
		envelope.Payload[0] = 'X'
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	envelope := ClusterEnvelope{
		Kind:    FrameRequest,
		Session: ClusterSessionID(3),
		Target:  ServiceTarget{ServiceName: "room"},
		Command: clusterTestCommand,
		Payload: []byte("hello"),
	}
	if err := transport.Send("node-a", "node-b", envelope); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	if gotFrom != "node-a" {
		t.Fatalf("receiver from = %q, want node-a", gotFrom)
	}
	if gotPayload != "hello" {
		t.Fatalf("receiver payload = %q, want hello", gotPayload)
	}
	if string(envelope.Payload) != "hello" {
		t.Fatalf("sender payload = %q, want unchanged", envelope.Payload)
	}
}

// 验证 InMemoryTransport 对节点、Envelope 和 payload 上限返回可判定错误。
func TestInMemoryTransportRejectsInvalidOrOversizedSend(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{MaxPayload: 4})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	if err := transport.Register("node-b", func(NodeID, ClusterEnvelope) {}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}

	envelope := ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  ServiceTarget{ServiceID: 1},
		Command: clusterTestCommand,
		Payload: []byte("large"),
	}
	if err := transport.Send("node-a", "node-b", envelope); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("oversized Send() error = %v, want ErrPayloadTooLarge", err)
	}
	if err := transport.Send("node-a", "missing", ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  ServiceTarget{ServiceID: 1},
		Command: clusterTestCommand,
	}); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("unknown destination Send() error = %v, want ErrNodeNotFound", err)
	}
	if err := transport.Send("", "node-b", envelope); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("empty source Send() error = %v, want ErrInvalidNode", err)
	}
	if err := transport.Register("", func(NodeID, ClusterEnvelope) {}); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("empty Register() error = %v, want ErrInvalidNode", err)
	}
}

// 验证 Close 是最终状态，重复关闭安全且关闭后不再接受传输操作。
func TestInMemoryTransportCloseIsFinal(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	if err := transport.Register("node-a", func(NodeID, ClusterEnvelope) {}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("first Close() error = %v, want nil", err)
	}
	if err := transport.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}

	envelope := ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  ServiceTarget{ServiceID: runtime.ServiceID(1)},
		Command: clusterTestCommand,
	}
	if err := transport.Send("node-a", "node-a", envelope); !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("closed Send() error = %v, want ErrTransportClosed", err)
	}
	if err := transport.Register("node-b", func(NodeID, ClusterEnvelope) {}); !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("closed Register() error = %v, want ErrTransportClosed", err)
	}
	if err := transport.Unregister("node-a"); !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("closed Unregister() error = %v, want ErrTransportClosed", err)
	}
}

// 验证非法配置在构造阶段失败，不创建不可用的 Transport。
func TestNewInMemoryTransportRejectsInvalidConfig(t *testing.T) {
	if _, err := NewInMemoryTransport(InMemoryTransportConfig{MaxPayload: -1}); !errors.Is(err, ErrInvalidTransportConfig) {
		t.Fatalf("negative MaxPayload error = %v, want ErrInvalidTransportConfig", err)
	}
}
