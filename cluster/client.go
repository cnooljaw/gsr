package cluster

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

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
	Source       runtime.ServiceID
	Senders      NodeSenderResolver
	Codecs       *CommandCodecRegistry
	PendingCalls *ClusterCallRegistry
	MaxCallWait  time.Duration
}

var (
	// ErrInvalidClusterClientConfig means a ClusterClient dependency is invalid.
	ErrInvalidClusterClientConfig = errors.New("cluster: invalid client config")
	// ErrNodeSenderUnavailable means a resolver did not provide a sender.
	ErrNodeSenderUnavailable = errors.New("cluster: node sender unavailable")
)

// ClusterClient is a narrow, source-bound client for remote Cluster requests.
// Sender ownership, response completion and remote execution stay in the
// Cluster layer.
type ClusterClient struct {
	source       runtime.ServiceID
	senders      NodeSenderResolver
	codecs       *CommandCodecRegistry
	pendingCalls *ClusterCallRegistry
	maxCallWait  time.Duration
}

// NewClusterClient validates and constructs one source-bound client.
func NewClusterClient(config ClusterClientConfig) (*ClusterClient, error) {
	if config.Source == 0 || isNilNodeSenderResolver(config.Senders) || config.Codecs == nil || !config.PendingCalls.valid() || config.MaxCallWait <= 0 {
		return nil, fmt.Errorf("%w: Source, Senders, Codecs, PendingCalls, and positive MaxCallWait are required", ErrInvalidClusterClientConfig)
	}
	return &ClusterClient{
		source:       config.Source,
		senders:      config.Senders,
		codecs:       config.Codecs,
		pendingCalls: config.PendingCalls,
		maxCallWait:  config.MaxCallWait,
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
	_, sender, payload, err := client.prepareRequest(node, target, command, args...)
	if err != nil {
		return err
	}
	return sender.Send(ClusterEnvelope{
		Kind:    FrameRequest,
		Target:  target,
		Command: command,
		Payload: payload,
	})
}

// Call sends one remote request and waits for its Response or Error.
//
// The effective wait is bounded by MaxCallWait and may be shortened by ctx.
// A nil ctx is rejected, and a successful return means only that the matching
// remote response was received and decoded; remote business errors are
// returned as runtime.StatusError values.
func (client *ClusterClient) Call(ctx context.Context, node NodeID, target ServiceTarget, command runtime.CommandID, args ...any) (any, error) {
	if ctx == nil {
		return nil, runtime.ErrInvalidCallContext
	}
	if err := client.validateConfig(); err != nil {
		return nil, err
	}
	effectiveContext, cancel := context.WithTimeout(ctx, client.maxCallWait)
	defer cancel()
	if err := effectiveContext.Err(); err != nil {
		return nil, clusterCallWaitError(ctx, effectiveContext)
	}
	codec, sender, payload, err := client.prepareRequest(node, target, command, args...)
	if err != nil {
		return nil, err
	}
	if err := effectiveContext.Err(); err != nil {
		return nil, clusterCallWaitError(ctx, effectiveContext)
	}
	pending, err := client.pendingCalls.begin(client.source, node, target, command)
	if err != nil {
		return nil, err
	}
	request := ClusterEnvelope{
		Kind:    FrameRequest,
		Session: pending.session,
		Target:  target,
		Command: command,
		Payload: payload,
	}
	if err := sender.Send(request); err != nil {
		client.pendingCalls.cancel(pending.session, err)
		return nil, err
	}
	select {
	case result := <-pending.done:
		return decodeClusterCallResult(codec, command, result)
	case <-effectiveContext.Done():
		waitErr := clusterCallWaitError(ctx, effectiveContext)
		if client.pendingCalls.cancel(pending.session, waitErr) {
			return nil, waitErr
		}
		result := <-pending.done
		return decodeClusterCallResult(codec, command, result)
	}
}

func (client *ClusterClient) validateConfig() error {
	if client == nil || client.source == 0 || isNilNodeSenderResolver(client.senders) || client.codecs == nil || !client.pendingCalls.valid() || client.maxCallWait <= 0 {
		return ErrInvalidClusterClientConfig
	}
	return nil
}

func (client *ClusterClient) prepareRequest(node NodeID, target ServiceTarget, command runtime.CommandID, args ...any) (CommandCodec, OutboundSender, []byte, error) {
	if err := client.validateConfig(); err != nil {
		return nil, nil, nil, err
	}
	if node == "" {
		return nil, nil, nil, ErrInvalidNode
	}
	if err := target.validate(); err != nil {
		return nil, nil, nil, err
	}
	codec, err := client.codecs.Lookup(command)
	if err != nil {
		return nil, nil, nil, err
	}
	ownedArgs := append([]any(nil), args...)
	payload, err := codec.Encode(ownedArgs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("encode command %d: %w", command, err)
	}
	sender, err := client.senders.ResolveNodeSender(node)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("resolve sender for node %q: %w", node, err)
	}
	if isNilOutboundSender(sender) {
		return nil, nil, nil, fmt.Errorf("%w: node %q", ErrNodeSenderUnavailable, node)
	}
	return codec, sender, payload, nil
}

func clusterCallWaitError(ctx context.Context, effectiveContext context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(effectiveContext.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w", runtime.ErrCallTimeout, context.DeadlineExceeded)
	}
	return effectiveContext.Err()
}

func decodeClusterCallResult(codec CommandCodec, command runtime.CommandID, result clusterCallResult) (any, error) {
	if result.err != nil {
		return nil, result.err
	}
	value, err := codec.Decode(result.payload)
	if err != nil {
		return nil, fmt.Errorf("decode response for command %d: %w", command, err)
	}
	return value, nil
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
