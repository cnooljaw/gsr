package cluster

import (
	"context"
	"errors"
	"fmt"

	"github.com/cnooljaw/gsr/runtime"
)

// localRequestAdapter 将已解帧的入站请求落地到本地 Runtime。
// 名称解析留在 Cluster 外层；此适配器不管理连接、注册表变更或 goroutine。
type localRequestAdapter struct {
	runtime *runtime.Runtime
	codecs  *CommandCodecRegistry
	resolve func(ServiceName) (runtime.ServiceID, error)
}

// handle 在调用 Service 之前完成协议校验，坏载荷只返回本地诊断错误，不回包。
// Call 的执行错误由 ConnectionAgent 回包；Send 的本地接纳错误返回接收侧记录，
// 已接纳 Send 的 Handler 错误仍由目标 Runtime 记录。
func (adapter *localRequestAdapter) handle(ctx context.Context, request ClusterEnvelope, write ResponseWriter) error {
	if adapter == nil || adapter.runtime == nil || adapter.codecs == nil || write == nil {
		return ErrInvalidConnectionAgentConfig
	}
	if ctx == nil || request.Kind != FrameRequest {
		return ErrInvalidConnectionAgentRequest
	}
	if err := request.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConnectionAgentRequest, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	codec, err := adapter.codecs.Lookup(request.Command)
	if err != nil {
		return err
	}
	decoded, err := codec.Decode(request.Payload)
	if err != nil {
		return err
	}
	args, ok := decoded.([]any)
	if !ok {
		return runtime.ErrInvalidCommandArguments
	}
	var localErr error
	var encodingErr error
	target := request.Target.ServiceID
	if request.Target.ServiceName != "" {
		if adapter.resolve == nil {
			localErr = runtime.ErrServiceNotFound
		} else {
			target, localErr = adapter.resolve(request.Target.ServiceName)
		}
	}
	agent := ConnectionAgent{
		write: write,
		handler: func(callCtx context.Context, envelope ClusterEnvelope) ([]byte, error) {
			if localErr != nil {
				return nil, localErr
			}
			if envelope.Session == 0 {
				localErr = adapter.runtime.Send(target, envelope.Command, args...)
				return nil, localErr
			}
			value, err := adapter.runtime.Call(callCtx, target, envelope.Command, args...)
			if err != nil {
				return nil, err
			}
			payload, err := encodeLocalResponse(codec, value)
			encodingErr = err
			return payload, err
		},
	}
	if err := agent.Handle(ctx, request); err != nil {
		return errors.Join(err, encodingErr)
	}
	if request.Session == 0 {
		return localErr
	}
	return encodingErr
}

// encodeLocalResponse 将编码扩展的 panic 转为安全错误，同时供回包和本地诊断使用。
func encodeLocalResponse(codec CommandCodec, value any) (payload []byte, err error) {
	defer func() {
		if recover() != nil {
			payload = nil
			err = errInboundRequestPanicked
		}
	}()
	return codec.Encode(value)
}
