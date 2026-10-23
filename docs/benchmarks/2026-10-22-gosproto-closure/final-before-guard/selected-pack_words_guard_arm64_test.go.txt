//go:build arm64 && !purego && (darwin || linux)

package sproto

import (
	"bytes"
	"fmt"
	"syscall"
	"testing"
)

// TestPackWordsGuardPages 保护原生压紧只读源及精确预留输出的边界，覆盖所有mask和短尾。
func TestPackWordsGuardPages(t *testing.T) {
	for mask := 0; mask < 256; mask++ {
		var word [8]byte
		for bit := range word {
			if mask&(1<<bit) != 0 {
				word[bit] = byte(0x81 + bit)
			}
		}
		for _, count := range []int{1, 16, 256} {
			for _, tail := range []int{0, 1, 7} {
				t.Run(fmt.Sprintf("%d/%d/%d", mask, count, tail), func(t *testing.T) {
					raw := append(bytes.Repeat(word[:], count), bytes.Repeat([]byte{0x9f}, tail)...)
					input, inputPage := guardedVectorBuffer(t, len(raw))
					copy(input, raw)
					if err := syscall.Mprotect(inputPage, syscall.PROT_READ); err != nil {
						t.Fatal(err)
					}
					output, outputPage := guardedVectorBuffer(t, packedBufferSize(len(input)))
					used := count * 8
					if mask == 255 {
						used = 0
					}
					want := referencePackInto(make([]byte, packedBufferSize(used)), input[:used])
					written, consumed := packWordsFast(output, input)
					if consumed != used || written != len(want) || !bytes.Equal(output[:written], want) || !bytes.Equal(input, raw) {
						t.Fatal("native output mismatch")
					}
					for _, value := range outputPage[:len(outputPage)-len(output)] {
						if value != 0xa5 {
							t.Fatal("output prefix changed")
						}
					}
				})
			}
		}
	}
}
