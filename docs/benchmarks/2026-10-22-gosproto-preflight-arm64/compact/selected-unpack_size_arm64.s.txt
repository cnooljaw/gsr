//go:build arm64 && !purego

#include "textflag.h"

// 只读预检；R0源游标、R1剩余输入、R2剩余预算、R3固定表。
TEXT ·unpackedSizeARM64(SB), NOSPLIT, $0-56
	MOVD src_base+0(FP), R0
	MOVD src_len+8(FP), R1
	MOVD maxBytes+24(FP), R2
	MOVD table+32(FP), R3
size_loop:
	CBZ R1, size_done
	MOVBU (R0), R5
	CBZ R5, size_zero
	CMP $255, R5
	BEQ size_ff
	MOVBU (R3)(R5), R6
	ADD $1, R6
	CMP R6, R1
	BLO size_bad
	CMP $8, R2
	BLO size_limit
	SUB $8, R2
size_advance:
	SUB R6, R1
	ADD R6, R0
	B size_loop
size_ff:
	CMP $2, R1
	BLO size_bad
	MOVBU 1(R0), R7
	ADD $1, R7
	LSL $3, R7
	ADD $2, R7, R6
	CMP R6, R1
	BLO size_bad
	CMP R7, R2
	BLO size_limit
	SUB R7, R2
	B size_advance
size_zero:
	CMP $8, R2
	BLO size_limit
	CMP $8, R1
	BLO size_zero_one
	MOVD (R0), R8
	CBNZ R8, size_zero_one
	CMP $64, R2
	BLO size_zero_one
	SUB $64, R2
	SUB $8, R1
	ADD $8, R0
	B size_loop
size_zero_one:
	SUB $8, R2
	SUB $1, R1
	ADD $1, R0
	B size_loop
size_done:
	MOVD maxBytes+24(FP), R4
	SUB R2, R4
	MOVD R4, size+40(FP)
	MOVD ZR, status+48(FP)
	RET
size_bad:
	MOVD $1, R4
	B size_error
size_limit:
	MOVD $2, R4
size_error:
	MOVD ZR, size+40(FP)
	MOVD R4, status+48(FP)
	RET
