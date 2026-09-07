package cluster

import (
	"context"
	"errors"
	"fmt"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// RequestHandler is the narrow adapter from a Cluster request to the local
// Runtime/Service dispatch path. The returned bytes must already be encoded
// with the command's response codec.
type RequestHandler func(context.Context, NodeID, ClusterEnvelope) ([]byte, error)

// ResponseWriter writes one response on the inbound connection that owns the
// agent. A TCP connection owner can bind this to its framed writer.
type ResponseWriter func(ClusterEnvelope) error

// ConnectionAgentConfig supplies one inbound connection's dispatch and write
// capabilities. The agent itself owns neither a socket nor a goroutine.
type ConnectionAgentConfig struct {
	Handler RequestHandler
	Write   ResponseWriter
}

var (
	// ErrInvalidConnectionAgentConfig means an agent dependency is invalid.
	ErrInvalidConnectionAgentConfig = errors.New("cluster: invalid connection agent config")
	// ErrInvalidConnectionAgentRequest means an inbound request violates the
	// logical Cluster request boundary.
	ErrInvalidConnectionAgentRequest = errors.New("cluster: invalid connection agent request")
	// ErrConnectionAgentWrite means an agent could not write a Call response.
	ErrConnectionAgentWrite = errors.New("cluster: connection agent write failed")
)

var errConnectionAgentHandlerPanicked = errors.New("cluster: connection agent handler panicked")

// ConnectionAgent handles logical requests received on one inbound
// connection. It keeps remote NodeID metadata at the Cluster boundary and
// never constructs or mutates a Core Message itself.
type ConnectionAgent struct {
	handler RequestHandler
	write   ResponseWriter
}

// NewConnectionAgent validates and constructs one inbound connection agent.
func NewConnectionAgent(config ConnectionAgentConfig) (*ConnectionAgent, error) {
	if config.Handler == nil || config.Write == nil {
		return nil, fmt.Errorf("%w: Handler and Write are required", ErrInvalidConnectionAgentConfig)
	}
	return &ConnectionAgent{
		handler: config.Handler,
		write:   config.Write,
	}, nil
}

// Handle validates one inbound Request and dispatches it through the local
// adapter. A Send (Session == 0) never writes a response; a Call writes one
// Response or structured Error with the original Session, Target and Command.
func (agent *ConnectionAgent) Handle(ctx context.Context, from NodeID, request ClusterEnvelope) error {
	if agent == nil || agent.handler == nil || agent.write == nil {
		return ErrInvalidConnectionAgentConfig
	}
	if ctx == nil {
		return fmt.Errorf("%w: context is nil", ErrInvalidConnectionAgentRequest)
	}
	if from == "" {
		return fmt.Errorf("%w: source node is empty", ErrInvalidConnectionAgentRequest)
	}
	if request.Kind != FrameRequest {
		return fmt.Errorf("%w: frame kind %s is not a request", ErrInvalidConnectionAgentRequest, request.Kind)
	}
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConnectionAgentRequest, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	payload, handlerErr := invokeConnectionAgentHandler(agent.handler, ctx, from, request.Clone())
	if request.Session == 0 {
		// Send owns no response path. The local Runtime/adapter is responsible
		// for recording a Handler error on the receiving node.
		return nil
	}
	response := ClusterEnvelope{
		Kind:    FrameResponse,
		Session: request.Session,
		Target:  request.Target,
		Command: request.Command,
		Payload: payload,
	}
	if handlerErr != nil {
		response.Kind = FrameError
		response.Payload = nil
		response.Error = normalizeConnectionAgentError(handlerErr)
	}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConnectionAgentRequest, err)
	}
	if err := writeConnectionAgentResponse(agent.write, response); err != nil {
		return err
	}
	return nil
}

func invokeConnectionAgentHandler(handler RequestHandler, ctx context.Context, from NodeID, request ClusterEnvelope) (payload []byte, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errConnectionAgentHandlerPanicked
			payload = nil
		}
	}()
	return handler(ctx, from, request)
}

func writeConnectionAgentResponse(writer ResponseWriter, response ClusterEnvelope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: response writer panicked", ErrConnectionAgentWrite)
		}
	}()
	if err := writer(response.Clone()); err != nil {
		return fmt.Errorf("%w: %w", ErrConnectionAgentWrite, err)
	}
	return nil
}

func normalizeConnectionAgentError(err error) *runtime.StatusError {
	if err == nil {
		return nil
	}
	if errors.Is(err, errConnectionAgentHandlerPanicked) {
		return cloneConnectionAgentStatus(runtime.ErrServicePanicked)
	}
	var status *runtime.StatusError
	if errors.As(err, &status) && status != nil {
		return runtime.NewStatusError(status.Class, status.Code, status.Message)
	}
	return cloneConnectionAgentStatus(runtime.ErrHandlerFailed)
}

func cloneConnectionAgentStatus(err error) *runtime.StatusError {
	var status *runtime.StatusError
	if errors.As(err, &status) && status != nil {
		return runtime.NewStatusError(status.Class, status.Code, status.Message)
	}
	return runtime.NewStatusError(runtime.ErrorClassRuntime, runtime.ErrorCodeHandlerFailed, "service Handler failed")
}
