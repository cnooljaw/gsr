package runtime

import (
	"context"
	"errors"
)

// StopServices 是基础设施的局部回收入口，不停止整个 Runtime。
// 先关闭所有目标的接纳、清理其 Timer/PendingCall，再在共享 ShutdownTimeout 预算内等待。
// ctx 只限制本次等待，取消不会撤销停止；不能强杀 Handler，未结束 runner 仍由 Runtime 跟踪。
// 缺失 ID 返回 ErrServiceNotFound，其余目标仍会停止；重复 ID 只处理一次。
func (runtime *Runtime) StopServices(ctx context.Context, ids ...ServiceID) error {
	if runtime == nil {
		return ErrRuntimeClosed
	}
	if ctx == nil {
		ctx = context.Background()
	}
	instances := make([]*serviceInstance, 0, len(ids))
	seen := make(map[ServiceID]struct{}, len(ids))
	var result error
	runtime.servicesMu.RLock()
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		instance := runtime.services[id]
		if instance == nil {
			result = errors.Join(result, ErrServiceNotFound)
			continue
		}
		instances = append(instances, instance)
	}
	runtime.servicesMu.RUnlock()
	for _, instance := range instances {
		stopContext, cancel := runtime.failureContext()
		instance.requestStopWithCancel(stopContext, cancel)
	}
	waitContext, cancel := context.WithTimeout(ctx, runtime.config.ShutdownTimeout)
	defer cancel()
	for _, instance := range instances {
		if err := waitForSignal(waitContext, instance.done); err != nil {
			return errors.Join(result, err)
		}
		result = errors.Join(result, instance.result())
	}
	return result
}
