package sproto

import (
	"bytes"
	"math"
	"reflect"
	"runtime"
	"runtime/debug"
	"testing"
)

// TestEncodeRecordPointerSlices 保护不同指针切片形状的当前值、nil语义、GC存活、输出独占与预算。
func TestEncodeRecordPointerSlices(t *testing.T) {
	type record struct {
		ignored [19]byte
		First   *int64 `sproto:"integer,0"`
		Last    *int64 `sproto:"integer,3"`
	}
	type pointer *record
	type slice []*record
	type document struct {
		Records  []*record `sproto:"struct,0,array"`
		Named    slice     `sproto:"struct,1,array"`
		Pointers []pointer `sproto:"struct,2,array"`
		Tail     *int64    `sproto:"integer,4"`
	}
	type integer int64
	type genericRecord struct {
		First *integer `sproto:"integer,0"`
		Last  *integer `sproto:"integer,3"`
	}
	type genericDocument struct {
		Records  []*genericRecord `sproto:"struct,0,array"`
		Named    []*genericRecord `sproto:"struct,1,array"`
		Pointers []*genericRecord `sproto:"struct,2,array"`
		Tail     *integer         `sproto:"integer,4"`
	}
	previousGC := debug.SetGCPercent(1)
	defer debug.SetGCPercent(previousGC)
	values := []int64{0, MaxEmbeddedInt, MaxEmbeddedInt + 1, -1, MinInt32, MinInt32 - 1, math.MinInt64, math.MaxInt64}
	for _, count := range []int{0, 1, 3, 32, 33, 129} {
		input := &document{Records: make([]*record, count), Named: make(slice, count), Pointers: make([]pointer, count)}
		reference := &genericDocument{Records: make([]*genericRecord, count), Named: make([]*genericRecord, count), Pointers: make([]*genericRecord, count)}
		var first, last int64
		a, b := integer(0), integer(0)
		for i := 0; i < count; i++ {
			if i%3 == 1 {
				continue
			}
			native := &record{First: &first, Last: &last}
			generic := &genericRecord{First: &a, Last: &b}
			if i%3 == 2 {
				native.First = nil
				generic.First = nil
			}
			input.Records[i], input.Named[i], input.Pointers[i] = native, native, pointer(native)
			reference.Records[i], reference.Named[i], reference.Pointers[i] = generic, generic, generic
		}
		input.Tail = &first
		reference.Tail = &a
		for i, value := range values {
			first, last = value, values[len(values)-1-i]
			a, b = integer(first), integer(last)
			runtime.GC()
			wire, err := Encode(input)
			want, wantErr := Encode(reference)
			if err != nil || wantErr != nil || !bytes.Equal(wire, want) || len(wire) != cap(wire) {
				t.Fatalf("wire count %d/value %d: %v/%v", count, value, err, wantErr)
			}
			snapshot := bytes.Clone(wire)
			packed, err := EncodePacked(input)
			if err != nil || !bytes.Equal(packed, Pack(want)) {
				t.Fatalf("packed count %d/value %d: %v", count, value, err)
			}
			first++
			again, err := Encode(input)
			first--
			if err != nil || bytes.Equal(again, wire) || !bytes.Equal(snapshot, wire) {
				t.Fatal("current value or output ownership changed")
			}
			if input.Tail != &first || *input.Tail != value {
				t.Fatal("input changed")
			}
			for _, maxBytes := range []int{len(wire) - 1, len(wire)} {
				result, gotErr := EncodeWithLimits(input, Limits{MaxBytes: maxBytes})
				if maxBytes < len(wire) {
					if gotErr == nil || result != nil {
						t.Fatal("byte budget admitted short limit")
					}
				} else if gotErr != nil || !bytes.Equal(result, wire) {
					t.Fatal("exact byte budget rejected")
				}
			}
			var decoded document
			used, err := Decode(wire, &decoded)
			var referenceDecoded document
			referenceUsed, referenceErr := Decode(want, &referenceDecoded)
			if err != nil || referenceErr != nil || used != len(wire) || referenceUsed != used || !reflect.DeepEqual(decoded, referenceDecoded) {
				t.Fatal("wire decode differs")
			}
			runtime.KeepAlive(input)
		}
	}
}
