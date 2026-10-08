package cluster

import (
	"errors"
	"sync"
)

// badPayloadError 标记调用业务之前的载荷解码失败，不按扩展错误文本分类。
type badPayloadError struct{ error }

func (err *badPayloadError) Unwrap() error {
	return err.error
}

var errBadFramesSuppressed = errors.New("cluster: repeated bad frames suppressed")

// badFrameDiagnostics 归单连接所有，固定三类，不按不可信 Command 或文本建表。
// 入站 Call 解码可以并发；连接汇总只在所有处理任务退出后读取。
type badFrameDiagnostics struct {
	mu         sync.Mutex
	seen       [3]bool
	suppressed uint64
}

func (records *badFrameDiagnostics) accept(err error) bool {
	var category int
	var payload *badPayloadError
	switch {
	case errors.Is(err, ErrMalformedFrame):
		category = 0
	case errors.As(err, &payload):
		category = 1
	case errors.Is(err, errInvalidInboundRequest):
		category = 2
	default:
		return true
	}
	records.mu.Lock()
	defer records.mu.Unlock()
	if records.seen[category] {
		records.suppressed++
		return false
	}
	records.seen[category] = true
	return true
}
