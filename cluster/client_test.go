package cluster

import (
	"errors"
	"testing"
	"time"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// 验证 source-bound ClusterClient 能编码参数并按目标 NodeSender 投递 Request。
func TestClusterClientSendsEncodedRequest(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	registry, err := NewCommandCodecRegistry([]CommandCodecEntry{{CommandID: clientTestCommand, Codec: valueCodec}})
	if err != nil {
		t.Fatalf("NewCommandCodecRegistry() error = %v, want nil", err)
	}
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatalf("NewInMemoryTransport() error = %v, want nil", err)
	}
	received := make(chan ClusterEnvelope, 1)
	if err := transport.Register("node-b", func(_ NodeID, envelope ClusterEnvelope) {
		received <- envelope
	}); err != nil {
		t.Fatalf("Register() error = %v, want nil", err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{
		LocalNode:  "node-a",
		RemoteNode: "node-b",
		Transport:  transport,
	})
	if err != nil {
		t.Fatalf("NewNodeSender() error = %v, want nil", err)
	}
	defer closeClientSender(t, sender)
	client, err := NewClusterClient(ClusterClientConfig{
		Source:  runtime.ServiceID(11),
		Senders: clientSenderResolver{sender: sender},
		Codecs:  registry,
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

// 验证 ClusterClient 在构造阶段拒绝零 Source、缺失 resolver 或 codec registry。
func TestNewClusterClientRejectsInvalidConfig(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	registry, err := NewCommandCodecRegistry([]CommandCodecEntry{{CommandID: clientTestCommand, Codec: valueCodec}})
	if err != nil {
		t.Fatalf("NewCommandCodecRegistry() error = %v, want nil", err)
	}
	valid := ClusterClientConfig{
		Source:  runtime.ServiceID(11),
		Senders: clientSenderResolver{},
		Codecs:  registry,
	}
	cases := []ClusterClientConfig{
		{},
		{Source: valid.Source, Senders: valid.Senders},
		{Source: valid.Source, Codecs: valid.Codecs},
		{Senders: valid.Senders, Codecs: valid.Codecs},
	}
	for index, config := range cases {
		if _, err := NewClusterClient(config); !errors.Is(err, ErrInvalidClusterClientConfig) {
			t.Fatalf("case %d NewClusterClient() error = %v, want ErrInvalidClusterClientConfig", index, err)
		}
	}
}

// 验证 ClusterClient 在 Send 前拒绝未知命令、非法目标和空 sender，并保留底层错误。
func TestClusterClientRejectsInvalidSend(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	registry, err := NewCommandCodecRegistry([]CommandCodecEntry{{CommandID: clientTestCommand, Codec: valueCodec}})
	if err != nil {
		t.Fatalf("NewCommandCodecRegistry() error = %v, want nil", err)
	}
	client, err := NewClusterClient(ClusterClientConfig{
		Source:  runtime.ServiceID(11),
		Senders: clientSenderResolver{},
		Codecs:  registry,
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
	if err := client.Send("node-b", ServiceTarget{ServiceID: 7}, runtime.CommandID(0x7fff)); !errors.Is(err, ErrCommandCodecNotFound) {
		t.Fatalf("unknown command Send() error = %v, want ErrCommandCodecNotFound", err)
	}
	if err := client.Send("node-b", ServiceTarget{ServiceID: 7}, clientTestCommand); !errors.Is(err, ErrNodeSenderUnavailable) {
		t.Fatalf("nil sender Send() error = %v, want ErrNodeSenderUnavailable", err)
	}
}

// 验证 ClusterClient 编码失败和 sender admission 失败都继续向调用方返回稳定错误。
func TestClusterClientPropagatesLocalErrors(t *testing.T) {
	valueCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	registry, err := NewCommandCodecRegistry([]CommandCodecEntry{{CommandID: clientTestCommand, Codec: valueCodec}})
	if err != nil {
		t.Fatalf("NewCommandCodecRegistry() error = %v, want nil", err)
	}
	senderErr := errors.New("sender admission failed")
	client, err := NewClusterClient(ClusterClientConfig{
		Source:  runtime.ServiceID(11),
		Senders: clientSenderResolver{sender: senderErrorStub{err: senderErr}},
		Codecs:  registry,
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

const clientTestCommand runtime.CommandID = 0x04100306

func closeClientSender(t *testing.T, sender *NodeSender) {
	t.Helper()
	if err := sender.Close(nil); err != nil {
		t.Fatalf("Close() error = %v, want nil", err)
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
