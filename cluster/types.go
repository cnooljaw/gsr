// Package cluster contains the transport and routing layer built on top of
// the local runtime Core.
package cluster

import (
	"errors"
	"fmt"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// NodeID selects a remote Runtime node. It is not encoded into a Core ServiceID.
type NodeID string

// ServiceName is a stable logical Service name resolved outside the Core.
type ServiceName string

// ClusterSessionID correlates one remote Call with its response or error.
// Zero identifies a one-way Send and does not require a response.
type ClusterSessionID uint64

// FrameKind identifies the direction and meaning of a ClusterEnvelope.
type FrameKind uint8

const (
	// FrameRequest carries a remote Send or Call request.
	FrameRequest FrameKind = iota + 1
	// FrameResponse carries a successful remote Call result.
	FrameResponse
	// FrameError carries a structured remote Call or delivery error.
	FrameError
)

// String returns the stable display name of a FrameKind.
func (kind FrameKind) String() string {
	switch kind {
	case FrameRequest:
		return "request"
	case FrameResponse:
		return "response"
	case FrameError:
		return "error"
	default:
		return "unknown"
	}
}

// ServiceTarget identifies a remote Service by its temporary local ID or its
// stable logical name. Exactly one field must be set.
type ServiceTarget struct {
	ServiceID   runtime.ServiceID
	ServiceName ServiceName
}

// ClusterEnvelope is the value boundary between Cluster and a transport.
//
// The transport supplies the source and destination NodeID. The envelope does
// not carry a Core Message pointer, NodeID, or an arbitrary source identity.
type ClusterEnvelope struct {
	Kind    FrameKind
	Session ClusterSessionID
	Target  ServiceTarget
	Command runtime.CommandID
	Payload []byte
	Error   *runtime.StatusError
}

var (
	// ErrInvalidEnvelope means an envelope violates the Cluster frame contract.
	ErrInvalidEnvelope = errors.New("cluster: invalid envelope")
	// ErrPayloadTooLarge means a transport payload exceeds its configured limit.
	ErrPayloadTooLarge = errors.New("cluster: payload too large")
)

// Validate checks the structural contract shared by all transports.
func (envelope ClusterEnvelope) Validate() error {
	if envelope.Kind != FrameRequest && envelope.Kind != FrameResponse && envelope.Kind != FrameError {
		return fmt.Errorf("%w: unknown frame kind %d", ErrInvalidEnvelope, envelope.Kind)
	}
	if err := envelope.Target.validate(); err != nil {
		return err
	}
	if envelope.Command == 0 {
		return fmt.Errorf("%w: command is zero", ErrInvalidEnvelope)
	}

	switch envelope.Kind {
	case FrameRequest:
		if envelope.Error != nil {
			return fmt.Errorf("%w: request contains error", ErrInvalidEnvelope)
		}
	case FrameResponse:
		if envelope.Session == 0 {
			return fmt.Errorf("%w: response session is zero", ErrInvalidEnvelope)
		}
		if envelope.Error != nil {
			return fmt.Errorf("%w: response contains error", ErrInvalidEnvelope)
		}
	case FrameError:
		if envelope.Session == 0 {
			return fmt.Errorf("%w: error session is zero", ErrInvalidEnvelope)
		}
		if err := validateStatusError(envelope.Error); err != nil {
			return err
		}
		if len(envelope.Payload) != 0 {
			return fmt.Errorf("%w: error contains payload", ErrInvalidEnvelope)
		}
	}
	return nil
}

func (target ServiceTarget) validate() error {
	hasID := target.ServiceID != 0
	hasName := target.ServiceName != ""
	if hasID == hasName {
		return fmt.Errorf("%w: exactly one ServiceID or ServiceName is required", ErrInvalidEnvelope)
	}
	return nil
}

func validateStatusError(status *runtime.StatusError) error {
	if status == nil || status.Class == "" || status.Code == "" {
		return fmt.Errorf("%w: error status must include class and code", ErrInvalidEnvelope)
	}
	return nil
}

// Clone returns an envelope whose mutable payload and error value are owned by
// the returned value. It is used at transport admission boundaries.
func (envelope ClusterEnvelope) Clone() ClusterEnvelope {
	clone := envelope
	clone.Payload = append([]byte(nil), envelope.Payload...)
	if envelope.Error != nil {
		status := *envelope.Error
		clone.Error = &status
	}
	return clone
}
