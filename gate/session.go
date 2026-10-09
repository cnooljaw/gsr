package gate

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// clientConnection 只向 Agent 暴露编码后入队能力，两种模式共用连接级诊断。
type clientConnection struct {
	connection           *websocket.Conn
	outbound             *outboundQueue
	connectionID         uint64
	counters             *gateCounters
	diagnosticMu         sync.Mutex
	diagnosticSeen       map[string]struct{}
	diagnosticSuppressed map[string]uint64
	diagnosticsFlushed   bool
}

func newClientConnection(connection *websocket.Conn, maxFrame, maxQueue int, counters *gateCounters, connectionID uint64) *clientConnection {
	return &clientConnection{
		connection:           connection,
		outbound:             newOutbound(connection, maxFrame, maxQueue),
		connectionID:         connectionID,
		counters:             counters,
		diagnosticSeen:       make(map[string]struct{}),
		diagnosticSuppressed: make(map[string]uint64),
	}
}

// noteDiagnostic 只允许同一连接同一类别的首条故障进入主日志，避免坏帧洪泛拖垮日志队列。
func (client *clientConnection) noteDiagnostic(category string) bool {
	client.diagnosticMu.Lock()
	defer client.diagnosticMu.Unlock()
	if client.diagnosticSeen == nil {
		client.diagnosticSeen = make(map[string]struct{})
	}
	if client.diagnosticSuppressed == nil {
		client.diagnosticSuppressed = make(map[string]uint64)
	}
	if _, seen := client.diagnosticSeen[category]; seen {
		client.diagnosticSuppressed[category]++
		return false
	}
	client.diagnosticSeen[category] = struct{}{}
	return true
}

func (client *clientConnection) takeSuppressedDiagnostics() map[string]uint64 {
	client.diagnosticMu.Lock()
	defer client.diagnosticMu.Unlock()
	if client.diagnosticsFlushed {
		return nil
	}
	client.diagnosticsFlushed = true
	suppressed := make(map[string]uint64, len(client.diagnosticSuppressed))
	for category, count := range client.diagnosticSuppressed {
		suppressed[category] = count
	}
	return suppressed
}

// Send 将完整 Envelope 编码后放入有界出站队列，不直接写入 WebSocket。
func (client *clientConnection) Send(envelope *Envelope) error {
	err := client.outbound.Send(envelope)
	switch {
	case errors.Is(err, errNilEnvelope):
		return errors.New("gate: nil Envelope")
	case errors.Is(err, errOutboundFrameTooLarge):
		client.counters.frameTooLarge.Add(1)
		return ErrFrameTooLarge
	case errors.Is(err, errOutboundQueueFull):
		client.counters.outboundQueueFull.Add(1)
		return ErrClientQueueFull
	case errors.Is(err, errOutboundClosed):
		return ErrClientSessionClosed
	case err != nil:
		return fmt.Errorf("gate: %w", err)
	default:
		return nil
	}
}

// Close 使当前客户端会话失效并关闭底层连接。
func (client *clientConnection) Close() error {
	return client.closeWith(websocket.CloseNormalClosure, "normal", time.Now().Add(closeFrameWait))
}

// writeLoop 是该连接唯一的 WebSocket 数据写入者，并返回终止写循环的错误。
func (client *clientConnection) writeLoop() error {
	return client.outbound.WriteLoop(defaultWriteWait, closeFrameWait)
}

func (client *clientConnection) closeWith(code int, reason string, deadline time.Time) error {
	return client.outbound.CloseWith(code, reason, deadline)
}
