package prodgate

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type queuedFrame struct {
	data   []byte
	charge int
}

// clientConnection 独占一条 WebSocket，并只向 Agent 暴露编码后入队能力。
type clientConnection struct {
	connection  *websocket.Conn
	maxFrame    int
	maxQueue    int
	counters    *gateCounters
	wake        chan struct{}
	closedCh    chan struct{}
	closeOnce   sync.Once
	mu          sync.Mutex
	queue       []queuedFrame
	queuedBytes int
	closed      bool
	closeErr    error
}

func newClientConnection(connection *websocket.Conn, maxFrame, maxQueue int, counters *gateCounters) *clientConnection {
	return &clientConnection{
		connection: connection,
		maxFrame:   maxFrame,
		maxQueue:   maxQueue,
		counters:   counters,
		wake:       make(chan struct{}, 1),
		closedCh:   make(chan struct{}),
	}
}

// Send 将完整 Envelope 编码后放入有界出站队列，不直接写入 WebSocket。
func (client *clientConnection) Send(envelope *Envelope) error {
	if envelope == nil {
		return errors.New("prodgate: nil Envelope")
	}
	if len(envelope.GetPayload()) > client.maxFrame {
		client.counters.frameTooLarge.Add(1)
		return ErrFrameTooLarge
	}
	frame, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("prodgate: marshal Envelope: %w", err)
	}
	if len(frame) > client.maxFrame {
		client.counters.frameTooLarge.Add(1)
		return ErrFrameTooLarge
	}
	if len(frame) > client.maxQueue-queueEntryOverhead {
		client.counters.outboundQueueFull.Add(1)
		return ErrClientQueueFull
	}
	charge := len(frame) + queueEntryOverhead

	client.mu.Lock()
	if client.closed {
		client.mu.Unlock()
		return ErrClientSessionClosed
	}
	if charge > client.maxQueue-client.queuedBytes {
		client.mu.Unlock()
		client.counters.outboundQueueFull.Add(1)
		return ErrClientQueueFull
	}
	client.queue = append(client.queue, queuedFrame{data: frame, charge: charge})
	client.queuedBytes += charge
	client.mu.Unlock()

	select {
	case client.wake <- struct{}{}:
	default:
	}
	return nil
}

// Close 使当前客户端会话失效并关闭底层连接。
func (client *clientConnection) Close() error {
	return client.closeWith(websocket.CloseNormalClosure, "normal", time.Now().Add(closeFrameWait))
}

// writeLoop 是该连接唯一的 WebSocket 数据写入者。
func (client *clientConnection) writeLoop() {
	for {
		frame, ok := client.nextFrame()
		if !ok {
			return
		}
		if err := client.connection.SetWriteDeadline(time.Now().Add(defaultWriteWait)); err != nil {
			client.closeWith(websocket.CloseGoingAway, "going_away", time.Now().Add(closeFrameWait))
			return
		}
		if err := client.connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			client.closeWith(websocket.CloseGoingAway, "going_away", time.Now().Add(closeFrameWait))
			return
		}
	}
}

func (client *clientConnection) nextFrame() ([]byte, bool) {
	for {
		client.mu.Lock()
		if client.closed {
			client.mu.Unlock()
			return nil, false
		}
		if len(client.queue) > 0 {
			frame := client.queue[0]
			if len(client.queue) == 1 {
				client.queue = nil
			} else {
				client.queue[0] = queuedFrame{}
				client.queue = client.queue[1:]
			}
			client.queuedBytes -= frame.charge
			client.mu.Unlock()
			return frame.data, true
		}
		client.mu.Unlock()

		select {
		case <-client.wake:
		case <-client.closedCh:
			return nil, false
		}
	}
}

func (client *clientConnection) closeWith(code int, reason string, deadline time.Time) error {
	client.closeOnce.Do(func() {
		client.mu.Lock()
		client.closed = true
		client.queue = nil
		client.queuedBytes = 0
		close(client.closedCh)
		client.mu.Unlock()

		if deadline.After(time.Now()) {
			_ = client.connection.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason),
				deadline,
			)
		}
		client.mu.Lock()
		client.closeErr = client.connection.Close()
		client.mu.Unlock()
	})
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.closeErr
}
