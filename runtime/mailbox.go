package runtime

import "sync"

// Message is the unencoded local value delivered through a Service mailbox.
type Message struct {
	Source  ServiceID
	Target  ServiceID
	Session SessionID
	Command CommandID
	Args    []any
}

// mailbox is a bounded FIFO queue with a separate wake-up signal.
//
// A mutex protects both the admission boundary and queue contents. The wake
// channel is never closed, which avoids a send/stop race on a shared channel.
type mailbox struct {
	mu        sync.Mutex
	capacity  int
	queue     []Message
	accepting bool
	wake      chan struct{}
}

func newMailbox(capacity int) *mailbox {
	return &mailbox{
		capacity:  capacity,
		accepting: true,
		wake:      make(chan struct{}, 1),
	}
}

func (mailbox *mailbox) enqueue(message Message) error {
	mailbox.mu.Lock()
	if !mailbox.accepting {
		mailbox.mu.Unlock()
		return ErrServiceStopping
	}
	if len(mailbox.queue) >= mailbox.capacity {
		mailbox.mu.Unlock()
		return ErrMailboxFull
	}
	mailbox.queue = append(mailbox.queue, message)
	mailbox.mu.Unlock()

	mailbox.signal()
	return nil
}

func (mailbox *mailbox) dequeue() (Message, bool) {
	for {
		mailbox.mu.Lock()
		if len(mailbox.queue) > 0 {
			message := mailbox.queue[0]
			mailbox.queue[0] = Message{}
			mailbox.queue = mailbox.queue[1:]
			mailbox.mu.Unlock()
			return message, true
		}
		if !mailbox.accepting {
			mailbox.mu.Unlock()
			return Message{}, false
		}
		mailbox.mu.Unlock()

		<-mailbox.wake
	}
}

func (mailbox *mailbox) stop() {
	mailbox.mu.Lock()
	mailbox.accepting = false
	mailbox.queue = nil
	mailbox.mu.Unlock()
	mailbox.signal()
}

func (mailbox *mailbox) snapshot() (depth, capacity int) {
	mailbox.mu.Lock()
	defer mailbox.mu.Unlock()
	return len(mailbox.queue), mailbox.capacity
}

func (mailbox *mailbox) signal() {
	select {
	case mailbox.wake <- struct{}{}:
	default:
	}
}
