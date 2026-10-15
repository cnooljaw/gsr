package benchmarks

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	sproto "github.com/cnooljaw/gosproto"
	benchpb "github.com/cnooljaw/gsr/examples/websocket-gate/benchmarks/gen/pb"
	benchsp "github.com/cnooljaw/gsr/examples/websocket-gate/benchmarks/gen/sproto"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type representativeCase struct {
	Name string                   `json:"name"`
	Type string                   `json:"type"`
	Data json.RawMessage          `json:"data"`
	Wire map[string]wireSignature `json:"wire"`
}

type representativeOperation struct {
	name string
	run  func() error
}

var representativeSink any

func representativeCases(tb testing.TB) []representativeCase {
	tb.Helper()
	contents, err := os.ReadFile("representative.json")
	if err != nil {
		tb.Fatal(err)
	}
	var cases []representativeCase
	if err := json.Unmarshal(contents, &cases); err != nil {
		tb.Fatal(err)
	}
	return cases
}

func representativeMessages(typeName string) (func() proto.Message, func() any) {
	switch typeName {
	case "AddressBook":
		return func() proto.Message { return new(benchpb.AddressBook) }, func() any { return new(benchsp.AddressBook) }
	case "GameState":
		return func() proto.Message { return new(benchpb.GameState) }, func() any { return new(benchsp.GameState) }
	case "IntArrays":
		return func() proto.Message { return new(benchpb.IntArrays) }, func() any { return new(benchsp.IntArrays) }
	case "EchoRequest":
		return func() proto.Message { return new(benchpb.EchoRequest) }, func() any { return new(benchsp.EchoRequest) }
	default:
		panic("unknown benchmark fixture: " + typeName)
	}
}

func representativeOperations(tb testing.TB, sample representativeCase) ([]representativeOperation, map[string][]byte) {
	tb.Helper()
	newProtobuf, newSproto := representativeMessages(sample.Type)
	protobufMessage := newProtobuf()
	if err := protojson.Unmarshal(sample.Data, protobufMessage); err != nil {
		tb.Fatal(err)
	}
	sprotoMessage := newSproto()
	if err := json.Unmarshal(sample.Data, sprotoMessage); err != nil {
		tb.Fatal(err)
	}
	protobufWire, err := proto.Marshal(protobufMessage)
	if err != nil {
		tb.Fatal(err)
	}
	sprotoWire, err := sproto.Encode(sprotoMessage)
	if err != nil {
		tb.Fatal(err)
	}
	packed := sproto.Pack(sprotoWire)
	// 语义、消费长度及 padding 在计时外检查，避免测到未正确处理的消息。
	protobufDecoded := newProtobuf()
	if err := proto.Unmarshal(protobufWire, protobufDecoded); err != nil || !proto.Equal(protobufMessage, protobufDecoded) {
		tb.Fatalf("%s protobuf round trip: %v", sample.Name, err)
	}
	sprotoDecoded := newSproto()
	consumed, err := sproto.Decode(sprotoWire, sprotoDecoded)
	if err != nil || consumed != len(sprotoWire) || !reflect.DeepEqual(sprotoMessage, sprotoDecoded) {
		tb.Fatalf("%s sproto round trip: %v", sample.Name, err)
	}
	unpacked, err := sproto.Unpack(packed)
	if err != nil || len(unpacked) < len(sprotoWire) || len(unpacked)-len(sprotoWire) > 7 ||
		!bytes.Equal(unpacked[:len(sprotoWire)], sprotoWire) ||
		!bytes.Equal(unpacked[len(sprotoWire):], make([]byte, len(unpacked)-len(sprotoWire))) {
		tb.Fatalf("%s packing round trip: %v", sample.Name, err)
	}
	operations := []representativeOperation{
		{"protobuf_encode", func() error {
			wire, err := proto.Marshal(protobufMessage)
			encodedSink = wire
			return err
		}},
		{"protobuf_decode", func() error {
			message := newProtobuf()
			err := proto.Unmarshal(protobufWire, message)
			representativeSink = message
			return err
		}},
		{"sproto_encode", func() error {
			wire, err := sproto.Encode(sprotoMessage)
			encodedSink = wire
			return err
		}},
		{"sproto_decode", func() error {
			message := newSproto()
			_, err := sproto.Decode(sprotoWire, message)
			representativeSink = message
			return err
		}},
		{"sproto_pack", func() error {
			encodedSink = sproto.Pack(sprotoWire)
			return nil
		}},
		{"sproto_unpack", func() error {
			wire, err := sproto.Unpack(packed)
			encodedSink = wire
			return err
		}},
		{"sproto_pencode", func() error {
			wire, err := sproto.EncodePacked(sprotoMessage)
			encodedSink = wire
			return err
		}},
		{"sproto_pdecode", func() error {
			message := newSproto()
			err := sproto.DecodePacked(packed, message)
			representativeSink = message
			return err
		}},
	}
	return operations, map[string][]byte{
		"protobuf_encode": protobufWire, "sproto_encode": sprotoWire, "sproto_pack": packed,
	}
}

func checkRepresentativeWire(tb testing.TB, sample representativeCase, wires map[string][]byte) {
	tb.Helper()
	for name, wire := range wires {
		signature := sample.Wire[name]
		if len(wire) != signature.Bytes || fmt.Sprintf("%x", sha256.Sum256(wire)) != signature.SHA256 {
			tb.Fatalf("%s/%s differs from shared wire fixture", sample.Name, name)
		}
	}
}

// TestRepresentativeFixtures 保护官方 AddressBook 及游戏数值/数组的语义和固定跨语言 wire。
func TestRepresentativeFixtures(t *testing.T) {
	export := make(map[string]map[string]wireSignature)
	for _, sample := range representativeCases(t) {
		_, wires := representativeOperations(t, sample)
		if os.Getenv("GSR_BENCH_EXPORT_WIRE") == "" {
			checkRepresentativeWire(t, sample, wires)
		}
		export[sample.Name] = make(map[string]wireSignature)
		for name, wire := range wires {
			export[sample.Name][name] = wireSignature{Bytes: len(wire), SHA256: fmt.Sprintf("%x", sha256.Sum256(wire))}
		}
	}
	// 导出只写独立证据文件，不自动覆盖作为契约的输入签名。
	if path := os.Getenv("GSR_BENCH_EXPORT_WIRE"); path != "" {
		contents, err := json.MarshalIndent(export, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(contents, '\n'), 0644); err != nil {
			t.Fatal(err)
		}
	}
}

// BenchmarkRepresentativeCodec 分离纯编解码、packing 和组合路径，不包含消息封装与业务调度。
func BenchmarkRepresentativeCodec(b *testing.B) {
	for _, sample := range representativeCases(b) {
		operations, wires := representativeOperations(b, sample)
		checkRepresentativeWire(b, sample, wires)
		for _, operation := range operations {
			b.Run(sample.Name+"/"+operation.name, func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					if err := operation.run(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
