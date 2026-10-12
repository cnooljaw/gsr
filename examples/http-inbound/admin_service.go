package httpinbound

import (
	"context"
	"time"

	gsr "github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	userGetCommand runtime.CommandID = 2 + iota
	userBanCommand
	dashboardCommand
)

type userRecord struct {
	ID     uint64 `json:"id"`
	Banned bool   `json:"banned"`
}

// userService 独占用户封禁状态，HTTP Handler 不直接读写它的 map。
type userService struct {
	dispatcher *gsr.Dispatcher
	banned     map[uint64]bool
}

var _ gsr.Service = (*userService)(nil)

func newUserService() *userService {
	service := &userService{banned: make(map[uint64]bool)}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		userGetCommand: service.get,
		userBanCommand: service.ban,
	})
	return service
}

func (service *userService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *userService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

func (service *userService) get(_ gsr.CommandContext, id uint64) (userRecord, error) {
	if id == 0 {
		return userRecord{}, runtime.ErrInvalidCommandArguments
	}
	return userRecord{ID: id, Banned: service.banned[id]}, nil
}

func (service *userService) ban(_ gsr.CommandContext, id uint64) (userRecord, error) {
	if id == 0 {
		return userRecord{}, runtime.ErrInvalidCommandArguments
	}
	service.banned[id] = true
	return userRecord{ID: id, Banned: true}, nil
}

func (*userService) OnStop(context.Context) error {
	return nil
}

type dashboardRecord struct {
	User        userRecord `json:"user"`
	RoomPlayers int64      `json:"room_players"`
}

// dashboardService 在自身 Handler 中汇总本地用户与远端房间数据。
type dashboardService struct {
	dispatcher *gsr.Dispatcher
	remoteNode cluster.NodeID
}

var _ gsr.Service = (*dashboardService)(nil)

func newDashboardService(remoteNode cluster.NodeID) *dashboardService {
	service := &dashboardService{remoteNode: remoteNode}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		dashboardCommand: service.dashboard,
	})
	return service
}

func (service *dashboardService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *dashboardService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

func (service *dashboardService) dashboard(self gsr.CommandContext, id uint64) (dashboardRecord, error) {
	if id == 0 {
		return dashboardRecord{}, runtime.ErrInvalidCommandArguments
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	userValue, err := self.CallName(ctx, "users", userGetCommand, id)
	if err != nil {
		return dashboardRecord{}, err
	}
	user, ok := userValue.(userRecord)
	if !ok {
		return dashboardRecord{}, runtime.ErrInvalidCommandArguments
	}
	roomValue, err := self.Cluster().Call(ctx, service.remoteNode, "rooms", roomCountCommand)
	if err != nil {
		return dashboardRecord{}, err
	}
	players, ok := roomValue.(int64)
	if !ok {
		return dashboardRecord{}, runtime.ErrInvalidCommandArguments
	}
	return dashboardRecord{User: user, RoomPlayers: players}, nil
}

func (*dashboardService) OnStop(context.Context) error {
	return nil
}
