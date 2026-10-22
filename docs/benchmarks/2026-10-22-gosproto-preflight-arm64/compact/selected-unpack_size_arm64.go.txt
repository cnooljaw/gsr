//go:build arm64 && !purego

package sproto

import "math/bits"

// 预检只需要字节数；固定256B表在初始化后只读，不携带恢复用的置换索引。
var unpackSizeCounts = func() (counts [256]uint8) {
	for header := range counts {
		counts[header] = uint8(bits.OnesCount8(uint8(header)))
	}
	return counts
}()

// unpackSizeFast 限定叶函数单次扫描规模；更小、更大输入保留Go预检。
func unpackSizeFast(src []byte, maxBytes int) (int, error, bool) {
	if len(src) < 128 || len(src) > 1<<20 {
		return 0, nil, false
	}
	size, status := unpackedSizeARM64(src, maxBytes, &unpackSizeCounts)
	switch status {
	case 1:
		return 0, ErrUnpack, true
	case 2:
		return 0, ErrLimit, true
	default:
		return size, nil, true
	}
}

// unpackedSizeARM64 只读src与表，要求maxBytes>0；status为0成功、1截断、2预算不足。
//
//go:noescape
func unpackedSizeARM64(src []byte, maxBytes int, table *[256]uint8) (size int, status uint64)
