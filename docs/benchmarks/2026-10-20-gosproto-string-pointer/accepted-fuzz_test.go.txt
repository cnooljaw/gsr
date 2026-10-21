package sproto_test

import (
	"bytes"
	"math"
	"testing"

	sproto "github.com/cnooljaw/gosproto"
)

type fuzzIntegerRecord struct {
	First *int64 `sproto:"integer,0"`
	Last  *int64 `sproto:"integer,2"`
}

type fuzzDocument struct {
	NativeText *string              `sproto:"string,9"`
	Next       *fuzzDocument        `sproto:"struct,0"`
	Children   []*fuzzDocument      `sproto:"struct,1,array"`
	Names      []string             `sproto:"string,2,array"`
	Flags      []bool               `sproto:"boolean,3,array"`
	Values     []int64              `sproto:"integer,4,array"`
	Number     float64              `sproto:"double,5"`
	Binary     []byte               `sproto:"string,6"`
	Native     *int64               `sproto:"integer,7"`
	Records    []*fuzzIntegerRecord `sproto:"struct,8,array"`
}

// 验证任意有限字节的 pack/unpack 保留内容，只追加八字节对齐的零填充。
func FuzzPackRoundTrip(f *testing.F) {
	for _, seed := range [][]byte{nil, {0}, {1}, bytes.Repeat([]byte{1}, 8), bytes.Repeat([]byte{255}, 2048), bytes.Repeat([]byte{1}, 2056), bytes.Repeat([]byte{0, 1, 2, 3, 4, 5, 6, 0}, 20)} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		packed := sproto.Pack(data)
		expected := append([]byte{}, data...)
		expected = append(expected, make([]byte, (8-len(data)%8)%8)...)
		output, err := sproto.UnpackWithLimit(packed, max(1, len(expected)))
		if err != nil || !bytes.Equal(output, expected) {
			t.Fatalf("pack round trip: input=%d packed=%d output=%d expected=%d error=%v", len(data), len(packed), len(output), len(expected), err)
		}
		// 同一字节输入也经过组合入口，保护工作区重叠时的 FF 扩张和尾部写出。
		message := &struct {
			Binary []byte `sproto:"string,0"`
		}{Binary: data}
		encoded, err := sproto.Encode(message)
		if err != nil {
			t.Fatal(err)
		}
		combined, err := sproto.EncodePacked(message)
		want := sproto.Pack(encoded)
		if err != nil || !bytes.Equal(combined, want) || cap(combined) != cap(want) {
			t.Fatalf("packed workspace: input=%d err=%v", len(data), err)
		}
	})
}

// 验证包含嵌套、集合和任意 double 位模式的有限值往返后得到相同的规范编码。
func FuzzCodecRoundTrip(f *testing.F) {
	for _, seed := range []struct {
		data []byte
		bits uint64
	}{{nil, 0}, {[]byte("中文🙂"), 1 << 63}, {[]byte{0, 255, 1}, 0x7ff8000000000042}, {[]byte{3, 1, 2, 3}, 0x7ff0000000000001}} {
		f.Add(seed.data, seed.bits)
	}
	f.Fuzz(func(t *testing.T, data []byte, bits uint64) {
		if len(data) > 256 {
			return
		}
		input := makeFuzzDocument(data, bits)
		limits := sproto.Limits{MaxBytes: 16384, MaxDepth: 8, MaxElements: 128}
		encoded, err := sproto.EncodeWithLimits(input, limits)
		if err != nil {
			t.Fatal(err)
		}
		var output fuzzDocument
		if used, err := sproto.DecodeWithLimits(encoded, &output, limits); err != nil || used != len(encoded) {
			t.Fatalf("decode: consumed=%d length=%d error=%v", used, len(encoded), err)
		}
		reencoded, err := sproto.EncodeWithLimits(&output, limits)
		if err != nil || !bytes.Equal(encoded, reencoded) {
			t.Fatalf("canonical bytes changed: %x -> %x error=%v", encoded, reencoded, err)
		}
		packed, err := sproto.EncodePacked(input)
		want := sproto.Pack(encoded)
		if err != nil || !bytes.Equal(packed, want) || cap(packed) != cap(want) {
			t.Fatalf("packed composition changed: err=%v", err)
		}
	})
}

func makeFuzzDocument(data []byte, bits uint64) *fuzzDocument {
	text := string(data)
	number := int64(bits)
	other := int64(^bits)
	root := &fuzzDocument{
		NativeText: &text,
		Native:     &number,
		Records:    []*fuzzIntegerRecord{{First: &number, Last: &other}, nil, {}},
		Names:      []string{"", string(data)}, Flags: []bool{false, true},
		Values: []int64{-1, int64(bits)}, Number: math.Float64frombits(bits),
		Binary:   append([]byte{}, data...),
		Children: []*fuzzDocument{{Number: math.Float64frombits(bits), Native: &number}, {}},
	}
	if len(data) != 0 {
		current := root
		for depth := 0; depth < int(data[0]%4); depth++ {
			current.Next = &fuzzDocument{Names: []string{string(data)}, Number: math.Float64frombits(bits)}
			current = current.Next
		}
	}
	return root
}
