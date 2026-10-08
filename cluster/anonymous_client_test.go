package cluster

import (
	"context"
	"errors"
	"testing"
	"time"
)

// 验证未开放接收能力的发送器可向目标投递，并记录目标不可达错误。
func TestNodeSenderOutboundWithoutListener(t *testing.T) {
	transport, err := NewInMemoryTransport(InMemoryTransportConfig{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { transport.Close() })
	received := make(chan struct{}, 1)
	if err := transport.Register("node-b", func(_ ClusterEnvelope) {
		received <- struct{}{}
	}); err != nil {
		t.Fatal(err)
	}
	sender, err := NewNodeSender(NodeSenderConfig{RemoteNode: "node-b", Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := sender.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	request := ClusterEnvelope{Kind: FrameRequest, Target: ServiceTarget{ServiceID: 1}, Command: clusterTestCommand}
	if err := sender.Send(request); err != nil {
		t.Fatal(err)
	}
	select {
	case <-received:
	case <-time.After(time.Second):
		t.Fatal("anonymous Send was not delivered")
	}
	if err := transport.Unregister("node-b"); err != nil {
		t.Fatal(err)
	}
	if err := sender.Send(request); err != nil {
		t.Fatal(err)
	}
	select {
	case failure := <-sender.Failures():
		if failure.RemoteNode != "node-b" || !errors.Is(failure.Err, ErrNodeNotFound) {
			t.Fatalf("anonymous delivery failure = %+v", failure)
		}
	case <-time.After(time.Second):
		t.Fatal("missing anonymous delivery diagnostic")
	}
}

// 验证匿名入站 Call 无需回查来源地址，按原 Session 经连接 writer 回复。
func TestLocalAdapterReplyWithoutSource(t *testing.T) {
	adapter, id, codec := newLocalAdapterTest(t)
	payload, err := codec.Encode([]any{"result"})
	if err != nil {
		t.Fatal(err)
	}
	request := ClusterEnvelope{Kind: FrameRequest, Session: 7, Target: ServiceTarget{ServiceID: id}, Command: clusterTestCommand, Payload: payload}
	replied := false
	if err := adapter.handle(context.Background(), request, func(response ClusterEnvelope) error {
		replied = true
		if response.Kind != FrameResponse || response.Session != request.Session || response.Target != request.Target || response.Command != request.Command {
			t.Errorf("response = %+v", response)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !replied {
		t.Fatal("missing reply on original connection")
	}
}
