// Package gatecommon 保存两种示例 Gate 共享的连接生命周期流程。
package gatecommon

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	gatecontract "github.com/cnooljaw/gsr/gate"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	disconnectRetryInterval = 5 * time.Millisecond
	disconnectAttemptWait   = 100 * time.Millisecond
)

// CreateAgent 捕获业务构造回调的 panic，并拒绝空 Agent。
func CreateAgent(factory func(gatecontract.ClientSession) runtime.Service, session gatecontract.ClientSession, component string) (service runtime.Service, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			service = nil
			err = fmt.Errorf("%s: Agent constructor panicked: %v", component, recovered)
		}
	}()
	service = factory(session)
	if service == nil {
		return nil, fmt.Errorf("%s: Agent constructor returned nil", component)
	}
	return service, nil
}

// RetryDisconnect 在正常运行时重试 Gate runner 的断线投递；停机后交给 OnStop 补发。
func RetryDisconnect(local *runtime.Runtime, serviceID runtime.ServiceID, command runtime.CommandID, connection any, stopping func() bool) {
	ticker := time.NewTicker(disconnectRetryInterval)
	defer ticker.Stop()
	for {
		if stopping() {
			return
		}
		attemptContext, cancel := context.WithTimeout(context.Background(), disconnectAttemptWait)
		_, err := local.Call(attemptContext, serviceID, command, connection)
		cancel()
		if err == nil || IsRuntimeStoppingError(err) || !isDisconnectRetryableError(err) {
			return
		}
		<-ticker.C
	}
}

// NotifyMappedAgentsOnStop 在停止期限内公平重试所有仍有连接映射的 Agent。
// pending 由调用方创建，排序和重试期间不再供调用方使用。
func NotifyMappedAgentsOnStop(ctx context.Context, local *runtime.Runtime, pending []runtime.ServiceID, component string) error {
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
			err := local.Send(agentID, gatecontract.ClientDisconnectedCommand)
			if err == nil || IsRuntimeStoppingError(err) {
				continue
			}
			if errors.Is(err, runtime.ErrMailboxFull) {
				retry = append(retry, agentID)
				continue
			}
			result = errors.Join(result, fmt.Errorf("%s: notify Agent %d on Gate stop: %w", component, agentID, err))
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

// IsRuntimeStoppingError 判断 Runtime 或目标 Service 已不能继续接纳断线通知。
func IsRuntimeStoppingError(err error) bool {
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
