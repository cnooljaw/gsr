//go:build arm64 && !purego
#include "textflag.h"
// 完整预检之后仅恢复字节，不分配、不保留指针。
TEXT ·unpackVectorARM64(SB), NOSPLIT, $0-56
	MOVD dst_base+0(FP), R0
	MOVD src_base+24(FP), R1
	MOVD src_len+32(FP), R2
	ADD R1, R2, R2
	MOVD table+48(FP), R3
loop:
	CMP R2, R1
	BEQ done
	MOVBU (R1), R4
	ADD $1, R1
	CBZ R4, zero
	CMP $255, R4
	BEQ ff
	ADD R4<<4, R3, R5
	MOVD 8(R5), R6
	SUB R1, R2, R8
	CMP $8, R8
	BLO tail
	FMOVD (R1), F0
	FMOVD (R5), F1
	VTBL V1.B8, [V0.B16], V2.B8
	FMOVD F2, (R0)
	ADD R6, R1
	ADD $8, R0
	B loop
zero:
	ADD $8, R0
zero_more:
	SUB R1, R2, R8
	CMP $8, R8
	BLO loop
	MOVD (R1), R13
	CBNZ R13, loop
	ADD $8, R1
	ADD $64, R0
	B zero_more
ff:
	MOVBU (R1), R6
	ADD $1, R1
	ADD $1, R6
	LSL $3, R6
	CMP $64, R6
	BLO ff_copy
ff_bulk:
	LDP (R1), (R7, R8)
	STP (R7, R8), (R0)
	LDP 16(R1), (R7, R8)
	STP (R7, R8), 16(R0)
	LDP 32(R1), (R7, R8)
	STP (R7, R8), 32(R0)
	LDP 48(R1), (R7, R8)
	STP (R7, R8), 48(R0)
	ADD $64, R1
	ADD $64, R0
	SUB $64, R6
	CMP $64, R6
	BHS ff_bulk
	CBZ R6, loop
ff_copy:
	MOVD (R1), R7
	MOVD R7, (R0)
	ADD $8, R1
	ADD $8, R0
	SUB $8, R6
	CBNZ R6, ff_copy
	B loop
tail:
	MOVD R4, R9
	MOVD R0, R12
tail_byte:
	TBZ $0, R9, tail_skip
	MOVBU (R1), R11
	ADD $1, R1
	MOVB R11, (R12)
tail_skip:
	ADD $1, R12
	LSR $1, R9
	CBNZ R9, tail_byte
	ADD $8, R0
	B loop
done:
	RET
