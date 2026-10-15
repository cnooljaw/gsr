package benchmarks

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	sproto "github.com/cnooljaw/gosproto"
	echopb "github.com/cnooljaw/gsr/examples/websocket-gate/gen/go/gsr/example/echo/v1"
	"github.com/cnooljaw/gsr/examples/websocket-gate/gen/sproto/c2s"
	commonpb "github.com/cnooljaw/gsr/protocol/gen/go/gsr/common/v1"
	"google.golang.org/protobuf/proto"
)

type benchmarkCase struct {
	Name    string                   `json:"name"`
	Pattern string                   `json:"pattern"`
	Repeat  int                      `json:"repeat"`
	Wire    map[string]wireSignature `json:"wire"`
}

type wireSignature struct {
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

// benchmarkCodec 仅组织基准操作，不进入生产代码或公共协议边界。
type benchmarkCodec struct {
	encode func() ([]byte, error)
	decode func([]byte) (string, error)
}

var encodedSink []byte
var decodedSink string

// BenchmarkProtocolCodec 比较相同 Echo 内容的 payload 和应用帧编解码，不包含 Gate 或业务分发。
func BenchmarkProtocolCodec(b *testing.B) {
	contents, err := os.ReadFile("cases.json")
	if err != nil {
		b.Fatal(err)
	}
	var cases []benchmarkCase
	if err := json.Unmarshal(contents, &cases); err != nil {
		b.Fatal(err)
	}
	for _, sample := range cases {
		text := strings.Repeat(sample.Pattern, sample.Repeat)
		codecs := echoCodecs(text)
		for _, name := range []string{"protobuf_payload", "sproto_payload", "protobuf_frame", "sproto_frame"} {
			codec := codecs[name]
			wire, err := codec.encode()
			if err != nil {
				b.Fatal(err)
			}
			signature := sample.Wire[name]
			if len(wire) != signature.Bytes || fmt.Sprintf("%x", sha256.Sum256(wire)) != signature.SHA256 {
				b.Fatalf("%s/%s wire differs from shared fixture", sample.Name, name)
			}
			decoded, err := codec.decode(wire)
			if err != nil || decoded != text {
				b.Fatalf("%s/%s round trip: %v", sample.Name, name, err)
			}
			b.Run(sample.Name+"/"+name+"/encode", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				b.ResetTimer()
				for range b.N {
					encodedSink, err = codec.encode()
					if err != nil {
						b.Fatal(err)
					}
				}
			})
			b.Run(sample.Name+"/"+name+"/decode", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(text)))
				b.ResetTimer()
				for range b.N {
					decodedSink, err = codec.decode(wire)
					if err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func echoCodecs(text string) map[string]benchmarkCodec {
	protobufMessage := &echopb.EchoRequest{Text: text}
	sprotoMessage := &c2s.EchoRequest{Text: &text}
	typeID, session := int64(1), int64(1)
	protobufPayload := benchmarkCodec{
		encode: func() ([]byte, error) {
			return proto.Marshal(protobufMessage)
		},
		decode: func(wire []byte) (string, error) {
			var message echopb.EchoRequest
			err := proto.Unmarshal(wire, &message)
			return message.GetText(), err
		},
	}
	sprotoPayload := benchmarkCodec{
		encode: func() ([]byte, error) {
			return sproto.Encode(sprotoMessage)
		},
		decode: func(wire []byte) (string, error) {
			var message c2s.EchoRequest
			_, err := sproto.Decode(wire, &message)
			if err != nil {
				return "", err
			}
			return *message.Text, nil
		},
	}
	return map[string]benchmarkCodec{
		"protobuf_payload": protobufPayload,
		"sproto_payload":   sprotoPayload,
		"protobuf_frame": {
			encode: func() ([]byte, error) {
				payload, err := protobufPayload.encode()
				if err != nil {
					return nil, err
				}
				return proto.Marshal(&commonpb.Envelope{
					MsgId: uint32(echopb.MsgId_C2S_ECHO_REQUEST), RequestId: 1, Payload: payload,
				})
			},
			decode: func(wire []byte) (string, error) {
				var envelope commonpb.Envelope
				if err := proto.Unmarshal(wire, &envelope); err != nil {
					return "", err
				}
				return protobufPayload.decode(envelope.Payload)
			},
		},
		"sproto_frame": {
			encode: func() ([]byte, error) {
				prefix, err := sproto.Encode(&c2s.Package{Type: &typeID, Session: &session})
				if err != nil {
					return nil, err
				}
				payload, err := sprotoPayload.encode()
				if err != nil {
					return nil, err
				}
				return sproto.Pack(append(prefix, payload...)), nil
			},
			decode: func(wire []byte) (string, error) {
				unpacked, err := sproto.Unpack(wire)
				if err != nil {
					return "", err
				}
				var prefix c2s.Package
				consumed, err := sproto.Decode(unpacked, &prefix)
				if err != nil {
					return "", err
				}
				return sprotoPayload.decode(unpacked[consumed:])
			},
		},
	}
}
