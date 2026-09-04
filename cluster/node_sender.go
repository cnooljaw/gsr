package cluster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"

	runtime "github.com/cnooljaw/gsr/runtime"
)

const (
	defaultOutboundQueueCapacity = 1024
	defaultFailureQueueCapacity  = 256
)

// DeliveryFailure describes an infrastructure failure after local admission.
// It contains routing metadata only and deliberately excludes business payload.
type DeliveryFailure struct {
	LocalNode  NodeID
	RemoteNode NodeID
	Kind       FrameKind
	Session    ClusterSessionID
	Target     ServiceTarget
	Command    runtime.CommandID
	Err        error
}

// NodeSenderConfig configures one outbound sender for one remote NodeID.
type NodeSenderConfig struct {
	LocalNode     NodeID
	RemoteNode    NodeID
	Transport     Transport
	QueueCapacity int
}

var (
	// ErrInvalidNodeSenderConfig means a NodeSender configuration is invalid.
	ErrInvalidNodeSenderConfig = errors.New("cluster: invalid node sender config")
	// ErrNodeSenderClosed means a NodeSender no longer accepts new messages.
	ErrNodeSenderClosed = errors.New("cluster: node sender is closed")
	// ErrOutboundQueueFull means the bounded sender queue rejected admission.
	ErrOutboundQueueFull = errors.New("cluster: outbound queue is full")
	// ErrTransportPanicked means the transport panicked while sending an envelope.
	ErrTransportPanicked = errors.New("cluster: transport panicked")
)

// NodeSender is the single outbound owner for one local-to-remote node pair.
// It admits cloned envelopes into a bounded queue and delivers them serially.
type NodeSender struct {
	localNode       NodeID
	remoteNode      NodeID
	transport       Transport
	queue           chan ClusterEnvelope
	failures        chan DeliveryFailure
	stop            chan struct{}
	done            chan struct{}
	closeOnce       sync.Once
	mu              sync.Mutex
	closed          bool
	droppedFailures atomic.Uint64
}

// NewNodeSender constructs and starts one bounded outbound sender worker.
func NewNodeSender(config NodeSenderConfig) (*NodeSender, error) {
	if config.LocalNode == "" || config.RemoteNode == "" || isNilTransport(config.Transport) {
		return nil, fmt.Errorf("%w: local node, remote node, and Transport are required", ErrInvalidNodeSenderConfig)
	}
	if config.QueueCapacity == 0 {
		config.QueueCapacity = defaultOutboundQueueCapacity
	}
	if config.QueueCapacity < 1 {
		return nil, fmt.Errorf("%w: QueueCapacity must be greater than zero", ErrInvalidNodeSenderConfig)
	}
	sender := &NodeSender{
		localNode:  config.LocalNode,
		remoteNode: config.RemoteNode,
		transport:  config.Transport,
		queue:      make(chan ClusterEnvelope, config.QueueCapacity),
		failures:   make(chan DeliveryFailure, defaultFailureQueueCapacity),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	go sender.run()
	return sender, nil
}

// Send admits a cloned envelope into the bounded outbound queue.
// A nil error confirms local admission only. Later infrastructure failures are
// published through Failures without invoking business code.
func (sender *NodeSender) Send(envelope ClusterEnvelope) error {
	if sender == nil {
		return ErrNodeSenderClosed
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	ownedEnvelope := envelope.Clone()

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.closed {
		return ErrNodeSenderClosed
	}
	select {
	case sender.queue <- ownedEnvelope:
		return nil
	default:
		return ErrOutboundQueueFull
	}
}

// Failures returns the bounded stream of post-admission infrastructure
// failures. A slow consumer cannot block the sender; excess events are dropped.
func (sender *NodeSender) Failures() <-chan DeliveryFailure {
	if sender == nil {
		return nil
	}
	return sender.failures
}

// DroppedFailureCount returns the number of diagnostic events discarded
// because the bounded failure stream was full. A nil sender reports zero.
func (sender *NodeSender) DroppedFailureCount() uint64 {
	if sender == nil {
		return 0
	}
	return sender.droppedFailures.Load()
}

// Close stops admission, reports queued envelopes as failed, and waits for the
// current transport call until ctx expires. A canceled ctx does not cancel the
// worker; a later Close call may continue waiting for it.
func (sender *NodeSender) Close(ctx context.Context) error {
	if sender == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sender.closeOnce.Do(func() {
		sender.mu.Lock()
		sender.closed = true
		close(sender.stop)
		sender.mu.Unlock()
	})
	select {
	case <-sender.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (sender *NodeSender) run() {
	defer close(sender.done)
	defer close(sender.failures)
	for {
		select {
		case <-sender.stop:
			sender.failQueued()
			return
		case envelope := <-sender.queue:
			select {
			case <-sender.stop:
				sender.tryReportFailure(envelope, ErrNodeSenderClosed)
				sender.failQueued()
				return
			default:
			}
			if err := sender.sendTransport(envelope); err != nil {
				sender.tryReportFailure(envelope, err)
			}
		}
	}
}

func (sender *NodeSender) sendTransport(envelope ClusterEnvelope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: %v", ErrTransportPanicked, recovered)
		}
	}()
	return sender.transport.Send(sender.localNode, sender.remoteNode, envelope)
}

func (sender *NodeSender) tryReportFailure(envelope ClusterEnvelope, err error) {
	failure := DeliveryFailure{
		LocalNode:  sender.localNode,
		RemoteNode: sender.remoteNode,
		Kind:       envelope.Kind,
		Session:    envelope.Session,
		Target:     envelope.Target,
		Command:    envelope.Command,
		Err:        err,
	}
	select {
	case sender.failures <- failure:
	default:
		sender.droppedFailures.Add(1)
	}
}

func (sender *NodeSender) failQueued() {
	for {
		select {
		case envelope := <-sender.queue:
			sender.tryReportFailure(envelope, ErrNodeSenderClosed)
		default:
			return
		}
	}
}

func isNilTransport(transport Transport) bool {
	if transport == nil {
		return true
	}
	value := reflect.ValueOf(transport)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
