package runtime

import "sync"

// messageKind 区分普通 Core Command 与受信任基础设施投递的客户端消息。
type messageKind uint8

const (
	messageKindCommand messageKind = iota
	messageKindClient
)

// Message 是通过 Service Mailbox 投递的 Runtime 内部消息容器，不是 wire frame。
type Message struct {
	Source  ServiceID
	Target  ServiceID
	Session SessionID
	Command CommandID
	Args    []any

	kind        messageKind // 当前内部消息类型。
	clientFrame []byte      // 客户端不透明消息的独占字节快照。
}

// mailbox 是带有独立唤醒信号的有界 FIFO 队列。
//
// 互斥锁同时保护接纳边界与队列内容。唤醒 channel 永不关闭，
// 避免在共享 channel 上发生发送与停止的竞争。
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
