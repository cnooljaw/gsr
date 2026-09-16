package main

import (
	"context"
	"fmt"
	"io"

	"github.com/cnooljaw/gsr/runtime"
)

// MainService 是普通应用入口，创建 Counter 并发起全部业务通信，不持有 Runtime。
type MainService struct {
	output io.Writer
	done   chan struct{}
}

// OnStart 完成本例的一次性业务流程；Counter 创建后即可使用，不需要 Start Command。
func (service *MainService) OnStart(ctx context.Context, scoped runtime.ServiceContext) error {
	id, err := scoped.CreateService(ctx, newCounterService())
	if err != nil {
		return err
	}
	for _, amount := range []int{2, 3} {
		if err := scoped.Send(id, addCommand, amount); err != nil {
			return err
		}
	}
	// Get 排在两次 Add 后；不用 sleep，也不直接读取 Counter 的状态。
	value, err := scoped.Call(ctx, id, getCommand)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(service.output, "count =", value); err != nil {
		return err
	}
	if err := scoped.Send(id, resetCommand); err != nil {
		return err
	}
	value, err = scoped.Call(ctx, id, getCommand)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(service.output, "count =", value); err != nil {
		return err
	}
	scoped.Exit()
	return nil
}

// Handle 不定义业务命令；这个入口在初始化完成后即退出。
func (*MainService) Handle(runtime.CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

// OnStop 仅通知宿主演示已收尾，不关闭 Runtime，也不停止 Counter。
func (service *MainService) OnStop(context.Context) error {
	close(service.done)
	return nil
}
