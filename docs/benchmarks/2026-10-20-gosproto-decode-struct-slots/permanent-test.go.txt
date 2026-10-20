package sproto_test

import (
	"errors"
	sproto "github.com/cnooljaw/gosproto"
	"reflect"
	"runtime"
	"runtime/debug"
	"strconv"
	"testing"
)

type structSlotChild struct {
	Number *int64  `sproto:"integer,0"`
	Text   *string `sproto:"string,1"`
}
type structSlotNamedPointer *structSlotChild
type structSlotNamedSlice []*structSlotChild
type structSlotDocument struct {
	Pad          int64                    `sproto:"integer,0"`
	Children     []*structSlotChild       `sproto:"struct,1,array"`
	Named        structSlotNamedSlice     `sproto:"struct,2,array"`
	PointerAlias []structSlotNamedPointer `sproto:"struct,3,array"`
	Tail         string                   `sproto:"string,4"`
}

// TestStructSliceDecodeOwnershipAndGC 保护不同切片形状的GC存活、元素与调用独占，以及坏尾部失败原子性。
func TestStructSliceDecodeOwnershipAndGC(t *testing.T) {
	oldGC := debug.SetGCPercent(1)
	defer debug.SetGCPercent(oldGC)
	var retained []*structSlotDocument
	for _, count := range []int{0, 1, 3, 64, 128} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			input := &structSlotDocument{Pad: 17, Tail: "tail", Children: make([]*structSlotChild, count), Named: make(structSlotNamedSlice, count), PointerAlias: make([]structSlotNamedPointer, count)}
			for i := 0; i < count; i++ {
				number := int64(i) * (-1 << 33)
				text := "child-" + strconv.Itoa(i)
				child := &structSlotChild{Number: &number, Text: &text}
				input.Children[i], input.Named[i], input.PointerAlias[i] = child, child, structSlotNamedPointer(child)
			}
			wire, err := sproto.Encode(input)
			if err != nil {
				t.Fatal(err)
			}
			var first, second structSlotDocument
			ub, eb := sproto.Decode(wire, &first)
			uc, ec := sproto.Decode(wire, &second)
			if eb != nil || ec != nil || ub != len(wire) || uc != ub || !reflect.DeepEqual(&first, input) || !reflect.DeepEqual(first, second) {
				t.Fatalf("decode: %d/%d %v/%v", ub, uc, eb, ec)
			}
			for _, v := range []*structSlotDocument{&first, &second} {
				if len(v.Children) != count || cap(v.Children) != count || len(v.Named) != count || cap(v.Named) != count || len(v.PointerAlias) != count || cap(v.PointerAlias) != count {
					t.Fatal("length/capacity changed")
				}
				for i := 0; i < count; i++ {
					child, named, alias := v.Children[i], v.Named[i], (*structSlotChild)(v.PointerAlias[i])
					if child == named || child == alias || named == alias || child.Number == named.Number || child.Number == alias.Number || child.Text == named.Text {
						t.Fatal("independent fields share storage")
					}
					if *child.Number != int64(i)*(-1<<33) || *child.Text != "child-"+strconv.Itoa(i) {
						t.Fatal("values changed")
					}
					if i > 0 && (child == v.Children[i-1] || child.Number == v.Children[i-1].Number) {
						t.Fatal("independent elements share storage")
					}
				}
			}
			if count > 0 {
				if first.Children[0] == second.Children[0] || first.Children[0].Number == second.Children[0].Number {
					t.Fatal("decode calls alias")
				}
				*first.Children[0].Number = 99
				if *second.Children[0].Number != 0 || *first.Named[0].Number != 0 || *input.Children[0].Number != 0 {
					t.Fatal("mutation escaped owner")
				}
			}
			retained = append(retained, &first, &second)
			// 前面的数组已恢复后才碰到坏尾部；公开Decode仍须保留调用方原对象。
			sentinel := &structSlotChild{}
			original := structSlotDocument{Pad: 99, Children: []*structSlotChild{sentinel}, Tail: "original"}
			dst := original
			used, err := sproto.Decode(wire[:len(wire)-1], &dst)
			if used != 0 || err == nil || dst.Pad != 99 || dst.Tail != "original" || len(dst.Children) != 1 || dst.Children[0] != sentinel {
				t.Fatal("failure mutated original")
			}
			runtime.GC()
		})
	}
	runtime.GC()
	for _, v := range retained {
		for i, child := range v.Named {
			if *child.Number != int64(i)*(-1<<33) || *child.Text != "child-"+strconv.Itoa(i) {
				t.Fatal("GC damaged retained child")
			}
		}
	}
	runtime.KeepAlive(retained)
}

// TestStructSliceDecodeDescriptorFallback 保护公开字段形状变化时仍执行原反射路径语义，并保留元素预算错误。
func TestStructSliceDecodeDescriptorFallback(t *testing.T) {
	number, text := int64(7), "fallback"
	input := &structSlotDocument{Children: []*structSlotChild{{Number: &number, Text: &text}}, Tail: "kept"}
	wire, err := sproto.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := sproto.GetSprotoType(reflect.TypeFor[structSlotDocument]())
	if err != nil {
		t.Fatal(err)
	}
	field := schema.FieldByTag(1)
	oldWire, oldArray := field.Wire, field.Array
	defer func() { field.Wire, field.Array = oldWire, oldArray }()
	for _, mode := range []string{"array", "wire"} {
		t.Run(mode, func(t *testing.T) {
			field.Wire, field.Array = oldWire, oldArray
			if mode == "array" {
				field.Array = false
			} else {
				field.Wire = sproto.WireVarintName
			}
			var output structSlotDocument
			used, err := sproto.Decode(wire, &output)
			if err != nil || used != len(wire) || !reflect.DeepEqual(&output, input) {
				t.Fatalf("fallback: %d %v", used, err)
			}
			_, err = sproto.DecodeWithLimits(wire, &output, sproto.Limits{MaxElements: 1})
			if !errors.Is(err, sproto.ErrLimit) {
				t.Fatalf("budget: %v", err)
			}
		})
	}
}

// TestStructSliceDecodeForeignDescriptor 保护异构描述不能静默向原切片写入不兼容指针。
func TestStructSliceDecodeForeignDescriptor(t *testing.T) {
	type foreignChild struct {
		Gap    [64]byte
		Number *int64  `sproto:"integer,0"`
		Text   *string `sproto:"string,1"`
	}
	type foreignDocument struct {
		Pad      int64           `sproto:"integer,0"`
		Children []*foreignChild `sproto:"struct,1,array"`
	}
	number, text := int64(7), "foreign"
	input := &structSlotDocument{Children: []*structSlotChild{{Number: &number, Text: &text}}, Tail: "kept"}
	wire, err := sproto.Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := sproto.GetSprotoType(reflect.TypeFor[structSlotDocument]())
	if err != nil {
		t.Fatal(err)
	}
	other, err := sproto.GetSprotoType(reflect.TypeFor[foreignDocument]())
	if err != nil {
		t.Fatal(err)
	}
	original := schema.Fields[1]
	schema.Fields[1] = other.FieldByTag(1)
	defer func() { schema.Fields[1] = original }()
	output := structSlotDocument{Tail: "original"}
	var decodeErr error
	panicked := func() (panicked bool) {
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		_, decodeErr = sproto.Decode(wire, &output)
		return false
	}()
	// 原反射路径拒绝不兼容赋值；不固定panic文本或将拒绝形式扩为公开API。
	if !panicked && decodeErr == nil {
		t.Fatal("incompatible element pointer silently accepted")
	}
	if output.Tail != "original" || output.Children != nil {
		t.Fatal("incompatible descriptor changed original")
	}
}
