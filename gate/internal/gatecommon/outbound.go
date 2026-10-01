package gatecommon

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	gatecontract "github.com/cnooljaw/gsr/gate"
	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"
)

// QueueEntryOverhead 为每条出站消息计入固定队列开销。
const QueueEntryOverhead = 64

var (
	// ErrNilEnvelope 表示发送方没有提供完整消息。
	ErrNilEnvelope = errors.New("nil Envelope")
	// ErrFrameTooLarge 表示消息超过单帧上限。
	ErrFrameTooLarge = errors.New("frame too large")
	// ErrQueueFull 表示出站队列无法接纳消息。
	ErrQueueFull = errors.New("client outbound queue full")
	// ErrClosed 表示连接已经关闭。
	ErrClosed = errors.New("client session closed")
)

type queuedFrame struct {
	data   []byte
	charge int
}

// Outbound 独占 WebSocket 的数据写入、有限队列和关闭过程。
type Outbound struct {
	connection  *websocket.Conn
	maxFrame    int
	maxQueue    int
	wake        chan struct{}
	closedCh    chan struct{}
	closeOnce   sync.Once
	mu          sync.Mutex
	queue       []queuedFrame
	queuedBytes int
	closed      bool
	closeErr    error
}

// NewOutbound 为一条 WebSocket 创建出站状态，调用方负责启动唯一写循环。
func NewOutbound(connection *websocket.Conn, maxFrame, maxQueue int) *Outbound {
	return &Outbound{
		connection: connection,
		maxFrame:   maxFrame,
		maxQueue:   maxQueue,
		wake:       make(chan struct{}, 1),
		closedCh:   make(chan struct{}),
	}
}

// Send 编码完整 Envelope，并按帧字节与固定条目开销限制队列。
func (outbound *Outbound) Send(envelope *gatecontract.Envelope) error {
	if envelope == nil {
		return ErrNilEnvelope
	}
	if len(envelope.GetPayload()) > outbound.maxFrame {
		return ErrFrameTooLarge
	}
	frame, err := proto.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal Envelope: %w", err)
	}
	if len(frame) > outbound.maxFrame {
		return ErrFrameTooLarge
	}
	if len(frame) > outbound.maxQueue-QueueEntryOverhead {
		return ErrQueueFull
	}
	charge := len(frame) + QueueEntryOverhead

	outbound.mu.Lock()
	if outbound.closed {
		outbound.mu.Unlock()
		return ErrClosed
	}
	if charge > outbound.maxQueue-outbound.queuedBytes {
		outbound.mu.Unlock()
		return ErrQueueFull
	}
	outbound.queue = append(outbound.queue, queuedFrame{data: frame, charge: charge})
	outbound.queuedBytes += charge
	outbound.mu.Unlock()

	select {
	case outbound.wake <- struct{}{}:
	default:
	}
	return nil
}

// Done 在连接关闭时解除等待中的读循环。
func (outbound *Outbound) Done() <-chan struct{} {
	return outbound.closedCh
}

// WriteLoop 是连接唯一的数据写入者；出错时关闭连接并返回写入与关闭错误。
func (outbound *Outbound) WriteLoop(writeWait, closeWait time.Duration) error {
	for {
		frame, ok := outbound.nextFrame()
		if !ok {
			return nil
		}
		if err := SetWebSocketWriteDeadline(outbound.connection, time.Now().Add(writeWait)); err != nil {
			writeErr := normalizeCloseError(err)
			if writeErr != nil {
				writeErr = fmt.Errorf("set WebSocket write deadline: %w", writeErr)
			}
			closeErr := outbound.CloseWith(websocket.CloseGoingAway, "going_away", time.Now().Add(closeWait))
			return errors.Join(writeErr, closeErr)
		}
		if err := outbound.connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			writeErr := normalizeCloseError(err)
			if writeErr != nil {
				writeErr = fmt.Errorf("write WebSocket message: %w", writeErr)
			}
			closeErr := outbound.CloseWith(websocket.CloseGoingAway, "going_away", time.Now().Add(closeWait))
			return errors.Join(writeErr, closeErr)
		}
	}
}

func (outbound *Outbound) nextFrame() ([]byte, bool) {
	for {
		outbound.mu.Lock()
		if outbound.closed {
			outbound.mu.Unlock()
			return nil, false
		}
		if len(outbound.queue) > 0 {
			frame := outbound.queue[0]
			if len(outbound.queue) == 1 {
				outbound.queue = nil
			} else {
				outbound.queue[0] = queuedFrame{}
				outbound.queue = outbound.queue[1:]
			}
			outbound.queuedBytes -= frame.charge
			outbound.mu.Unlock()
			return frame.data, true
		}
		outbound.mu.Unlock()

		select {
		case <-outbound.wake:
		case <-outbound.closedCh:
			return nil, false
		}
	}
}

// CloseWith 关闭队列和 WebSocket；重复调用返回首次关闭的结果。
func (outbound *Outbound) CloseWith(code int, reason string, deadline time.Time) error {
	outbound.closeOnce.Do(func() {
		outbound.mu.Lock()
		outbound.closed = true
		outbound.queue = nil
		outbound.queuedBytes = 0
		close(outbound.closedCh)
		outbound.mu.Unlock()

		var closeErr error
		if deadline.After(time.Now()) {
			if err := outbound.connection.UnderlyingConn().SetWriteDeadline(deadline); err != nil {
				if err := normalizeCloseError(err); err != nil {
					closeErr = fmt.Errorf("set WebSocket close write deadline: %w", err)
				}
			} else if err := normalizeCloseError(outbound.connection.WriteControl(
				websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason),
				deadline,
			)); err != nil {
				closeErr = fmt.Errorf("write WebSocket close frame: %w", err)
			}
		}
		if err := normalizeCloseError(outbound.connection.Close()); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close WebSocket connection: %w", err))
		}
		outbound.mu.Lock()
		outbound.closeErr = closeErr
		outbound.mu.Unlock()
	})
	outbound.mu.Lock()
	defer outbound.mu.Unlock()
	return outbound.closeErr
}

// SetWebSocketWriteDeadline 同时检查 Gorilla 写路径可能忽略的底层 deadline 错误。
func SetWebSocketWriteDeadline(connection *websocket.Conn, deadline time.Time) error {
	connection.SetWriteDeadline(deadline)
	return connection.UnderlyingConn().SetWriteDeadline(deadline)
}

func normalizeCloseError(err error) error {
	if err == nil || errors.Is(err, websocket.ErrCloseSent) || errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) || errors.Is(err, io.ErrClosedPipe) || errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ENOTCONN) {
		return nil
	}
	return err
}
