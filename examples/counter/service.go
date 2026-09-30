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
	*gsr.Dispatcher
	count int
}

func newCounterService() *CounterService {
	service := &CounterService{}
	service.Dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		addCommand:   service.add,
		getCommand:   service.get,
		resetCommand: service.reset,
	})
	return service
}

// OnStart 无外部资源需要初始化。
func (service *CounterService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

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
