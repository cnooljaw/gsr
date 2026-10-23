package benchmarks

import (
	"bytes"
	"fmt"
	"testing"

	sproto "github.com/cnooljaw/gosproto"
)

type unpackShape struct {
	name string
	raw  []byte
}

func unpackShapes() []unpackShape {
	var shapes []unpackShape
	for _, size := range []int{32, 4096, 65536} {
		shapes = append(shapes,
			unpackShape{fmt.Sprintf("zero%d", size), make([]byte, size)},
			unpackShape{fmt.Sprintf("ascii%d", size), bytes.Repeat([]byte{0x7f}, size)},
		)
	}
	for count := 1; count <= 7; count++ {
		raw := make([]byte, 2048+3)[3:]
		for word := 0; word < 256; word++ {
			for index := 0; index < 8; index++ {
				if (index+word)%8 < count {
					raw[word*8+index] = 0x80 + byte(index)
				}
			}
		}
		shapes = append(shapes, unpackShape{fmt.Sprintf("nonzero%dx256", count), raw})
	}
	sparse := make([]byte, 4096)
	for index := 0; index < len(sparse); index += 4 {
		sparse[index] = 0x80
	}
	shapes = append(shapes, unpackShape{"FF512-sparse4096", append(bytes.Repeat([]byte{0x7f}, 512), sparse...)})
	for _, words := range []int{8, 16, 64} {
		block := append(make([]byte, words*8), bytes.Repeat([]byte{0x7f}, 8)...)
		shapes = append(shapes, unpackShape{fmt.Sprintf("zeroRun%dx64", words), bytes.Repeat(block, 64)})
	}
	random := make([]byte, 4096)
	state := uint32(12)
	for index := range random {
		state = state*1664525 + 1013904223
		random[index] = byte(state >> 24)
	}
	return append(shapes, unpackShape{"random4096", random})
}

// BenchmarkUnpackShapes 检查普通 word、零段和 FF/稀疏混合输入，防止仅大消息受益。
func BenchmarkUnpackShapes(b *testing.B) {
	for _, shape := range unpackShapes() {
		packed := sproto.Pack(shape.raw)
		output, err := sproto.Unpack(packed)
		if err != nil || !bytes.Equal(output, shape.raw) {
			b.Fatalf("%s unpack differs: %v", shape.name, err)
		}
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(shape.raw)))
			b.ResetTimer()
			for range b.N {
				output, err := sproto.Unpack(packed)
				if err != nil {
					b.Fatal(err)
				}
				encodedSink = output
			}
		})
	}
}

// BenchmarkPackShapes 分离压缩阶段，复用零段、FF、稀疏及高密度输入观察吞吐与分配。
func BenchmarkPackShapes(b *testing.B) {
	for _, shape := range unpackShapes() {
		packed := sproto.Pack(shape.raw)
		unpacked, err := sproto.Unpack(packed)
		if err != nil || !bytes.Equal(unpacked, shape.raw) {
			b.Fatalf("%s pack round trip differs: %v", shape.name, err)
		}
		b.Run(shape.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(shape.raw)))
			b.ResetTimer()
			for range b.N {
				encodedSink = sproto.Pack(shape.raw)
			}
		})
	}
}
