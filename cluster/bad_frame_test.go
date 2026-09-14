package cluster

import (
	"errors"
	"fmt"
	"testing"
)

// 验证同连接按稳定类别保留首条，不因错误文本或 Command 变化扩大去重状态。
func TestBadFrameFirstOnly(t *testing.T) {
	var records badFrameDiagnostics
	for _, err := range []error{ErrMalformedFrame, ErrCommandCodecNotFound, &badPayloadError{errors.New("decode")}, ErrInvalidConnectionAgentRequest} {
		if !records.accept(err) || records.accept(fmt.Errorf("different detail: %w", err)) {
			t.Fatalf("first-only failed: %v", err)
		}
	}
	if records.suppressed != 4 {
		t.Fatalf("suppressed=%d", records.suppressed)
	}
	for i := 0; i < 2; i++ {
		if !records.accept(ErrConnectionAgentWrite) {
			t.Fatal("write failure suppressed")
		}
	}
	var other badFrameDiagnostics
	if !other.accept(ErrMalformedFrame) {
		t.Fatal("new connection lost first record")
	}
}
