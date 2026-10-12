package httpinbound

import (
	"context"

	"github.com/cnooljaw/gsr/runtime"
)

const roomCountCommand runtime.CommandID = 1

// roomService 独占房间人数，HTTP Handler 只能通过 Command 读取。
type roomService struct {
	players int
}

var _ runtime.Service = (*roomService)(nil)

func (service *roomService) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	return nil
}

func (service *roomService) Handle(_ runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if command != roomCountCommand {
		return nil, runtime.ErrUnknownCommand
	}
	if len(args) != 0 {
		return nil, runtime.ErrInvalidCommandArguments
	}
	return service.players, nil
}

func (service *roomService) OnStop(_ context.Context) error {
	return nil
}
