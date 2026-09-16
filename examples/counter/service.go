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
	count int
}

func newCounterService() *CounterService {
	return &CounterService{}
}

// OnStart 无外部资源需要初始化。
func (service *CounterService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

// Handle 在目标 Service 判断命令及参数；非法请求不修改状态。
func (service *CounterService) Handle(_ gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	switch command {
	case addCommand:
		if len(args) != 1 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		amount, ok := args[0].(int)
		if !ok {
			return nil, runtime.ErrInvalidCommandArguments
		}
		service.count += amount
		return nil, nil
	case getCommand:
		if len(args) != 0 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		return service.count, nil
	case resetCommand:
		if len(args) != 0 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		service.count = 0
		return nil, nil
	default:
		return nil, runtime.ErrUnknownCommand
	}
}

// OnStop 无外部资源需要释放。
func (service *CounterService) OnStop(_ context.Context) error {
	return nil
}
