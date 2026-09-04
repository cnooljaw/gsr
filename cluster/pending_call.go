package cluster

import (
	"errors"
	"fmt"
	"sync"

	runtime "github.com/cnooljaw/gsr/runtime"
)

var (
	// ErrClusterCallClosed means the Cluster pending-call owner has stopped.
	ErrClusterCallClosed = errors.New("cluster: pending calls are closed")
	// ErrClusterSessionNotFound means a response does not match a pending call.
	ErrClusterSessionNotFound = errors.New("cluster: session not found")
	// ErrClusterResponseMismatch means a response does not match its request route.
	ErrClusterResponseMismatch = errors.New("cluster: response does not match request")
	// ErrInvalidClusterCall means a pending-call registration is invalid.
	ErrInvalidClusterCall = errors.New("cluster: invalid call")
	// ErrInvalidClusterResponse means a completion frame is not a valid response.
	ErrInvalidClusterResponse = errors.New("cluster: invalid response")
)

type clusterCallResult struct {
	payload []byte
	err     error
}

// ClusterCallRegistry is the Cluster-owned completion boundary shared by all
// source-bound ClusterClient instances in one Cluster.
//
// It keeps session allocation and pending-call ownership out of individual
// Service clients. A transport or ConnectionAgent completes a response with
// Complete; business code does not access pending-call records directly. Use
// NewClusterCallRegistry; the zero value is not an open registry.
type ClusterCallRegistry struct {
	pending *clusterPendingCalls
}

var (
	// ErrInvalidClusterCallRegistry means a call registry receiver is invalid.
	ErrInvalidClusterCallRegistry = errors.New("cluster: invalid call registry")
)

// NewClusterCallRegistry creates an open Cluster-wide pending-call owner.
func NewClusterCallRegistry() *ClusterCallRegistry {
	return &ClusterCallRegistry{pending: newClusterPendingCalls()}
}

func (registry *ClusterCallRegistry) valid() bool {
	return registry != nil && registry.pending != nil
}

// Complete matches one remote Response or Error to its pending call.
//
// The from NodeID is supplied by the transport rather than read from the
// envelope. A mismatch or unknown session is rejected without completing any
// other call.
func (registry *ClusterCallRegistry) Complete(from NodeID, envelope ClusterEnvelope) error {
	if !registry.valid() {
		return ErrInvalidClusterCallRegistry
	}
	return registry.pending.complete(from, envelope)
}

// Close completes all remaining calls with ErrClusterCallClosed and prevents
// new sessions from being registered.
func (registry *ClusterCallRegistry) Close() error {
	if !registry.valid() {
		return ErrInvalidClusterCallRegistry
	}
	return registry.pending.close(nil)
}

// PendingCount reports the number of calls that are still waiting for a
// response. It is intended for lifecycle and diagnostic checks.
func (registry *ClusterCallRegistry) PendingCount() int {
	if !registry.valid() {
		return 0
	}
	return registry.pending.pendingCount()
}

func (registry *ClusterCallRegistry) begin(source runtime.ServiceID, node NodeID, target ServiceTarget, command runtime.CommandID) (*clusterPendingCall, error) {
	if !registry.valid() {
		return nil, ErrInvalidClusterCallRegistry
	}
	return registry.pending.begin(source, node, target, command)
}

func (registry *ClusterCallRegistry) cancel(session ClusterSessionID, err error) bool {
	if !registry.valid() {
		return false
	}
	return registry.pending.cancel(session, err)
}

type clusterPendingCall struct {
	session ClusterSessionID
	source  runtime.ServiceID
	node    NodeID
	target  ServiceTarget
	command runtime.CommandID
	done    chan clusterCallResult
}

// clusterPendingCalls owns ClusterSessionID allocation and response completion
// for one Cluster instance. It does not perform network I/O or wake Runtime
// runners directly; the caller owns that integration.
type clusterPendingCalls struct {
	mu      sync.Mutex
	next    ClusterSessionID
	pending map[ClusterSessionID]*clusterPendingCall
	closed  bool
}

func newClusterPendingCalls() *clusterPendingCalls {
	return &clusterPendingCalls{
		next:    1,
		pending: make(map[ClusterSessionID]*clusterPendingCall),
	}
}

func (registry *clusterPendingCalls) begin(source runtime.ServiceID, node NodeID, target ServiceTarget, command runtime.CommandID) (*clusterPendingCall, error) {
	if registry == nil || source == 0 || node == "" || target.validate() != nil || command == 0 {
		return nil, fmt.Errorf("%w: source, node, target, and command are required", ErrInvalidClusterCall)
	}

	registry.mu.Lock()
	defer registry.mu.Unlock()
	if registry.closed {
		return nil, ErrClusterCallClosed
	}
	if registry.next == 0 {
		return nil, runtime.ErrIDExhausted
	}
	session := registry.next
	if session == ^ClusterSessionID(0) {
		registry.next = 0
	} else {
		registry.next++
	}
	pending := &clusterPendingCall{
		session: session,
		source:  source,
		node:    node,
		target:  target,
		command: command,
		done:    make(chan clusterCallResult, 1),
	}
	registry.pending[session] = pending
	return pending, nil
}

func (registry *clusterPendingCalls) complete(from NodeID, envelope ClusterEnvelope) error {
	if registry == nil {
		return ErrClusterSessionNotFound
	}
	if from == "" {
		return fmt.Errorf("%w: source node is empty", ErrInvalidClusterResponse)
	}
	if envelope.Kind != FrameResponse && envelope.Kind != FrameError {
		return fmt.Errorf("%w: frame kind %s cannot complete a call", ErrInvalidClusterResponse, envelope.Kind)
	}
	if err := envelope.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidClusterResponse, err)
	}

	registry.mu.Lock()
	pending, exists := registry.pending[envelope.Session]
	if !exists {
		registry.mu.Unlock()
		return fmt.Errorf("%w: session %d", ErrClusterSessionNotFound, envelope.Session)
	}
	if pending.node != from || pending.target != envelope.Target || pending.command != envelope.Command {
		registry.mu.Unlock()
		return fmt.Errorf("%w: session %d", ErrClusterResponseMismatch, envelope.Session)
	}
	delete(registry.pending, envelope.Session)
	registry.mu.Unlock()

	result := clusterCallResult{}
	if envelope.Kind == FrameResponse {
		result.payload = append([]byte(nil), envelope.Payload...)
	} else {
		status := *envelope.Error
		result.err = &status
	}
	finishClusterPendingCall(pending, result)
	return nil
}

func (registry *clusterPendingCalls) cancel(session ClusterSessionID, err error) bool {
	if registry == nil || session == 0 {
		return false
	}
	registry.mu.Lock()
	pending, exists := registry.pending[session]
	if exists {
		delete(registry.pending, session)
	}
	registry.mu.Unlock()
	if !exists {
		return false
	}
	finishClusterPendingCall(pending, clusterCallResult{err: err})
	return true
}

func (registry *clusterPendingCalls) close(err error) error {
	if registry == nil {
		return nil
	}
	registry.mu.Lock()
	if registry.closed {
		registry.mu.Unlock()
		return nil
	}
	registry.closed = true
	pending := make([]*clusterPendingCall, 0, len(registry.pending))
	for _, call := range registry.pending {
		pending = append(pending, call)
	}
	registry.pending = make(map[ClusterSessionID]*clusterPendingCall)
	registry.mu.Unlock()
	if err == nil {
		err = ErrClusterCallClosed
	}
	for _, call := range pending {
		finishClusterPendingCall(call, clusterCallResult{err: err})
	}
	return nil
}

func (registry *clusterPendingCalls) pendingCount() int {
	if registry == nil {
		return 0
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	return len(registry.pending)
}

func finishClusterPendingCall(pending *clusterPendingCall, result clusterCallResult) {
	pending.done <- result
	close(pending.done)
}
