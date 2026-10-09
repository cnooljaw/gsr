package gate

import (
	"testing"

	"github.com/gorilla/websocket"
)

// TestAlreadySentCloseIsNotFailure 验证 WebSocket 已发出 Close 帧后再次收尾不算连接关闭失败。
func TestAlreadySentCloseIsNotFailure(t *testing.T) {
	if err := normalizeCloseError(websocket.ErrCloseSent); err != nil {
		t.Fatalf("normalizeCloseError(ErrCloseSent) = %v, want nil", err)
	}
}
