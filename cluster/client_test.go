package cluster

import (
	"context"
	"errors"
	"testing"
	"time"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// 验证绑定来源身份的 ClusterClient 能编码参数并按目标 NodeSender 投递 Request。
func TestClusterClientSendsEncodedRequest(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	received := make(chan ClusterEnvelope, 1)
	if err := transport.Register("node-b", func(envelope ClusterEnvelope) {
		received <- envelope
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{

		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	defer closeClientSender(t, sender)
	client, err := NewClusterClient(ClusterClientConfig{
		Source:       runtime.ServiceID(11),
		Senders:      clientSenderResolver{sender: sender},
		Codec:        valueCodec,
		PendingCalls: NewClusterCallRegistry(),
		MaxCallWait:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewClusterClient() error = %v, want nil", err)
	}
	if got := client.Source(); got != 11 {
		t.Fatalf("Source() = %d, want 11", got)
	}
	if err := client.Send("node-b", ServiceTarget{ServiceID: 7}, clientTestCommand, "hello", int64(3)); err != nil {
		t.Fatalf("Send() error = %v, want nil admission", err)
	}
	select {
	case envelope := <-received:
		if envelope.Kind != FrameRequest || envelope.Session != 0 {
			t.Fatalf("envelope kind/session = %v/%d, want request/0", envelope.Kind, envelope.Session)
		}
		if envelope.Target.ServiceID != 7 || envelope.Command != clientTestCommand {
			t.Fatalf("envelope route = %+v/%d, want service 7 command %d", envelope.Target, envelope.Command, clientTestCommand)
		}
		decoded, err := valueCodec.Decode(envelope.Payload)
		if err != nil {
			t.Fatalf("Decode(request payload) error = %v, want nil", err)
		}
		args, ok := decoded.([]any)
		if !ok || len(args) != 2 || args[0] != "hello" || args[1] != int64(3) {
			t.Fatalf("decoded args = %#v, want [hello 3]", decoded)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for encoded request")
	}
}

// 验证 ClusterClient 在构造阶段拒绝零 Source、缺失解析器或编解码器。
func TestNewClusterClientRejectsInvalidConfig(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	valid := ClusterClientConfig{
		Source:       runtime.ServiceID(11),
		Senders:      clientSenderResolver{},
		Codec:        valueCodec,
		PendingCalls: NewClusterCallRegistry(),
		MaxCallWait:  time.Second,
	}
	var invalidCalls ClusterCallRegistry
	cases := []ClusterClientConfig{
		{},
		{Source: valid.Source, Senders: valid.Senders},
		{Source: valid.Source, Codec: valid.Codec},
		{Senders: valid.Senders, Codec: valid.Codec},
		{Source: valid.Source, Senders: valid.Senders, Codec: valid.Codec, PendingCalls: valid.PendingCalls},
		{Source: valid.Source, Senders: valid.Senders, Codec: valid.Codec, MaxCallWait: time.Second},
		{Source: valid.Source, Senders: valid.Senders, Codec: valid.Codec, PendingCalls: valid.PendingCalls, MaxCallWait: -time.Second},
		{Source: valid.Source, Senders: valid.Senders, Codec: valid.Codec, PendingCalls: &invalidCalls, MaxCallWait: time.Second},
	}
	for index, config := range cases {
		if _, err := NewClusterClient(config); !errors.Is(err, ErrInvalidClusterClientConfig) {
			t.Fatalf("case %d NewClusterClient() error = %v, want ErrInvalidClusterClientConfig", index, err)
		}
	}
}

// 验证 ClusterClient 在 Send 前拒绝非法目标和空 sender，并保留底层错误。
func TestClusterClientRejectsInvalidSend(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	client, err := NewClusterClient(ClusterClientConfig{
		Source:       runtime.ServiceID(11),
		Senders:      clientSenderResolver{},
		Codec:        valueCodec,
		PendingCalls: NewClusterCallRegistry(),
		MaxCallWait:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewClusterClient() error = %v, want nil", err)
	}
	if err := client.Send("", ServiceTarget{ServiceID: 7}, clientTestCommand); !errors.Is(err, ErrInvalidNode) {
		t.Fatalf("empty node Send() error = %v, want ErrInvalidNode", err)
	}
	if err := client.Send("node-b", ServiceTarget{}, clientTestCommand); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("empty target Send() error = %v, want ErrInvalidEnvelope", err)
	}
	if err := client.Send("node-b", ServiceTarget{ServiceID: 7}, runtime.CommandID(0x7fff)); !errors.Is(err, ErrNodeSenderUnavailable) {
		t.Fatalf("unknown command Send() error = %v, want ErrNodeSenderUnavailable", err)
	}
	if err := client.Send("node-b", ServiceTarget{ServiceID: 7}, clientTestCommand); !errors.Is(err, ErrNodeSenderUnavailable) {
		t.Fatalf("nil sender Send() error = %v, want ErrNodeSenderUnavailable", err)
	}
}

// 验证 ClusterClient 编码失败和发送器接纳失败都向调用方返回稳定错误。
func TestClusterClientPropagatesLocalErrors(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	senderErr := errors.New("sender admission failed")
	client, err := NewClusterClient(ClusterClientConfig{
		Source:       runtime.ServiceID(11),
		Senders:      clientSenderResolver{sender: senderErrorStub{err: senderErr}},
		Codec:        valueCodec,
		PendingCalls: NewClusterCallRegistry(),
		MaxCallWait:  time.Second,
	})
	if err != nil {
		t.Fatalf("NewClusterClient() error = %v, want nil", err)
	}
	if err := client.Send("node-b", ServiceTarget{ServiceID: 7}, clientTestCommand, "ok"); !errors.Is(err, senderErr) {
		t.Fatalf("sender error = %v, want %v", err, senderErr)
	}
	if err := client.Send("node-b", ServiceTarget{ServiceID: 7}, clientTestCommand, func() {}); !errors.Is(err, ErrUnsupportedValue) {
		t.Fatalf("codec error = %v, want ErrUnsupportedValue", err)
	}
}

// 验证 ClusterClient.Call 分配非零 Session，并在匹配 Response 后解码结果。
func TestClusterClientCallReturnsDecodedResponse(t *testing.T) {
	calls := NewClusterCallRegistry()
	var responsePayload []byte
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	responsePayload, err = valueCodec.Encode("world")
	if err != nil {
		t.Fatalf("Encode(response) error = %v, want nil", err)
	}
	received := make(chan ClusterEnvelope, 1)
	client, transport, sender := newClientCallTestClient(t, calls, func(envelope ClusterEnvelope) {
		received <- envelope
		if envelope.Session == 0 {
			t.Errorf("request session = 0, want non-zero")
		}
		if err := calls.Complete("node-b", ClusterEnvelope{
			Kind:    FrameResponse,
			Session: envelope.Session,
			Target:  envelope.Target,
			Command: envelope.Command,
			Payload: responsePayload,
		}); err != nil {
			t.Errorf("Complete(response) error = %v, want nil", err)
		}
	}, time.Second)
	defer closeCallTestResources(t, transport, sender)

	value, err := client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand, "hello")
	if err != nil {
		t.Fatalf("Call() error = %v, want nil", err)
	}
	if value != "world" {
		t.Fatalf("Call() value = %#v, want world", value)
	}
	request := <-received
	decoded, err := valueCodec.Decode(request.Payload)
	if err != nil {
		t.Fatalf("Decode(request payload) error = %v, want nil", err)
	}
	args, ok := decoded.([]any)
	if !ok || len(args) != 1 || args[0] != "hello" {
		t.Fatalf("decoded args = %#v, want [hello]", decoded)
	}
	if got := calls.PendingCount(); got != 0 {
		t.Fatalf("PendingCount() = %d, want 0", got)
	}
}

// 验证远端 Error 帧会沿 Call 返回结构化 StatusError，而不是伪造业务载荷。
func TestClusterClientCallReturnsRemoteError(t *testing.T) {
	calls := NewClusterCallRegistry()
	wantErr := runtime.NewStatusError(runtime.ErrorClassDomain, runtime.ErrorCode("room.full"), "room is full")
	client, transport, sender := newClientCallTestClient(t, calls, func(envelope ClusterEnvelope) {
		if err := calls.Complete("node-b", ClusterEnvelope{
			Kind:    FrameError,
			Session: envelope.Session,
			Target:  envelope.Target,
			Command: envelope.Command,
			Error:   wantErr,
		}); err != nil {
			t.Errorf("Complete(error) error = %v, want nil", err)
		}
	}, time.Second)
	defer closeCallTestResources(t, transport, sender)

	value, err := client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand)
	if value != nil {
		t.Fatalf("Call() value = %#v, want nil on remote error", value)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("Call() error = %v, want %v", err, wantErr)
	}
	if got := calls.PendingCount(); got != 0 {
		t.Fatalf("PendingCount() = %d, want 0", got)
	}
}

// 验证 Runtime 最大等待时间会结束远程 Call，并在超时后拒绝迟到 Response。
func TestClusterClientCallHonorsMaxWait(t *testing.T) {
	calls := NewClusterCallRegistry()
	client, transport, sender := newClientCallTestClient(t, calls, func(ClusterEnvelope) {}, 25*time.Millisecond)
	defer closeCallTestResources(t, transport, sender)

	value, err := client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand)
	if value != nil {
		t.Fatalf("Call() value = %#v, want nil on timeout", value)
	}
	if !errors.Is(err, runtime.ErrCallTimeout) {
		t.Fatalf("Call() error = %v, want runtime.ErrCallTimeout", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call() error = %v, want context.DeadlineExceeded compatibility", err)
	}
	if got := calls.PendingCount(); got != 0 {
		t.Fatalf("PendingCount() = %d, want 0 after timeout", got)
	}
}

// 验证调用方更早的 deadline 优先于 MaxCallWait，并不伪装成 Runtime 最大等待超时。
func TestClusterClientCallHonorsEarlierDeadline(t *testing.T) {
	calls := NewClusterCallRegistry()
	client, transport, sender := newClientCallTestClient(t, calls, func(ClusterEnvelope) {}, time.Second)
	defer closeCallTestResources(t, transport, sender)

	callContext, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	value, err := client.Call(callContext, "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand)
	if value != nil {
		t.Fatalf("Call() value = %#v, want nil on earlier deadline", value)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Call() error = %v, want context.DeadlineExceeded", err)
	}
	if errors.Is(err, runtime.ErrCallTimeout) {
		t.Fatalf("Call() error = %v, want caller deadline rather than runtime timeout", err)
	}
	if got := calls.PendingCount(); got != 0 {
		t.Fatalf("PendingCount() = %d, want 0 after earlier deadline", got)
	}
}

// 验证调用方 context 可以缩短 Cluster Call 等待，并只取消自己的 PendingCall。
func TestClusterClientCallHonorsCallerCancellation(t *testing.T) {
	calls := NewClusterCallRegistry()
	received := make(chan struct{}, 1)
	client, transport, sender := newClientCallTestClient(t, calls, func(ClusterEnvelope) {
		received <- struct{}{}
	}, time.Second)
	defer closeCallTestResources(t, transport, sender)

	callContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	type outcome struct {
		value any
		err   error
	}
	outcomes := make(chan outcome, 1)
	go func() {
		value, err := client.Call(callContext, "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand)
		outcomes <- outcome{value: value, err: err}
	}()
	select {
	case <-received:
		cancel()
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for remote Call admission")
	}
	select {
	case result := <-outcomes:
		if result.value != nil {
			t.Fatalf("Call() value = %#v, want nil on cancellation", result.value)
		}
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("Call() error = %v, want context.Canceled", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cancelled Call")
	}
	if got := calls.PendingCount(); got != 0 {
		t.Fatalf("PendingCount() = %d, want 0 after cancellation", got)
	}
}

// 验证 ClusterCallRegistry.Close 会完成等待中的 Call，且关闭后不再接受迟到响应。
func TestClusterCallRegistryCloseCompletesClientCall(t *testing.T) {
	calls := NewClusterCallRegistry()
	received := make(chan ClusterEnvelope, 1)
	client, transport, sender := newClientCallTestClient(t, calls, func(envelope ClusterEnvelope) {
		received <- envelope
	}, time.Second)
	defer closeCallTestResources(t, transport, sender)

	outcomes := make(chan error, 1)
	go func() {
		_, err := client.Call(context.Background(), "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand)
		outcomes <- err
	}()
	var request ClusterEnvelope
	select {
	case request = <-received:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for remote Call admission")
	}
	if err := calls.Close(); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
	if err := calls.Close(); err != nil {
		t.Fatalf("second Close() error = %v, want nil", err)
	}
	select {
	case err := <-outcomes:
		if !errors.Is(err, ErrClusterCallClosed) {
			t.Fatalf("Call() error = %v, want ErrClusterCallClosed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for closed Call")
	}
	if got := calls.PendingCount(); got != 0 {
		t.Fatalf("PendingCount() = %d, want 0 after close", got)
	}
	if err := calls.Complete("node-b", ClusterEnvelope{
		Kind:    FrameResponse,
		Session: request.Session,
		Target:  request.Target,
		Command: request.Command,
	}); !errors.Is(err, ErrClusterSessionNotFound) {
		t.Fatalf("late Complete() error = %v, want ErrClusterSessionNotFound", err)
	}
}

// 验证 nil context 在建立 Cluster PendingCall 前被拒绝。
func TestClusterClientCallRejectsNilContext(t *testing.T) {
	calls := NewClusterCallRegistry()
	client, transport, sender := newClientCallTestClient(t, calls, func(ClusterEnvelope) {
		t.Error("nil-context Call unexpectedly reached remote receiver")
	}, time.Second)
	defer closeCallTestResources(t, transport, sender)

	value, err := client.Call(nil, "node-b", ServiceTarget{ServiceID: 7}, clientTestCommand)
	if value != nil {
		t.Fatalf("Call() value = %#v, want nil", value)
	}
	if !errors.Is(err, runtime.ErrInvalidCallContext) {
		t.Fatalf("Call() error = %v, want runtime.ErrInvalidCallContext", err)
	}
	if got := calls.PendingCount(); got != 0 {
		t.Fatalf("PendingCount() = %d, want 0", got)
	}
}

const clientTestCommand runtime.CommandID = 0x04100306

func closeClientSender(t *testing.T, sender *NodeSender) {
	t.Helper()
	if err := sender.Close(nil); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
	}
}

func newClientCallTestClient(t *testing.T, calls *ClusterCallRegistry, receiver Receiver, maxWait time.Duration) (*ClusterClient, *InMemoryTransport, *NodeSender) {
	t.Helper()
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	if err := transport.Register("node-b", receiver); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{

		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	client, err := NewClusterClient(ClusterClientConfig{
		Source:       runtime.ServiceID(11),
		Senders:      clientSenderResolver{sender: sender},
		Codec:        valueCodec,
		PendingCalls: calls,
		MaxCallWait:  maxWait,
	})
	if err != nil {
		closeClientSender(t, sender)
		transport.Close()
		t.Fatalf("NewClusterClient() error = %v, want nil", err)
	}
	return client, transport, sender
}

func closeCallTestResources(t *testing.T, transport *InMemoryTransport, sender *NodeSender) {
	t.Helper()
	closeClientSender(t, sender)
	if err := transport.Close(); err != nil {
		t.Fatalf("transport.Close() error = %v, want nil", err)
	}
}

type clientSenderResolver struct {
	sender OutboundSender
}

func (resolver clientSenderResolver) ResolveNodeSender(NodeID) (OutboundSender, error) {
	return resolver.sender, nil
}

type senderErrorStub struct {
	err error
}

func (stub senderErrorStub) Send(ClusterEnvelope) error {
	return stub.err
}
