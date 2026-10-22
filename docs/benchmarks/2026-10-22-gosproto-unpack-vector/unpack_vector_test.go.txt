package sproto

import (
	"bytes"
	"errors"
	"testing"
)

// 混合普通mask、零段和宽FF段必须逐字节恢复，预算和坏尾部失败不得暴露部分输出。
func TestUnpackVectorMixedWire(t *testing.T) {
	seed := uint64(0x13579bdf)
	next := func() byte { seed = seed*6364136223846793005 + 1; return byte(seed >> 56) }
	for sample := 0; sample < 4096; sample++ {
		var packed, want []byte
		groups := 1 + int(next()%48)
		for group := 0; group < groups; group++ {
			switch group % 4 {
			case 0:
				packed = append(packed, 0)
				want = append(want, make([]byte, 8)...)
			case 1:
				count := int(next() % 33)
				packed = append(packed, 255, byte(count))
				for i := 0; i < (count+1)*8; i++ {
					value := next()
					packed = append(packed, value)
					want = append(want, value)
				}
			default:
				header := byte(1 + int(next())%254)
				packed = append(packed, header)
				for position := 0; position < 8; position++ {
					value := byte(0)
					if header&(1<<position) != 0 {
						value = next()
						packed = append(packed, value)
					}
					want = append(want, value)
				}
			}
		}
		before := append([]byte(nil), packed...)
		got, err := UnpackWithLimit(packed, len(want))
		if err != nil || !bytes.Equal(got, want) || cap(got) != len(want) || !bytes.Equal(packed, before) {
			t.Fatalf("sample %d: %v", sample, err)
		}
		if got, err := UnpackWithLimit(packed, len(want)-1); got != nil || !errors.Is(err, ErrLimit) {
			t.Fatalf("budget sample %d: %v", sample, err)
		}
		broken := append(append([]byte(nil), packed...), 255)
		if got, err := UnpackWithLimit(broken, len(want)+8); got != nil || !errors.Is(err, ErrUnpack) {
			t.Fatalf("tail sample %d: %v", sample, err)
		}
	}
}
