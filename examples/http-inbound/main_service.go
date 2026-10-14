package httpinbound

import (
	"context"
	"net/http"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

// adminMainService 组装用户、汇总 Service 和 HTTP 入口，完成后自身退出。
type adminMainService struct {
	address string
}

var _ gsr.Service = (*adminMainService)(nil)

func (service *adminMainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	users, err := self.CreateService(ctx, newUserService())
	if err != nil {
		return err
	}
	if err := self.Node().RegisterService("users", users); err != nil {
		return err
	}
	dashboard, err := self.CreateService(ctx, newDashboardService("rooms-node"))
	if err != nil {
		return err
	}
	if err := self.Node().RegisterService("dashboard", dashboard); err != nil {
		return err
	}
	if err := self.Node().OpenHTTP(service.address, func(in *gsr.HTTPIngress) http.Handler {
		return newAdminMux(in)
	}); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (*adminMainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (*adminMainService) OnStop(_ context.Context) error {
	return nil
}
