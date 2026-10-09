package gate

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

const (
	disconnectRetryInterval = 5 * time.Millisecond
	disconnectAttemptWait   = 100 * time.Millisecond
)

// createAgent 捕获业务构造回调的 panic，并拒绝空 Agent。
func createAgent(factory func(ClientSession) runtime.Service, session ClientSession) (service runtime.Service, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			service = nil
			err = fmt.Errorf("gate: Agent constructor panicked: %v", recovered)
		}
	}()
	service = factory(session)
	if service == nil {
		return nil, errors.New("gate: Agent constructor returned nil")
	}
	return service, nil
}

// callGateConnect 在调用方有限期限内等待 Gate 发布或 Mailbox 恢复。
// 只重试尚未接纳的失败；Handler error、Call 超时或取消均直接返回，避免重复创建 Agent。
func callGateConnect(ctx context.Context, local *runtime.Runtime, serviceID runtime.ServiceID, command runtime.CommandID, connection any) (any, error) {
	ticker := time.NewTicker(disconnectRetryInterval)
	defer ticker.Stop()
	for {
		result, err := local.Call(ctx, serviceID, command, connection)
		if err == nil || (!errors.Is(err, runtime.ErrServiceNotFound) && !errors.Is(err, runtime.ErrMailboxFull)) {
			return result, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
		}
	}
}

// retryDisconnect 在正常运行时重试 Gate runner 的断线投递；停机后交给 OnStop 补发。
func retryDisconnect(local *runtime.Runtime, serviceID runtime.ServiceID, command runtime.CommandID, connection any, stopping func() bool) {
	ticker := time.NewTicker(disconnectRetryInterval)
	defer ticker.Stop()
	for {
		if stopping() {
			return
		}
		attemptContext, cancel := context.WithTimeout(context.Background(), disconnectAttemptWait)
		_, err := local.Call(attemptContext, serviceID, command, connection)
		cancel()
		if err == nil || isRuntimeStoppingError(err) || !isDisconnectRetryableError(err) {
			return
		}
		<-ticker.C
	}
}

// notifyAgentsOnStop 在停止期限内公平重试所有仍有连接映射的 Agent。
// pending 由调用方创建，排序和重试期间不再供调用方使用。
func notifyAgentsOnStop(ctx context.Context, local *runtime.Runtime, pending []runtime.ServiceID) error {
	sort.Slice(pending, func(left, right int) bool { return pending[left] < pending[right] })
	retry := make([]runtime.ServiceID, 0, len(pending))
	ticker := time.NewTicker(disconnectRetryInterval)
	defer ticker.Stop()
	var result error
	for len(pending) > 0 {
		retry = retry[:0]
		for _, agentID := range pending {
			if err := ctx.Err(); err != nil {
				return errors.Join(result, err)
			}
			err := local.Send(agentID, ClientDisconnectedCommand)
			if err == nil || isRuntimeStoppingError(err) {
				continue
			}
			if errors.Is(err, runtime.ErrMailboxFull) {
				retry = append(retry, agentID)
				continue
			}
			result = errors.Join(result, fmt.Errorf("gate: notify Agent %d on Gate stop: %w", agentID, err))
		}
		pending, retry = retry, pending
		if len(pending) == 0 {
			return result
		}
		select {
		case <-ctx.Done():
			return errors.Join(result, ctx.Err())
		case <-ticker.C:
		}
	}
	return result
}

// isRuntimeStoppingError 判断 Runtime 或目标 Service 已不能继续接纳断线通知。
func isRuntimeStoppingError(err error) bool {
	return errors.Is(err, runtime.ErrServiceNotFound) ||
		errors.Is(err, runtime.ErrServiceClosed) ||
		errors.Is(err, runtime.ErrServiceStopping) ||
		errors.Is(err, runtime.ErrRuntimeStopping) ||
		errors.Is(err, runtime.ErrRuntimeStopped) ||
		errors.Is(err, runtime.ErrRuntimeClosed)
}

func isDisconnectRetryableError(err error) bool {
	return errors.Is(err, runtime.ErrMailboxFull) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, runtime.ErrCallTimeout)
}
