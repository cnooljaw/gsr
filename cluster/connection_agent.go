package cluster

import (
	"context"
	"errors"
	"fmt"

	runtime "github.com/cnooljaw/gsr/runtime"
)

// RequestHandler 将 Cluster 请求适配到本地 Runtime/Service 分派路径。
// 返回的字节必须已通过该命令的响应编解码器编码。
type RequestHandler func(context.Context, NodeID, ClusterEnvelope) ([]byte, error)

// ResponseWriter 通过 agent 所属的入站连接写回一个响应。
// TCP 连接管理组件可以将它绑定到自身的帧写入函数。
type ResponseWriter func(ClusterEnvelope) error

// ConnectionAgentConfig 提供单条入站连接的分派与写入能力。
// agent 自身不管理 socket 或 goroutine。
type ConnectionAgentConfig struct {
	Handler RequestHandler
	Write   ResponseWriter
}

var (
	// ErrInvalidConnectionAgentConfig 表示 agent 的依赖无效。
	ErrInvalidConnectionAgentConfig = errors.New("cluster: invalid connection agent config")
	// ErrInvalidConnectionAgentRequest 表示入站请求违反 Cluster 逻辑请求边界。
	ErrInvalidConnectionAgentRequest = errors.New("cluster: invalid connection agent request")
	// ErrConnectionAgentWrite 表示 agent 无法写回 Call 响应。
	ErrConnectionAgentWrite = errors.New("cluster: connection agent write failed")
)

var errConnectionAgentHandlerPanicked = errors.New("cluster: connection agent handler panicked")

// ConnectionAgent 处理单条入站连接收到的逻辑请求。
// 远端 NodeID 元数据保留在 Cluster 边界，agent 自身不构造或修改 Core Message。
type ConnectionAgent struct {
	handler RequestHandler
	write   ResponseWriter
}

// NewConnectionAgent 校验并构造一个入站连接 agent。
func NewConnectionAgent(config ConnectionAgentConfig) (*ConnectionAgent, error) {
	if config.Handler == nil || config.Write == nil {
		return nil, fmt.Errorf("%w: Handler and Write are required", ErrInvalidConnectionAgentConfig)
	}
	return &ConnectionAgent{
		handler: config.Handler,
		write:   config.Write,
	}, nil
}

// Handle 校验入站 Request 并通过本地适配器分派。
// Send（Session == 0）不写回响应；Call 使用原始 Session、Target 和 Command
// 写回一个 Response 或结构化 Error。
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
		// Send 没有响应路径，由接收节点的本地 Runtime/适配器记录 Handler 错误。
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
