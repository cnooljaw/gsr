package clientv1_test

import (
	"bytes"
	"testing"

	clientv1 "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/client/v1"
	"google.golang.org/protobuf/proto"
)

// TestEnvelopeWireFieldsRemainCompatible 验证 Envelope 的字段编号和 Protobuf 编码保持稳定。
func TestEnvelopeWireFieldsRemainCompatible(t *testing.T) {
	message := &clientv1.Envelope{
		MsgId:     1001,
		RequestId: 42,
		Payload:   []byte{0x08, 0x01},
	}

	encoded, err := proto.Marshal(message)
	if err != nil {
		t.Fatalf("marshal Envelope: %v", err)
	}

	want := []byte{0x08, 0xe9, 0x07, 0x10, 0x2a, 0x1a, 0x02, 0x08, 0x01}
	if !bytes.Equal(encoded, want) {
		t.Fatalf("unexpected wire bytes: got %x, want %x", encoded, want)
	}

	var decoded clientv1.Envelope
	if err := proto.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal Envelope: %v", err)
	}
	if !proto.Equal(message, &decoded) {
		t.Fatalf("round trip mismatch: got %v, want %v", &decoded, message)
	}
}
