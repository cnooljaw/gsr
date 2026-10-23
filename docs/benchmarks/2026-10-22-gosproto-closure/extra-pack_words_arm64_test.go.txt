//go:build arm64 && !purego

package sproto

import (
	"bytes"
	"testing"
)

// TestPackWordsDifferential 保护普通mask压紧、FF停止、尾部和共享工作区的wire与容量。
func TestPackWordsDifferential(t *testing.T) {
	check := func(input []byte) {
		t.Helper()
		before := bytes.Clone(input)
		capacity := packedBufferSize(len(input))
		want := referencePackInto(make([]byte, capacity), input)
		got := Pack(input)
		if !bytes.Equal(got, want) || cap(got) != cap(want) || !bytes.Equal(input, before) {
			t.Fatal("public Pack mismatch", len(input))
		}
		workspace := make([]byte, capacity)
		start := capacity - len(input)
		copy(workspace[start:], input)
		shared := packInto(workspace, workspace[start:])
		if !bytes.Equal(shared, want) || cap(shared) != cap(want) {
			t.Fatal("shared workspace mismatch", len(input))
		}
		if len(input) >= 8 && len(input) <= 1<<20 {
			consumed := 0
			for consumed+8 <= len(input) {
				ff := true
				for _, v := range input[consumed : consumed+8] {
					if v == 0 {
						ff = false
					}
				}
				if ff {
					break
				}
				consumed += 8
			}
			output := make([]byte, capacity)
			written, used := packWordsFast(output, input)
			expected := referencePackInto(make([]byte, packedBufferSize(consumed)), input[:consumed])
			if used != consumed || written != len(expected) || !bytes.Equal(output[:written], expected) {
				t.Fatal("native prefix mismatch", len(input), written, used, consumed)
			}
		}
	}
	for mask := 0; mask < 256; mask++ {
		var word [8]byte
		for bit := range word {
			if mask&(1<<bit) != 0 {
				word[bit] = byte(0x81 + bit)
			}
		}
		for _, words := range []int{1, 16, 255, 256, 257} {
			for tail := 0; tail < 8; tail++ {
				input := append(bytes.Repeat(word[:], words), bytes.Repeat([]byte{0x8f}, tail)...)
				check(input)
			}
		}
	}
	for _, length := range []int{127, 128, 129, (1 << 20) - 1, 1 << 20, (1 << 20) + 1} {
		input := make([]byte, length)
		for i := range input {
			if i%8 != 0 {
				input[i] = 0x81
			}
		}
		check(input)
	}
	// 交替FF与至少16个普通word，覆盖消耗增长额度后继续原生写出的重叠窗口。
	for _, ffWords := range []int{1, 255, 256, 257} {
		for _, mask := range []byte{0, 1, 31, 127} {
			var word [8]byte
			for bit := range word {
				if mask&(1<<bit) != 0 {
					word[bit] = byte(0x81 + bit)
				}
			}
			for tail := 0; tail < 8; tail++ {
				input := bytes.Repeat([]byte{0x81}, ffWords*8)
				input = append(input, bytes.Repeat(word[:], 16)...)
				input = append(input, bytes.Repeat([]byte{0x82}, 8)...)
				input = append(input, bytes.Repeat(word[:], 17)...)
				input = append(input, bytes.Repeat([]byte{0x83}, tail)...)
				check(input)
			}
		}
	}
	state := uint32(0x13579bdf)
	for i := 0; i < 4096; i++ {
		state = state*1664525 + 1013904223
		input := make([]byte, int(state%4097))
		for j := range input {
			state = state*1664525 + 1013904223
			input[j] = byte(state >> 24)
			if j%3 == 0 {
				input[j] = 0
			}
		}
		check(input)
	}
}
