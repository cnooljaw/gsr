package cluster

import (
	"encoding/binary"
	"errors"
	"math"
	"reflect"
	goruntime "runtime"
	"testing"
)

// 验证默认值 Codec 能往返支持的标量、字节串、列表和字符串键映射。
func TestValueCodecRoundTripsSupportedValues(t *testing.T) {
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}

	cases := []struct {
		name  string
		value any
		want  any
	}{
		{name: "nil", value: nil, want: nil},
		{name: "bool", value: true, want: true},
		{name: "false", value: false, want: false},
		{name: "signed integer", value: int32(-7), want: int64(-7)},
		{name: "minimum signed integer", value: int64(math.MinInt64), want: int64(math.MinInt64)},
		{name: "maximum signed integer", value: int64(math.MaxInt64), want: int64(math.MaxInt64)},
		{name: "unsigned integer", value: uint16(9), want: uint64(9)},
		{name: "maximum unsigned integer", value: uint64(math.MaxUint64), want: uint64(math.MaxUint64)},
		{name: "float", value: float32(1.5), want: float64(1.5)},
		{name: "float64", value: float64(2.5), want: float64(2.5)},
		{name: "string", value: "hello", want: "hello"},
		{name: "unicode string", value: "你好，GSR", want: "你好，GSR"},
		{name: "empty string", value: "", want: ""},
		{name: "bytes", value: []byte{1, 2, 3}, want: []byte{1, 2, 3}},
		{name: "empty bytes", value: []byte{}, want: []byte(nil)},
		{name: "nil bytes", value: []byte(nil), want: []byte(nil)},
		{name: "integer slice", value: []int{1, -2}, want: []any{int64(1), int64(-2)}},
		{name: "string slice", value: []string{"a", "b"}, want: []any{"a", "b"}},
		{name: "array", value: [2]int32{1, -2}, want: []any{int64(1), int64(-2)}},
		{name: "byte array", value: [2]byte{1, 2}, want: []any{uint64(1), uint64(2)}},
		{name: "empty list", value: []any{}, want: []any{}},
		{name: "nil list", value: []any(nil), want: []any{}},
		{name: "empty map", value: map[string]any{}, want: map[string]any{}},
		{name: "nil map", value: map[string]any(nil), want: map[string]any{}},
		{name: "typed map", value: map[string]int{"count": 3}, want: map[string]any{"count": int64(3)}},
		{
			name: "list",
			value: []any{
				"first",
				int(2),
				[]byte("third"),
			},
			want: []any{"first", int64(2), []byte("third")},
		},
		{
			name: "map",
			value: map[string]any{
				"name":  "room",
				"count": uint64(3),
				"items": []any{false, float64(2.5)},
			},
			want: map[string]any{
				"name":  "room",
				"count": uint64(3),
				"items": []any{false, float64(2.5)},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			payload, err := codec.Encode(testCase.value)
			if err != nil {
				t.Fatalf("Encode() error = %v, want nil", err)
			}
			got, err := codec.Decode(payload)
			if err != nil {
				t.Fatalf("Decode() error = %v, want nil", err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("Decode() = %#v, want %#v", got, testCase.want)
			}
		})
	}
}

// 验证 map 的编码顺序稳定，避免相同值生成不同载荷影响缓存和诊断。
func TestValueCodecMapEncodingIsDeterministic(t *testing.T) {
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}
	value := map[string]any{
		"z": int64(1),
		"a": int64(2),
		"m": int64(3),
	}
	first, err := codec.Encode(value)
	if err != nil {
		t.Fatalf("first Encode() error = %v, want nil", err)
	}
	second, err := codec.Encode(value)
	if err != nil {
		t.Fatalf("second Encode() error = %v, want nil", err)
	}
	if string(first) != string(second) {
		t.Fatalf("map payloads differ: %x != %x", first, second)
	}
}

// 验证默认值 Codec 拒绝指针、函数、复杂键类型和非有限浮点等不稳定值。
func TestValueCodecRejectsUnsupportedValues(t *testing.T) {
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}

	value := 1
	cases := []struct {
		name  string
		value any
	}{
		{name: "pointer", value: &value},
		{name: "function", value: func() {}},
		{name: "struct", value: struct{ Name string }{Name: "room"}},
		{name: "integer-key map", value: map[int]string{1: "room"}},
		{name: "complex", value: complex(1, 2)},
		{name: "uintptr", value: uintptr(1)},
		{name: "nan", value: math.NaN()},
		{name: "infinity", value: math.Inf(1)},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := codec.Encode(testCase.value); !errors.Is(err, ErrUnsupportedValue) {
				t.Fatalf("Encode() error = %v, want ErrUnsupportedValue", err)
			}
		})
	}
}

// 验证 Codec 的最大嵌套深度和载荷上限在编码、解码两侧都生效。
func TestValueCodecEnforcesDepthAndPayloadLimits(t *testing.T) {
	depthCodec, err := NewValueCodec(ValueCodecConfig{MaxDepth: 1})
	if err != nil {
		t.Fatalf("NewValueCodec(depth) error = %v, want nil", err)
	}
	if _, err := depthCodec.Encode([]any{[]any{"too deep"}}); !errors.Is(err, ErrMaxDepthExceeded) {
		t.Fatalf("deep Encode() error = %v, want ErrMaxDepthExceeded", err)
	}
	defaultCodec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec(default) error = %v, want nil", err)
	}
	deepPayload, err := defaultCodec.Encode([]any{[]any{"too deep"}})
	if err != nil {
		t.Fatalf("default Encode() error = %v, want nil", err)
	}
	if _, err := depthCodec.Decode(deepPayload); !errors.Is(err, ErrMaxDepthExceeded) {
		t.Fatalf("deep Decode() error = %v, want ErrMaxDepthExceeded", err)
	}

	payloadCodec, err := NewValueCodec(ValueCodecConfig{MaxPayload: 4})
	if err != nil {
		t.Fatalf("NewValueCodec(payload) error = %v, want nil", err)
	}
	if _, err := payloadCodec.Encode("too large"); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("large Encode() error = %v, want ErrPayloadTooLarge", err)
	}
	if _, err := payloadCodec.Decode([]byte{byte(valueTagString), 0xff, 0xff, 0xff, 0xff}); !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("large Decode() error = %v, want ErrPayloadTooLarge", err)
	}
}

// 验证 Codec 对空数据、未知标签、截断数据和尾随数据返回结构化 malformed 错误。
func TestValueCodecRejectsMalformedPayload(t *testing.T) {
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatalf("NewValueCodec() error = %v, want nil", err)
	}

	valid, err := codec.Encode("valid")
	if err != nil {
		t.Fatalf("Encode() error = %v, want nil", err)
	}
	cases := [][]byte{
		nil,
		{0xff},
		valid[:len(valid)-1],
		append(valid, 0),
	}
	for index, payload := range cases {
		if _, err := codec.Decode(payload); !errors.Is(err, ErrMalformedPayload) {
			t.Fatalf("case %d Decode() error = %v, want ErrMalformedPayload", index, err)
		}
	}
}

// 验证坏 map 的数量字段不会在拒绝载荷前触发远大于输入的大容量预分配。
func TestValueCodecRejectsInflatedMapCountBeforeAllocation(t *testing.T) {
	codec, err := NewValueCodec(ValueCodecConfig{})
	if err != nil {
		t.Fatal(err)
	}
	payload := make([]byte, 64<<10)
	payload[0] = byte(valueTagMap)
	binary.LittleEndian.PutUint32(payload[1:5], uint32(len(payload)-5))
	var before, after goruntime.MemStats
	goruntime.ReadMemStats(&before)
	_, err = codec.Decode(payload)
	goruntime.ReadMemStats(&after)
	if !errors.Is(err, ErrMalformedPayload) {
		t.Fatalf("Decode = %v", err)
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 1<<20 {
		t.Fatalf("invalid 64 KiB map allocated %d bytes before rejection", allocated)
	}
}

// 验证 Codec 构造阶段拒绝非法配置，避免运行期出现不可控限制。
func TestNewValueCodecRejectsInvalidConfig(t *testing.T) {
	cases := []ValueCodecConfig{
		{MaxDepth: -1},
		{MaxPayload: -1},
	}
	for _, config := range cases {
		if _, err := NewValueCodec(config); !errors.Is(err, ErrInvalidCodecConfig) {
			t.Fatalf("NewValueCodec(%+v) error = %v, want ErrInvalidCodecConfig", config, err)
		}
	}
}
