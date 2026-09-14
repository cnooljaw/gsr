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
// Call 的执行错误经共享响应函数回包；Send 的本地接纳错误返回接收侧记录，
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
		return &badPayloadError{err}
	}
	args, ok := decoded.([]any)
	if !ok {
		return &badPayloadError{runtime.ErrInvalidCommandArguments}
	}
	var targetErr error
	target := request.Target.ServiceID
	if request.Target.ServiceName != "" {
		if adapter.resolve == nil {
			targetErr = runtime.ErrServiceNotFound
		} else {
			target, targetErr = adapter.resolve(request.Target.ServiceName)
		}
	}
	// 解码或外层名称解析期间可能取消，保留实际投递前的第二次检查。
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.Session == 0 {
		if targetErr != nil {
			return targetErr
		}
		return adapter.runtime.Send(target, request.Command, args...)
	}
	if targetErr != nil {
		return writeRequestResponse(write, request, nil, targetErr)
	}
	value, err := adapter.runtime.Call(ctx, target, request.Command, args...)
	if err != nil {
		return writeRequestResponse(write, request, nil, err)
	}
	payload, encodingErr := encodeLocalResponse(codec, value)
	writeErr := writeRequestResponse(write, request, payload, encodingErr)
	// 编码失败既需安全回包，也需在 B 侧记录；写入失败不能掩盖编码原因。
	return errors.Join(writeErr, encodingErr)
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
