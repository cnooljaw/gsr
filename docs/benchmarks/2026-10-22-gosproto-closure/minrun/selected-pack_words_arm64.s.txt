//go:build arm64 && !purego

#include "textflag.h"

// 只处理完整普通word，FF及短尾交回Go；调用方保证输出增长额度和八字节余量。
TEXT ·packWordsARM64(SB), NOSPLIT, $0-72
	MOVD dst_base+0(FP), R4
	MOVD src_base+24(FP), R1
	MOVD src_len+32(FP), R2
	MOVD table+48(FP), R3
	MOVD $0x7f7f7f7f7f7f7f7f, R12
	MOVD $0x8080808080808080, R13
	MOVD $0x0102040810204080, R14
pack_words_loop:
	CMP $8, R2
	BLO pack_words_done
	MOVD (R1), R6
	AND R12, R6, R7
	ADD R12, R7
	ORR R6, R7
	AND R13, R7
	LSR $7, R7
	MUL R14, R7
	LSR $56, R7, R8
	CMP $255, R8
	BEQ pack_words_done
	ADD R8<<4, R3, R9
	MOVD 8(R9), R10
	FMOVD R6, F0
	FMOVD (R9), F1
	VTBL V1.B8, [V0.B16], V2.B8
	FMOVD F2, 1(R4)
	MOVB R8, (R4)
	ADD $1, R10
	ADD R10, R4
	ADD $8, R1
	SUB $8, R2
	B pack_words_loop
pack_words_done:
	MOVD dst_base+0(FP), R0
	SUB R0, R4
	MOVD R4, written+56(FP)
	MOVD src_len+32(FP), R0
	SUB R2, R0
	MOVD R0, consumed+64(FP)
	RET
