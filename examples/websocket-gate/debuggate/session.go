package debuggate

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

type queuedFrame struct {
	data   []byte
	charge int
}

// clientConnection 只向 Agent 暴露编码后入队能力，不暴露 WebSocket 连接。
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

// Send 编码完整 Envelope 并放入按字节和条目开销共同计费的有限出站队列。
func (client *clientConnection) Send(envelope *Envelope) error {
	if envelope == nil {
		return errors.New("debuggate: nil Envelope")
	}
	if len(envelope.GetPayload()) > client.maxFrame {
		client.counters.frameTooLarge.Add(1)
		return ErrFrameTooLarge
	}
	frame, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("debuggate: marshal Envelope: %w", err)
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

func (client *clientConnection) writeLoop() {
	for {
		frame, ok := client.nextFrame()
		if !ok {
			return
		}
		if err := setWebSocketWriteDeadline(client.connection, time.Now().Add(defaultWriteWait)); err != nil {
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

		var closeErr error
		if deadline.After(time.Now()) {
			if err := client.connection.UnderlyingConn().SetWriteDeadline(deadline); err != nil {
				if err := normalizeCloseError(err); err != nil {
					closeErr = fmt.Errorf("set WebSocket close write deadline: %w", err)
				}
			} else if err := normalizeCloseError(client.connection.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason),
				deadline,
			)); err != nil {
				closeErr = fmt.Errorf("write WebSocket close frame: %w", err)
			}
		}
		if err := normalizeCloseError(client.connection.Close()); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close WebSocket connection: %w", err))
		}
		client.mu.Lock()
		client.closeErr = closeErr
		client.mu.Unlock()
	})
	client.mu.Lock()
	defer client.mu.Unlock()
	return client.closeErr
}

// setWebSocketWriteDeadline 设置 Gorilla 后续数据写入的截止时间，并检查会被其写入路径忽略的底层错误。
func setWebSocketWriteDeadline(connection *websocket.Conn, deadline time.Time) error {
	connection.SetWriteDeadline(deadline)
	return connection.UnderlyingConn().SetWriteDeadline(deadline)
}

// normalizeCloseError 将对端已经断开时的底层关闭错误视为幂等收尾成功。
func normalizeCloseError(err error) error {
	if err == nil || errors.Is(err, net.ErrClosed) || errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ENOTCONN) {
		return nil
	}
	return err
}
