//go:build arm64 && !purego

package sproto

import "math/bits"

// referenceUnpackSize 保留引入原生扫描前的完整Go预检，作为边界与错误顺序的独立参考。
func referenceUnpackSize(src []byte, maxBytes int) (int, error) {
	if maxBytes < 1 {
		return 0, ErrInvalidLimits
	}
	// 首次只扫描 tag/连续块长度，在分配前确认完整边界及有限的确切输出长度。
	size := 0
	for remaining := src; len(remaining) > 0; {
		header := remaining[0]
		consumed, produced := 1, 8
		if header == 0 {
			if len(remaining) >= 2 && remaining[1] == 0 {
				words := (maxBytes - size) / 8
				if words < 2 {
					return 0, ErrLimit
				}
				// 多检查一个零 mask 即可判定超额，不为小预算扫描整段大输入。
				end := min(len(remaining), words+1)
				run := 2 + zeroHeaderPrefix(remaining[2:end])
				if run > words {
					return 0, ErrLimit
				}
				consumed, produced = run, run*8
			}
		} else if header == 0xff {
			if len(remaining) < 2 {
				return 0, ErrUnpack
			}
			produced = (int(remaining[1]) + 1) * 8
			consumed = 2 + produced
		} else {
			consumed += bits.OnesCount8(header)
		}
		if consumed > len(remaining) {
			return 0, ErrUnpack
		}
		if produced > maxBytes-size {
			return 0, ErrLimit
		}
		size += produced
		remaining = remaining[consumed:]
	}
	return size, nil
}
