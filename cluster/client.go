package cluster

import (
	"errors"
	"fmt"
	"reflect"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// OutboundSender is the minimum capability a ClusterClient needs to admit a
// request. NodeSender is the standard implementation.
type OutboundSender interface {
	Send(ClusterEnvelope) error
}

// NodeSenderResolver supplies the unique outbound sender for a remote node.
// Sender ownership, connection creation and reconnection remain outside the
// per-Service ClusterClient.
type NodeSenderResolver interface {
	ResolveNodeSender(NodeID) (OutboundSender, error)
}

// ClusterClientConfig binds a per-Service source to Cluster infrastructure.
type ClusterClientConfig struct {
	Source  runtime.ServiceID
	Senders NodeSenderResolver
	Codecs  *CommandCodecRegistry
}

var (
	// ErrInvalidClusterClientConfig means a ClusterClient dependency is invalid.
	ErrInvalidClusterClientConfig = errors.New("cluster: invalid client config")
	// ErrNodeSenderUnavailable means a resolver did not provide a sender.
	ErrNodeSenderUnavailable = errors.New("cluster: node sender unavailable")
)

// ClusterClient is a narrow, source-bound client for remote Cluster requests.
// It only assembles and admits one-way request envelopes; sender ownership and
// remote execution stay in the Cluster layer.
type ClusterClient struct {
	source  runtime.ServiceID
	senders NodeSenderResolver
	codecs  *CommandCodecRegistry
}

// NewClusterClient validates and constructs one source-bound client.
func NewClusterClient(config ClusterClientConfig) (*ClusterClient, error) {
	if config.Source == 0 || isNilNodeSenderResolver(config.Senders) || config.Codecs == nil {
		return nil, fmt.Errorf("%w: Source, Senders, and Codecs are required", ErrInvalidClusterClientConfig)
	}
	return &ClusterClient{
		source:  config.Source,
		senders: config.Senders,
		codecs:  config.Codecs,
	}, nil
}

// Source returns the Runtime ServiceID captured when this client was bound.
// It is read-only and cannot be replaced for an individual request.
func (client *ClusterClient) Source() runtime.ServiceID {
	if client == nil {
		return 0
	}
	return client.source
}

// Send encodes args as one command argument list and admits a request to the
// resolved remote NodeSender. A nil error confirms local admission only.
func (client *ClusterClient) Send(node NodeID, target ServiceTarget, command runtime.CommandID, args ...any) error {
	if client == nil || client.source == 0 || isNilNodeSenderResolver(client.senders) || client.codecs == nil {
		return ErrInvalidClusterClientConfig
	}
	if node == "" {
		return ErrInvalidNode
	}
	if err := target.validate(); err != nil {
		return err
	}
	codec, err := client.codecs.Lookup(command)
	if err != nil {
		return err
	}
	ownedArgs := append([]any(nil), args...)
	payload, err := codec.Encode(ownedArgs)
	if err != nil {
		return fmt.Errorf("encode command %d: %w", command, err)
	}
	sender, err := client.senders.ResolveNodeSender(node)
	if err != nil {
		return fmt.Errorf("resolve sender for node %q: %w", node, err)
	}
	if isNilOutboundSender(sender) {
		return fmt.Errorf("%w: node %q", ErrNodeSenderUnavailable, node)
	}
	return sender.Send(ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  target,
		Command: command,
		Payload: payload,
	})
}

func isNilNodeSenderResolver(resolver NodeSenderResolver) bool {
	if resolver == nil {
		return true
	}
	value := reflect.ValueOf(resolver)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func isNilOutboundSender(sender OutboundSender) bool {
	if sender == nil {
		return true
	}
	value := reflect.ValueOf(sender)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
