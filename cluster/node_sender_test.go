package cluster

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 验证 NodeSender 按单一 owner 顺序投递已 admission 的 Envelope。
func TestNodeSenderDeliversAdmittedEnvelopesInOrder(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	received := make(chan string, 2)
	if err := transport.Register("node-b", func(_ NodeID, envelope ClusterEnvelope) {
		received <- string(envelope.Payload)
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:     "node-a",
		RemoteNode:    "node-b",
		Transport:     transport,
		QueueCapacity: 2,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}

	if err := sender.Send(senderTestEnvelope("first")); err != nil {
		t.Fatalf("first Send() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("second")); err != nil {
		t.Fatalf("second Send() error = %v, want nil", err)
	}
	for _, want := range []string{"first", "second"} {
		select {
		case got := <-received:
			if got != want {
				t.Fatalf("received payload = %q, want %q", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for payload %q", want)
		}
	}
	select {
	case failure := <-sender.Failures():
		t.Fatalf("unexpected delivery failure: %+v", failure)
	default:
	}
	closeSender(t, sender)
}

// 验证 NodeSender 的出站队列有界且满载时立即返回稳定错误。
func TestNodeSenderRejectsFullQueueWithoutBlocking(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	transport := &senderTestTransport{
		send: func(NodeID, NodeID, ClusterEnvelope) error {
			close(started)
			<-release
			return nil
		},
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:     "node-a",
		RemoteNode:    "node-b",
		Transport:     transport,
		QueueCapacity: 1,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("in-flight")); err != nil {
		t.Fatalf("in-flight Send() error = %v, want nil", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for in-flight transport call")
	}
	if err := sender.Send(senderTestEnvelope("queued")); err != nil {
		t.Fatalf("queued Send() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("rejected")); !errors.Is(err, ErrOutboundQueueFull) {
		t.Fatalf("full Send() error = %v, want ErrOutboundQueueFull", err)
	}
	close(release)
	closeSender(t, sender)
}

// 验证 Send admission 会复制 Envelope，发送方后续修改不会影响 worker 的输入快照。
func TestNodeSenderClonesEnvelopeOnAdmission(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	received := make(chan ClusterEnvelope, 1)
	transport := &senderTestTransport{
		send: func(_ NodeID, _ NodeID, envelope ClusterEnvelope) error {
			close(started)
			<-release
			received <- envelope
			return nil
		},
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:  "node-a",
		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	envelope := senderTestEnvelope("before")
	if err := sender.Send(envelope); err != nil {
		t.Fatalf("Send() error = %v, want nil", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for transport call")
	}
	envelope.Payload[0] = 'a'
	close(release)
	select {
	case got := <-received:
		if string(got.Payload) != "before" {
			t.Fatalf("transport payload = %q, want before", got.Payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cloned envelope")
	}
	closeSender(t, sender)
}

// 验证 admission 后的传输失败以不含业务 payload 的基础设施事件对外可见。
func TestNodeSenderReportsPostAdmissionTransportFailure(t *testing.T) {
	wantErr := errors.New("dial failed")
	transport := &senderTestTransport{
		send: func(NodeID, NodeID, ClusterEnvelope) error {
			return wantErr
		},
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:  "node-a",
		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("failed")); err != nil {
		t.Fatalf("Send() error = %v, want nil admission", err)
	}
	select {
	case failure := <-sender.Failures():
		if !errors.Is(failure.Err, wantErr) {
			t.Fatalf("failure.Err = %v, want %v", failure.Err, wantErr)
		}
		if failure.LocalNode != "node-a" || failure.RemoteNode != "node-b" {
			t.Fatalf("failure nodes = %q -> %q, want node-a -> node-b", failure.LocalNode, failure.RemoteNode)
		}
		if failure.Kind != FrameRequest || failure.Target.ServiceID != 7 || failure.Command != frameTestCommand {
			t.Fatalf("failure routing metadata = %+v, want request to service 7 command %d", failure, frameTestCommand)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for delivery failure")
	}
	closeSender(t, sender)
}

// 验证远端节点不可用只产生一条底层失败记录，且 sender 随后仍可继续投递。
func TestNodeSenderReportsUnavailableRemoteAndContinues(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:  "node-a",
		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("while-down")); err != nil {
		t.Fatalf("Send(while-down) error = %v, want nil admission", err)
	}
	select {
	case failure := <-sender.Failures():
		if !errors.Is(failure.Err, ErrNodeNotFound) {
			t.Fatalf("failure.Err = %v, want ErrNodeNotFound", failure.Err)
		}
		if failure.RemoteNode != "node-b" {
			t.Fatalf("failure.RemoteNode = %q, want node-b", failure.RemoteNode)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for unavailable-node failure")
	}

	received := make(chan string, 1)
	if err := transport.Register("node-b", func(_ NodeID, envelope ClusterEnvelope) {
		received <- string(envelope.Payload)
	}); err != nil {
		t.Fatalf("Register(node-b) error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("after-up")); err != nil {
		t.Fatalf("Send(after-up) error = %v, want nil", err)
	}
	select {
	case got := <-received:
		if got != "after-up" {
			t.Fatalf("received payload = %q, want after-up", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-failure delivery")
	}
	closeSender(t, sender)
}

// 验证 Transport panic 被 sender owner 转成可观察的投递失败，而不会杀死进程。
func TestNodeSenderReportsTransportPanic(t *testing.T) {
	transport := &senderTestTransport{
		send: func(NodeID, NodeID, ClusterEnvelope) error {
			panic("transport panic")
		},
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:  "node-a",
		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("panic")); err != nil {
		t.Fatalf("Send() error = %v, want nil admission", err)
	}
	select {
	case failure := <-sender.Failures():
		if !errors.Is(failure.Err, ErrTransportPanicked) {
			t.Fatalf("failure.Err = %v, want ErrTransportPanicked", failure.Err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for panic failure")
	}
	closeSender(t, sender)
}

// 验证失败事件消费者停滞时 NodeSender 丢弃多余诊断并继续投递后续消息。
func TestNodeSenderDoesNotBlockOnFullFailureQueue(t *testing.T) {
	wantErr := errors.New("dial failed")
	total := defaultFailureQueueCapacity + 2
	attempted := make(chan int, total)
	attempt := 0
	transport := &senderTestTransport{
		send: func(_ NodeID, _ NodeID, _ ClusterEnvelope) error {
			attempt++
			attempted <- attempt
			if attempt == total {
				return nil
			}
			return wantErr
		},
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:     "node-a",
		RemoteNode:    "node-b",
		Transport:     transport,
		QueueCapacity: total,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	for index := 0; index < total; index++ {
		if err := sender.Send(senderTestEnvelope("message")); err != nil {
			t.Fatalf("Send(%d) error = %v, want nil admission", index, err)
		}
	}
	for want := 1; want <= total; want++ {
		select {
		case got := <-attempted:
			if got != want {
				t.Fatalf("transport attempt = %d, want %d", got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for transport attempt %d", want)
		}
	}
	if got := sender.DroppedFailureCount(); got != 1 {
		t.Fatalf("DroppedFailureCount() = %d, want 1", got)
	}
	closeSender(t, sender)
}

// 验证关闭时仍在队列中的 Envelope 不会静默丢失，而会以关闭错误报告。
func TestNodeSenderReportsQueuedMessagesOnClose(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	transport := &senderTestTransport{
		send: func(NodeID, NodeID, ClusterEnvelope) error {
			close(started)
			<-release
			return nil
		},
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:     "node-a",
		RemoteNode:    "node-b",
		Transport:     transport,
		QueueCapacity: 1,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("in-flight")); err != nil {
		t.Fatalf("in-flight Send() error = %v, want nil", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for in-flight transport call")
	}
	if err := sender.Send(senderTestEnvelope("queued")); err != nil {
		t.Fatalf("queued Send() error = %v, want nil", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sender.Close(canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Close() error = %v, want context.Canceled", err)
	}
	close(release)
	closeSender(t, sender)
	select {
	case failure := <-sender.Failures():
		if !errors.Is(failure.Err, ErrNodeSenderClosed) {
			t.Fatalf("failure.Err = %v, want ErrNodeSenderClosed", failure.Err)
		}
		if failure.Target.ServiceID != 7 || failure.Command != frameTestCommand {
			t.Fatalf("failure routing metadata = %+v, want service 7 command %d", failure, frameTestCommand)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for queued failure")
	}
}

// 验证 NodeSender 关闭后拒绝新 admission，重复关闭可安全等待同一 worker。
func TestNodeSenderCloseIsFinal(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:  "node-a",
		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	closeSender(t, sender)
	if err := sender.Close(context.Background()); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
	if err := sender.Send(senderTestEnvelope("after-close")); !errors.Is(err, ErrNodeSenderClosed) {
		t.Fatalf("closed Send() error = %v, want ErrNodeSenderClosed", err)
	}
	if _, open := <-sender.Failures(); open {
		t.Fatal("Failures() remains open after Close()")
	}
}

// 验证 NodeSender 构造阶段拒绝缺失依赖及非法的消息队列容量。
func TestNewNodeSenderRejectsInvalidConfig(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	valid := NodeSenderConfig{
		LocalNode:  "node-a",
		RemoteNode: "node-b",
		Transport:  transport,
	}
	cases := []NodeSenderConfig{
		{},
		{LocalNode: valid.LocalNode, RemoteNode: valid.RemoteNode},
		{LocalNode: valid.LocalNode, RemoteNode: valid.RemoteNode, Transport: valid.Transport, QueueCapacity: -1},
	}
	for index, config := range cases {
		if _, err := NewNodeSender(config); !errors.Is(err, ErrInvalidNodeSenderConfig) {
			t.Fatalf("case %d NewNodeSender() error = %v, want ErrInvalidNodeSenderConfig", index, err)
		}
	}
}

func senderTestEnvelope(payload string) ClusterEnvelope {
	return ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  ServiceTarget{ServiceID: 7},
		Command: frameTestCommand,
		Payload: []byte(payload),
	}
}

func closeSender(t *testing.T, sender *NodeSender) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := sender.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

type senderTestTransport struct {
	send func(NodeID, NodeID, ClusterEnvelope) error
}

func (transport *senderTestTransport) Register(NodeID, Receiver) error {
	return nil
}

func (transport *senderTestTransport) Unregister(NodeID) error {
	return nil
}

func (transport *senderTestTransport) Send(from NodeID, to NodeID, envelope ClusterEnvelope) error {
	return transport.send(from, to, envelope)
}

func (transport *senderTestTransport) Close() error {
	return nil
}
