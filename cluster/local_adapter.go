package cluster

import (
	"context"
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
func (adapter *localRequestAdapter) handle(ctx context.Context, from NodeID, request ClusterEnvelope, write ResponseWriter) error {
	if adapter == nil || adapter.runtime == nil || adapter.codecs == nil || write == nil {
		return ErrInvalidConnectionAgentConfig
	}
	if ctx == nil || from == "" || request.Kind != FrameRequest {
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
	agent := ConnectionAgent{
		write: write,
		handler: func(callCtx context.Context, _ NodeID, envelope ClusterEnvelope) ([]byte, error) {
			target := envelope.Target.ServiceID
			if envelope.Target.ServiceName != "" {
				if adapter.resolve == nil {
					localErr = runtime.ErrServiceNotFound
					return nil, localErr
				}
				target, localErr = adapter.resolve(envelope.Target.ServiceName)
				if localErr != nil {
					return nil, localErr
				}
			}
			if envelope.Session == 0 {
				localErr = adapter.runtime.Send(target, envelope.Command, args...)
				return nil, localErr
			}
			value, err := adapter.runtime.Call(callCtx, target, envelope.Command, args...)
			if err != nil {
				return nil, err
			}
			return codec.Encode(value)
		},
	}
	if err := agent.Handle(ctx, from, request); err != nil {
		return err
	}
	if request.Session == 0 {
		return localErr
	}
	return nil
}
