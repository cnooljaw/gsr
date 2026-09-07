package runtime

import (
	"context"
	"errors"
	"fmt"
)

// Call 是 Infrastructure API，供宿主或受信任基础设施调用本地 Service，消息 Source 固定为零。
// 普通 Service 应使用 ServiceContext.Call；目标仍通过 Mailbox 串行执行。
// 等待不超过 MaxCallWait，ctx 可进一步缩短；本方法不创建 goroutine。
func (runtime *Runtime) Call(ctx context.Context, target ServiceID, command CommandID, args ...any) (any, error) {
	if ctx == nil {
		return nil, ErrInvalidCallContext
	}
	if runtime == nil {
		return nil, ErrRuntimeClosed
	}
	if err := runtime.callAdmissionError(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runtime.pendingCalls == nil {
		return nil, ErrRuntimeClosed
	}
	pending, err := runtime.pendingCalls.beginRuntimeOwned(target)
	if err != nil {
		return nil, err
	}
	return runtime.waitCall(ctx, pending, func(session SessionID) error {
		return runtime.dispatcher.dispatchCallRuntimeOwned(target, command, args, session)
	})
}

func (runtime *Runtime) callAdmissionError() error {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.state != RuntimeRunning {
		return runtime.runtimeAdmissionErrorLocked()
	}
	return nil
}

func (runtime *Runtime) call(
	caller *serviceInstance,
	ctx context.Context,
	target ServiceID,
	command CommandID,
	args ...any,
) (any, error) {
	if ctx == nil {
		return nil, ErrInvalidCallContext
	}
	if runtime == nil || caller == nil || caller.runtime != runtime {
		return nil, ErrInvalidService
	}

	if err := runtime.callAdmissionError(); err != nil {
		return nil, err
	}

	switch caller.currentState() {
	case serviceStarting, serviceRunning:
	case serviceStopping:
		return nil, ErrServiceStopping
	case serviceStopped, serviceFailed:
		return nil, ErrServiceClosed
	default:
		return nil, ErrServiceClosed
	}
	if target == caller.id {
		return nil, ErrCallCycle
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runtime.pendingCalls == nil {
		return nil, ErrRuntimeClosed
	}
	pending, err := runtime.pendingCalls.begin(caller, target)
	if err != nil {
		return nil, err
	}
	return runtime.waitCall(ctx, pending, func(session SessionID) error {
		return runtime.dispatcher.dispatchCallFromService(caller, target, command, args, session)
	})
}

// waitCall 共享等待与完成机制；来源身份由调用方选择的显式分派入口确定。
func (runtime *Runtime) waitCall(ctx context.Context, pending *pendingCall, dispatch func(SessionID) error) (any, error) {
	if runtime.timers == nil || runtime.timers.clock == nil {
		runtime.pendingCalls.cancel(pending.session, ErrRuntimeClosed)
		result := <-pending.done
		return result.value, result.err
	}
	maxTimer := runtime.timers.clock.NewTimer(runtime.config.MaxCallWait)
	defer maxTimer.Stop()
	if err := ctx.Err(); err != nil {
		runtime.pendingCalls.cancel(pending.session, err)
		result := <-pending.done
		return result.value, result.err
	}

	if runtime.dispatcher == nil {
		runtime.pendingCalls.cancel(pending.session, ErrRuntimeClosed)
		result := <-pending.done
		return result.value, result.err
	}
	if err := dispatch(pending.session); err != nil {
		runtime.pendingCalls.cancel(pending.session, err)
		result := <-pending.done
		return result.value, result.err
	}

	select {
	case result := <-pending.done:
		return result.value, result.err
	case <-ctx.Done():
		if runtime.pendingCalls.cancel(pending.session, ctx.Err()) {
			return nil, ctx.Err()
		}
		result := <-pending.done
		return result.value, result.err
	case <-maxTimer.C():
		timeoutErr := callTimeoutError()
		if runtime.pendingCalls.cancel(pending.session, timeoutErr) {
			runtime.recordCallTimeout()
			return nil, timeoutErr
		}
		result := <-pending.done
		return result.value, result.err
	}
}

func callTimeoutError() error {
	return fmt.Errorf("%w: %w", ErrCallTimeout, context.DeadlineExceeded)
}

func normalizeCallError(err error) error {
	if err == nil {
		return nil
	}
	if isStatusCode(err, ErrorCodeServicePanicked) {
		return ErrServicePanicked
	}
	var status *StatusError
	if errors.As(err, &status) && status != nil {
		return NewStatusError(status.Class, status.Code, status.Message)
	}
	return ErrHandlerFailed
}

func (runtime *Runtime) completeMessage(message Message, result any, err error) {
	if runtime == nil || message.Session == 0 || runtime.pendingCalls == nil {
		return
	}
	runtime.pendingCalls.complete(message.Session, callResult{
		value: result,
		err:   normalizeCallError(err),
	})
}
