package runtime

import (
	"context"
	"errors"
	"fmt"
)

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

	runtime.mu.Lock()
	if runtime.state != RuntimeRunning {
		err := runtime.runtimeAdmissionErrorLocked()
		runtime.mu.Unlock()
		return nil, err
	}
	runtime.mu.Unlock()

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
	if err := runtime.dispatcher.dispatchCallFromService(caller, target, command, args, pending.session); err != nil {
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
