package gate

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// queueEntryOverhead 为每条出站消息计入固定队列开销。
const queueEntryOverhead = 64

var (
	// errOutboundFrameTooLarge 表示消息超过单帧上限。
	errOutboundFrameTooLarge = errors.New("frame too large")
	// errOutboundQueueFull 表示出站队列无法接纳消息。
	errOutboundQueueFull = errors.New("client outbound queue full")
	// errOutboundClosed 表示连接已经关闭。
	errOutboundClosed = errors.New("client session closed")
)

type queuedFrame struct {
	data   []byte
	charge int
}

// outboundQueue 独占 WebSocket 的数据写入、有限队列和关闭过程。
type outboundQueue struct {
	connection    *websocket.Conn
	maxFrame      int
	maxQueue      int
	wake          chan struct{}
	closedCh      chan struct{}
	closeOnce     sync.Once
	mu            sync.Mutex
	queue         []queuedFrame
	queuedBytes   int
	closed        bool
	closeErr      error
	closing       bool
	drainDeadline time.Time
	drainErr      error
	drainTimer    *time.Timer
	drainDone     chan struct{}
	socketOnce    sync.Once
	socketErr     error
	closeFinished bool
	drainWritten  bool
}

// newOutbound 为一条 WebSocket 创建出站状态，调用方负责启动唯一写循环。
func newOutbound(connection *websocket.Conn, maxFrame, maxQueue int) *outboundQueue {
	return &outboundQueue{
		connection: connection,
		maxFrame:   maxFrame,
		maxQueue:   maxQueue,
		wake:       make(chan struct{}, 1),
		closedCh:   make(chan struct{}),
	}
}

// Send 按不透明字节与固定条目开销限制队列，接纳时复制快照，不解释消息格式。
func (outbound *outboundQueue) Send(frame []byte) error {
	outbound.mu.Lock()
	if outbound.closed || outbound.closing {
		outbound.mu.Unlock()
		return errOutboundClosed
	}
	if err := outbound.checkFrame(frame); err != nil {
		outbound.mu.Unlock()
		return err
	}
	charge := len(frame) + queueEntryOverhead
	outbound.queue = append(outbound.queue, queuedFrame{data: append([]byte(nil), frame...), charge: charge})
	outbound.queuedBytes += charge
	outbound.mu.Unlock()

	select {
	case outbound.wake <- struct{}{}:
	default:
	}
	return nil
}

// Done 在连接关闭时解除等待中的读循环。
func (outbound *outboundQueue) Done() <-chan struct{} {
	return outbound.closedCh
}

// WriteLoop 是连接唯一的数据写入者；出错时关闭连接并返回写入与关闭错误。
func (outbound *outboundQueue) WriteLoop(writeWait, closeWait time.Duration) (result error) {
	defer func() {
		outbound.mu.Lock()
		done := outbound.drainDone
		outbound.mu.Unlock()
		if done != nil {
			<-done
		}
		outbound.mu.Lock()
		result = errors.Join(result, outbound.drainErr)
		outbound.mu.Unlock()
	}()
	for {
		frame, ok := outbound.nextFrame()
		if !ok {
			outbound.mu.Lock()
			closing := outbound.closing
			outbound.mu.Unlock()
			if closing {
				return outbound.CloseWith(websocket.CloseNormalClosure, "normal", outbound.writeDeadline(closeWait))
			}
			return nil
		}
		if err := setWebSocketWriteDeadline(outbound.connection, outbound.writeDeadline(writeWait)); err != nil {
			writeErr := normalizeCloseError(err)
			if writeErr != nil {
				writeErr = fmt.Errorf("set WebSocket write deadline: %w", writeErr)
			}
			closeErr := outbound.CloseWith(websocket.CloseGoingAway, "going_away", outbound.writeDeadline(closeWait))
			return errors.Join(writeErr, closeErr)
		}
		if err := outbound.connection.WriteMessage(websocket.BinaryMessage, frame); err != nil {
			writeErr := normalizeCloseError(err)
			if writeErr != nil {
				writeErr = fmt.Errorf("write WebSocket message: %w", writeErr)
			}
			closeErr := outbound.CloseWith(websocket.CloseGoingAway, "going_away", outbound.writeDeadline(closeWait))
			return errors.Join(writeErr, closeErr)
		}
	}
}

func (outbound *outboundQueue) nextFrame() ([]byte, bool) {
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
		if outbound.closing {
			outbound.drainWritten = true
			outbound.mu.Unlock()
			return nil, false
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
func (outbound *outboundQueue) CloseWith(code int, reason string, deadline time.Time) error {
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
		if err := outbound.closeSocket(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close WebSocket connection: %w", err))
		}
		outbound.mu.Lock()
		outbound.closeErr = closeErr
		outbound.closeFinished = true
		if outbound.drainTimer != nil && outbound.drainTimer.Stop() {
			close(outbound.drainDone)
		}
		outbound.mu.Unlock()
	})
	outbound.mu.Lock()
	defer outbound.mu.Unlock()
	return outbound.closeErr
}

// setWebSocketWriteDeadline 同时检查 Gorilla 写路径可能忽略的底层 deadline 错误。
func setWebSocketWriteDeadline(connection *websocket.Conn, deadline time.Time) error {
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
