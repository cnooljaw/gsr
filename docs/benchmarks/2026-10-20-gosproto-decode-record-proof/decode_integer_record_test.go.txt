package sproto

import (
	"encoding/binary"
	"errors"
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"
)

type proofRecord struct {
	Padding [32]byte
	First   *int64 `sproto:"integer,0"`
	Last    *int64 `sproto:"integer,1"`
}
type proofRows []*proofRecord
type proofNamedPointer *proofRecord
type proofDocument struct {
	Items  []*proofRecord `sproto:"struct,0,array"`
	Marker int
}
type proofNamedDocument struct {
	Items proofRows `sproto:"struct,0,array"`
}
type proofPointerDocument struct {
	Items []proofNamedPointer `sproto:"struct,0,array"`
}
type proofForeign struct {
	Small   byte
	Value   *int64 `sproto:"integer,0"`
	Padding [32]byte
}

// 验证整数记录数组保持逐对象/指针独占、低GC、命名类型、深度与失败原子性。
func TestIntegerRecordDecodeProofOwnership(t *testing.T) {
	old := debug.SetGCPercent(1)
	defer debug.SetGCPercent(old)
	for _, count := range []int{0, 1, 2, 3, 4, 7, 8, 9, 16, 128} {
		items := make([]*proofRecord, count)
		for i := range items {
			first, last := int64(i+1), -int64(i+1)
			items[i] = &proofRecord{First: &first, Last: &last}
		}
		named := make([]proofNamedPointer, count)
		for i, v := range items {
			named[i] = proofNamedPointer(v)
		}
		for _, sample := range []struct {
			value any
			fresh func() any
		}{
			{&proofDocument{Items: items}, func() any { return new(proofDocument) }},
			{&proofNamedDocument{Items: proofRows(items)}, func() any { return new(proofNamedDocument) }},
			{&proofPointerDocument{Items: named}, func() any { return new(proofPointerDocument) }},
		} {
			data, err := Encode(sample.value)
			if err != nil {
				t.Fatal(err)
			}
			output := sample.fresh()
			if used, err := Decode(data, output); err != nil || used != len(data) {
				t.Fatal(used, err)
			}
			before, err := Encode(output)
			if err != nil {
				t.Fatal(err)
			}
			again := sample.fresh()
			if _, err := Decode(data, again); err != nil {
				t.Fatal(err)
			}
			if count > 0 {
				a := reflect.ValueOf(output).Elem().Field(0)
				b := reflect.ValueOf(again).Elem().Field(0)
				if a.Index(0).Pointer() == b.Index(0).Pointer() {
					t.Fatal("record aliases another call")
				}
				if count > 1 && a.Index(0).Pointer() == a.Index(1).Pointer() {
					t.Fatal("records alias")
				}
				af := a.Index(0).Elem().FieldByName("First")
				bf := b.Index(0).Elem().FieldByName("First")
				if af.Pointer() == bf.Pointer() {
					t.Fatal("scalar aliases another call")
				}
				bf.Elem().SetInt(999)
				if af.Elem().Int() != 1 {
					t.Fatal("later decode changed earlier scalar")
				}
			}
			for i := range data {
				data[i] ^= 255
			}
			runtime.GC()
			after, err := Encode(output)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("input or GC changed result", err)
			}
		}
	}
	// 子项chunk边界合法但内容为空时，深度检查仍先于子消息长度检查。
	wire := make([]byte, 8+16*4)
	wire[0] = 1
	binary.LittleEndian.PutUint32(wire[4:], 16*4)
	for _, sample := range []struct {
		limit Limits
		want  error
	}{{Limits{MaxDepth: 1}, ErrLimit}, {Limits{MaxDepth: 2}, ErrDecode}} {
		output := proofDocument{Marker: 31}
		if _, err := DecodeWithLimits(wire, &output, sample.limit); !errors.Is(err, sample.want) || output.Marker != 31 {
			t.Fatal("child error order/atomicity", err)
		}
	}
	// 合法的两个child chunk中，第二项已声明标量的非法宽度仍必须拒绝。
	first, last := int64(1), int64(-2)
	damagedItems := make([]*proofRecord, 16)
	damagedItems[0] = &proofRecord{First: &first, Last: &last}
	damagedItems[1] = &proofRecord{Last: &last}
	data, err := Encode(&proofDocument{Items: damagedItems})
	if err != nil {
		t.Fatal(err)
	}
	second := 8 + 4 + int(binary.LittleEndian.Uint32(data[8:]))
	child := second + 4
	// child头包含跳跃项与非内联Last，最后一个数据长度为8，改成3而不改外围边界。
	headerEnd := child + 2 + 2*int(binary.LittleEndian.Uint16(data[child:]))
	binary.LittleEndian.PutUint32(data[headerEnd:], 3)
	output := proofDocument{Marker: 31}
	if _, err := Decode(data, &output); !errors.Is(err, ErrDecode) || output.Marker != 31 {
		t.Fatal("late scalar width/atomicity", err)
	}
}

// 验证当前可达字段证明不跳过负Tag，且nil/短Fields及外来owner保持原来的路由语义。
func TestIntegerRecordDecodeProofMetadata(t *testing.T) {
	first, last := int64(1), int64(2)
	items := make([]*proofRecord, 16)
	items[0] = &proofRecord{First: &first, Last: &last}
	wire, err := Encode(&proofDocument{Items: items})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := GetSprotoType(reflect.TypeFor[proofRecord]())
	if err != nil {
		t.Fatal(err)
	}
	original := append([]*SprotoField(nil), schema.Fields...)
	defer func() { schema.Fields = original }()
	// 负Tag不改变Decoder的私有路由；仅改Tag时仍应解出Last。
	oldTag := schema.Fields[2].Tag
	schema.Fields[2].Tag = -1
	var output proofDocument
	if _, err := Decode(wire, &output); err != nil || *output.Items[0].Last != 2 {
		t.Fatal("negative tag changed routing", err)
	}
	schema.Fields[2].Array = true
	sentinel := proofDocument{Marker: 31}
	if _, err := Decode(wire, &sentinel); !errors.Is(err, ErrDecode) || sentinel.Marker != 31 {
		t.Fatal("negative tag bypassed shape check", err)
	}
	schema.Fields[2].Array = false
	schema.Fields[2].Tag = oldTag
	// nil描述按未知字段处理。
	schema.Fields = append([]*SprotoField(nil), original...)
	schema.Fields[2] = nil
	output = proofDocument{}
	if _, err := Decode(wire, &output); err != nil || output.Items[0].Last != nil || *output.Items[0].First != 1 {
		t.Fatal("nil descriptor semantics", err)
	}
	schema.Fields = original
	empty, err := Encode(&proofDocument{Items: make([]*proofRecord, 16)})
	if err != nil {
		t.Fatal(err)
	}
	// 未访问字段的描述缺失不应因提前证明而产生新panic。
	schema.Fields = original[:1]
	output = proofDocument{}
	if _, err := Decode(empty, &output); err != nil || len(output.Items) != 16 {
		t.Fatal("short unused fields", err)
	}
	schema.Fields = original
	foreign, err := GetSprotoType(reflect.TypeFor[proofForeign]())
	if err != nil {
		t.Fatal(err)
	}
	schema.Fields = append([]*SprotoField(nil), original...)
	schema.Fields[2] = foreign.Fields[1]
	output = proofDocument{}
	if _, err := Decode(wire, &output); err != nil || *output.Items[0].First != 2 || output.Items[0].Last != nil {
		t.Fatal("foreign owner index semantics", err)
	}
}

type proofInteger int64

type proofGenericRecord struct {
	First *proofInteger `sproto:"integer,0"`
	Last  *proofInteger `sproto:"integer,1"`
}

type proofGenericDocument struct {
	Items  []*proofGenericRecord `sproto:"struct,0,array"`
	Marker int
}

// 验证整数记录数组与命名整数通用路径对变异wire给出相同结果、错误及消费长度。
func TestIntegerRecordDecodeProofWire(t *testing.T) {
	wrap := func(child []byte) []byte {
		wire := make([]byte, 8+16*(4+len(child)))
		wire[0] = 1
		binary.LittleEndian.PutUint32(wire[4:], uint32(len(wire)-8))
		for i := 0; i < 16; i++ {
			start := 8 + i*(4+len(child))
			binary.LittleEndian.PutUint32(wire[start:], uint32(len(child)))
			copy(wire[start+4:], child)
		}
		return wire
	}
	check := func(child []byte) {
		wire := wrap(child)
		for _, limits := range []Limits{{MaxBytes: 4096, MaxDepth: 2, MaxElements: 128}, {MaxBytes: 4096, MaxDepth: 1, MaxElements: 128}, {MaxBytes: 4096, MaxDepth: 2, MaxElements: 17}} {
			native := proofDocument{Marker: 31}
			generic := proofGenericDocument{Marker: 31}
			used, err := DecodeWithLimits(wire, &native, limits)
			referenceUsed, referenceErr := DecodeWithLimits(wire, &generic, limits)
			if used != referenceUsed || (err == nil) != (referenceErr == nil) || (err != nil && !errors.Is(err, referenceErr)) {
				t.Fatalf("child %x: used %d/%d error %v/%v", child, used, referenceUsed, err, referenceErr)
			}
			if err != nil {
				if native.Marker != 31 || generic.Marker != 31 || native.Items != nil || generic.Items != nil {
					t.Fatal("failed decode changed caller")
				}
				continue
			}
			encoded, encodeErr := Encode(&native)
			reference, referenceEncodeErr := Encode(&generic)
			if encodeErr != nil || referenceEncodeErr != nil || !reflect.DeepEqual(encoded, reference) {
				t.Fatalf("decoded values diverged for %x: %x/%x errors %v/%v", child, encoded, reference, encodeErr, referenceEncodeErr)
			}
		}
	}
	seeds := [][]byte{{0, 0}, {1, 0, 2, 0}, {2, 0, 2, 0, 254, 255}, {1, 0, 0, 0, 4, 0, 0, 0, 255, 255, 255, 255}, {1, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128},
		{3, 0, 255, 255, 253, 255, 16, 0}, {3, 0, 255, 255, 255, 255, 2, 0}, {2, 0, 3, 0, 0, 0, 3, 0, 0, 0, 1, 2, 3}, {255, 255},
	}
	for _, seed := range seeds {
		check(seed)
		for n := 0; n < len(seed); n++ {
			check(seed[:n])
		}
		for i := range seed {
			for bit := uint(0); bit < 8; bit++ {
				changed := append([]byte(nil), seed...)
				changed[i] ^= 1 << bit
				check(changed)
			}
		}
	}
}
