package cluster

import (
	"errors"
	"fmt"
	"sync"
)

const defaultMaxPayload = 4 << 20

// Receiver handles one envelope delivered to a registered node. The envelope
// is a transport-owned value snapshot and may be modified by the receiver.
type Receiver func(from NodeID, envelope ClusterEnvelope)

// Transport moves ClusterEnvelope values between nodes. It deliberately does
// not expose Core Service, Mailbox, Session or connection implementation details.
type Transport interface {
	Register(NodeID, Receiver) error
	Unregister(NodeID) error
	Send(from NodeID, to NodeID, envelope ClusterEnvelope) error
	Close() error
}

// InMemoryTransportConfig configures the deterministic in-process transport.
type InMemoryTransportConfig struct {
	// MaxPayload bounds the envelope payload accepted by Send. Zero uses the default.
	MaxPayload int
}

func (config InMemoryTransportConfig) normalized() (InMemoryTransportConfig, error) {
	if config.MaxPayload == 0 {
		config.MaxPayload = defaultMaxPayload
	}
	if config.MaxPayload < 0 {
		return InMemoryTransportConfig{}, fmt.Errorf("%w: max payload is negative", ErrInvalidTransportConfig)
	}
	return config, nil
}

var (
	// ErrInvalidTransportConfig means an in-memory transport configuration is invalid.
	ErrInvalidTransportConfig = errors.New("cluster: invalid transport config")
	// ErrInvalidNode means a node ID or receiver is invalid.
	ErrInvalidNode = errors.New("cluster: invalid node")
	// ErrDuplicateNode means a node already has a receiver in this transport.
	ErrDuplicateNode = errors.New("cluster: duplicate node")
	// ErrNodeNotFound means no receiver is registered for a destination node.
	ErrNodeNotFound = errors.New("cluster: node not found")
	// ErrTransportClosed means the transport has stopped accepting operations.
	ErrTransportClosed = errors.New("cluster: transport is closed")
)

// InMemoryTransport synchronously delivers cloned envelopes between registered
// nodes. It is a deterministic test seam, not a production network transport.
type InMemoryTransport struct {
	mu         sync.RWMutex
	maxPayload int
	closed     bool
	receivers  map[NodeID]Receiver
}

// NewInMemoryTransport constructs a deterministic in-process transport.
func NewInMemoryTransport(config InMemoryTransportConfig) (*InMemoryTransport, error) {
	normalized, err := config.normalized()
	if err != nil {
		return nil, err
	}
	return &InMemoryTransport{
		maxPayload: normalized.MaxPayload,
		receivers:  make(map[NodeID]Receiver),
	}, nil
}

// Register attaches one receiver to a local NodeID.
func (transport *InMemoryTransport) Register(node NodeID, receiver Receiver) error {
	if transport == nil || node == "" || receiver == nil {
		return ErrInvalidNode
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.closed {
		return ErrTransportClosed
	}
	if _, exists := transport.receivers[node]; exists {
		return ErrDuplicateNode
	}
	transport.receivers[node] = receiver
	return nil
}

// Unregister removes a node receiver. It is idempotent for an existing open transport.
func (transport *InMemoryTransport) Unregister(node NodeID) error {
	if transport == nil || node == "" {
		return ErrInvalidNode
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if transport.closed {
		return ErrTransportClosed
	}
	delete(transport.receivers, node)
	return nil
}

// Send validates and synchronously delivers a cloned envelope to the destination node.
func (transport *InMemoryTransport) Send(from NodeID, to NodeID, envelope ClusterEnvelope) error {
	if transport == nil {
		return ErrTransportClosed
	}
	if from == "" || to == "" {
		return ErrInvalidNode
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	if len(envelope.Payload) > transport.maxPayload {
		return fmt.Errorf("%w: got %d, limit %d", ErrPayloadTooLarge, len(envelope.Payload), transport.maxPayload)
	}

	transport.mu.RLock()
	if transport.closed {
		transport.mu.RUnlock()
		return ErrTransportClosed
	}
	receiver, exists := transport.receivers[to]
	transport.mu.RUnlock()
	if !exists {
		return ErrNodeNotFound
	}
	receiver(from, envelope.Clone())
	return nil
}

// Close prevents future registration and delivery, and is idempotent.
func (transport *InMemoryTransport) Close() error {
	if transport == nil {
		return nil
	}
	transport.mu.Lock()
	transport.closed = true
	transport.receivers = nil
	transport.mu.Unlock()
	return nil
}
