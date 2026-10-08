package main

import (
	"context"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	addCommand runtime.CommandID = iota + 1
	getCommand
	resetCommand
)

// CounterService 独占计数状态；只能通过 Command 读写，不需要业务锁或 goroutine。
type CounterService struct {
	dispatcher *gsr.Dispatcher
	count      int
}

func newCounterService() *CounterService {
	service := &CounterService{}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		addCommand:   service.add,
		getCommand:   service.get,
		resetCommand: service.reset,
	})
	return service
}

// Handle 是 CounterService 的命令入口，具体命令由 dispatcher 分发。
func (service *CounterService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

// OnStart 无外部资源需要初始化。
func (service *CounterService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

// add 只用于本地；若扩展为 Cluster 命令，应显式将边界参数和结果改为 int64。
func (service *CounterService) add(_ gsr.CommandContext, amount int) error {
	service.count += amount
	return nil
}

func (service *CounterService) get(_ gsr.CommandContext) (int, error) {
	return service.count, nil
}

func (service *CounterService) reset(_ gsr.CommandContext) error {
	service.count = 0
	return nil
}

// OnStop 无外部资源需要释放。
func (service *CounterService) OnStop(_ context.Context) error {
	return nil
}
