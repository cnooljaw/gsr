package main

import (
	"context"
	"fmt"
	"io"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

// MainService 是普通应用入口，创建 Counter 并发起全部业务通信，不持有 Runtime。
type MainService struct {
	output   io.Writer
	finished context.CancelFunc // 有限演示通知宿主结束等待；普通服务端通常由进程信号取消。
}

var _ gsr.Service = (*MainService)(nil)

// OnStart 完成本例的一次性业务流程；Counter 创建后即可使用，不需要 Start Command。
func (service *MainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	const counterName = "counter"
	id, err := self.CreateService(ctx, newCounterService())
	if err != nil {
		return err
	}
	if err := self.Register(counterName, id); err != nil {
		return err
	}
	for _, amount := range []int{2, 3} {
		if err := self.SendName(counterName, addCommand, amount); err != nil {
			return err
		}
	}
	// Get 排在两次 Add 后；不用 sleep，也不直接读取 Counter 的状态。
	value, err := self.CallName(ctx, counterName, getCommand)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(service.output, "count =", value); err != nil {
		return err
	}
	if err := self.SendName(counterName, resetCommand); err != nil {
		return err
	}
	value, err = self.CallName(ctx, counterName, getCommand)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(service.output, "count =", value); err != nil {
		return err
	}
	self.Exit()
	return nil
}

// Handle 不定义业务命令；这个入口在初始化完成后即退出。
func (service *MainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

// OnStop 仅通知宿主演示已收尾，不关闭 Runtime，也不停止 Counter。
func (service *MainService) OnStop(_ context.Context) error {
	service.finished()
	return nil
}
