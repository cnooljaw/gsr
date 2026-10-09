package gate

import (
	"errors"

	"github.com/gorilla/websocket"
)

// safeWebSocketErrorFields 不把对端提供的 Close 原因文本写入 Gate 诊断。
func safeWebSocketErrorFields(err error) []any {
	if err == nil {
		return nil
	}
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) && closeErr != nil {
		return []any{"peer_close_code", closeErr.Code}
	}
	return []any{"error", err}
}
