package debuggate

import (
	"errors"
	"fmt"
	"time"

	"github.com/cnooljaw/gsr/gate/internal/gatecommon"
	"github.com/gorilla/websocket"
)

// clientConnection 只向 Agent 暴露编码后入队能力，不暴露 WebSocket 连接。
type clientConnection struct {
	connection *websocket.Conn
	outbound   *gatecommon.Outbound
	counters   *gateCounters
}

func newClientConnection(connection *websocket.Conn, maxFrame, maxQueue int, counters *gateCounters) *clientConnection {
	return &clientConnection{
		connection: connection,
		outbound:   gatecommon.NewOutbound(connection, maxFrame, maxQueue),
		counters:   counters,
	}
}

// Send 编码完整 Envelope 并放入按字节和条目开销共同计费的有限出站队列。
func (client *clientConnection) Send(envelope *Envelope) error {
	err := client.outbound.Send(envelope)
	switch {
	case errors.Is(err, gatecommon.ErrNilEnvelope):
		return errors.New("debuggate: nil Envelope")
	case errors.Is(err, gatecommon.ErrFrameTooLarge):
		client.counters.frameTooLarge.Add(1)
		return ErrFrameTooLarge
	case errors.Is(err, gatecommon.ErrQueueFull):
		client.counters.outboundQueueFull.Add(1)
		return ErrClientQueueFull
	case errors.Is(err, gatecommon.ErrClosed):
		return ErrClientSessionClosed
	case err != nil:
		return fmt.Errorf("debuggate: %w", err)
	default:
		return nil
	}
}

// Close 使当前客户端会话失效并关闭底层连接。
func (client *clientConnection) Close() error {
	return client.closeWith(websocket.CloseNormalClosure, "normal", time.Now().Add(closeFrameWait))
}

func (client *clientConnection) writeLoop() error {
	return client.outbound.WriteLoop(defaultWriteWait, closeFrameWait)
}

func (client *clientConnection) closeWith(code int, reason string, deadline time.Time) error {
	return client.outbound.CloseWith(code, reason, deadline)
}
