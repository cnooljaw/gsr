package gate

import (
	"errors"
	"time"

	"github.com/gorilla/websocket"
)

// ErrSendAndCloseTimeout 仅用于连接诊断，表示发送后关闭的总预算已耗尽。
var ErrSendAndCloseTimeout = errors.New("gate: send and close timed out")

// checkFrame 在持有队列锁时校验包括固定条目开销在内的容量。
func (outbound *outboundQueue) checkFrame(frame []byte) error {
	if len(frame) > outbound.maxFrame {
		return errOutboundFrameTooLarge
	}
	if len(frame) > outbound.maxQueue-queueEntryOverhead || len(frame)+queueEntryOverhead > outbound.maxQueue-outbound.queuedBytes {
		return errOutboundQueueFull
	}
	return nil
}

func (outbound *outboundQueue) sendAndClose(frame []byte, budget time.Duration) error {
	outbound.mu.Lock()
	if outbound.closed || outbound.closing {
		outbound.mu.Unlock()
		return errOutboundClosed
	}
	err := outbound.checkFrame(frame)
	outbound.closing = true
	outbound.drainDeadline = time.Now().Add(budget)
	outbound.drainDone = make(chan struct{})
	if err == nil {
		charge := len(frame) + queueEntryOverhead
		outbound.queue = append(outbound.queue, queuedFrame{data: append([]byte(nil), frame...), charge: charge})
		outbound.queuedBytes += charge
	} else {
		outbound.drainErr = err
		// 接纳失败不等待旧队列，异步中止连接；仍由 writer 等待 timer 的真实退出。
		budget = 0
	}
	if err == nil {
		budget = time.Until(outbound.drainDeadline)
	}
	outbound.drainTimer = time.AfterFunc(budget, func() {
		defer close(outbound.drainDone)
		outbound.mu.Lock()
		if !outbound.closeFinished && outbound.drainErr == nil {
			outbound.drainErr = ErrSendAndCloseTimeout
		}
		outbound.mu.Unlock()
		// 不能等待唯一 writer 获取写锁再中断阻塞写；直接关闭 socket 解除 I/O。
		socketErr := outbound.closeSocket()
		closeErr := outbound.CloseWith(websocket.CloseGoingAway, "going_away", time.Time{})
		outbound.mu.Lock()
		outbound.drainErr = errors.Join(outbound.drainErr, socketErr, closeErr)
		outbound.mu.Unlock()
	})
	outbound.mu.Unlock()
	select {
	case outbound.wake <- struct{}{}:
	default:
	}
	return err
}

func (outbound *outboundQueue) writeDeadline(wait time.Duration) time.Time {
	deadline := time.Now().Add(wait)
	outbound.mu.Lock()
	if outbound.closing && outbound.drainDeadline.Before(deadline) {
		deadline = outbound.drainDeadline
	}
	outbound.mu.Unlock()
	return deadline
}

func (outbound *outboundQueue) isClosing() bool {
	outbound.mu.Lock()
	defer outbound.mu.Unlock()
	return outbound.closed || outbound.closing
}

// admitInbound 与末帧接纳使用同一锁，回调只做有界 Runtime admission，不执行 Handler。
func (outbound *outboundQueue) admitInbound(admit func() error) error {
	outbound.mu.Lock()
	defer outbound.mu.Unlock()
	if outbound.closed || outbound.closing {
		return ErrClientSessionClosed
	}
	return admit()
}

func (outbound *outboundQueue) closeSocket() error {
	outbound.socketOnce.Do(func() { outbound.socketErr = normalizeCloseError(outbound.connection.Close()) })
	return outbound.socketErr
}
