package cluster

import (
	"errors"
	"fmt"

	"github.com/cnooljaw/gsr/runtime"
)

var (
	// errInvalidInboundRequest 标记投递前的非法逻辑请求，供坏帧诊断分类。
	errInvalidInboundRequest = errors.New("cluster: invalid inbound request")
	// errInboundResponseWrite 标记原连接写回失败，连接 owner 据此关闭失效连接。
	errInboundResponseWrite = errors.New("cluster: inbound response write failed")
)

// writeRequestResponse 沿入站原连接写回 Call 结果；Send 在本地适配器中不走此路径。
func writeRequestResponse(writer func(ClusterEnvelope) error, request ClusterEnvelope, payload []byte, handlerErr error) error {
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
		response.Error = normalizeInboundError(handlerErr)
	}
	if err := response.Validate(); err != nil {
		return fmt.Errorf("%w: %v", errInvalidInboundRequest, err)
	}
	return writeInboundResponse(writer, response)
}

func writeInboundResponse(writer func(ClusterEnvelope) error, response ClusterEnvelope) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w: response writer panicked", errInboundResponseWrite)
		}
	}()
	if err := writer(response.Clone()); err != nil {
		return fmt.Errorf("%w: %w", errInboundResponseWrite, err)
	}
	return nil
}

// normalizeInboundError 的错误链分类同样属于扩展边界，panic 不能跳过 Call 回包。
func normalizeInboundError(err error) (status *runtime.StatusError) {
	defer func() {
		if recover() != nil {
			status = cloneInboundStatus(runtime.ErrHandlerFailed)
		}
	}()
	if err == nil {
		return nil
	}
	if errors.As(err, &status) && status != nil {
		return runtime.NewStatusError(status.Class, status.Code, status.Message)
	}
	return cloneInboundStatus(runtime.ErrHandlerFailed)
}

func cloneInboundStatus(err error) *runtime.StatusError {
	var status *runtime.StatusError
	if errors.As(err, &status) && status != nil {
		return runtime.NewStatusError(status.Class, status.Code, status.Message)
	}
	return runtime.NewStatusError(runtime.ErrorClassRuntime, runtime.ErrorCodeHandlerFailed, "service Handler failed")
}
