package httpinbound

import (
	"context"
	"time"

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
	banned map[uint64]bool
}

func (service *userService) OnStart(_ context.Context, _ runtime.ServiceContext) error {
	service.banned = make(map[uint64]bool)
	return nil
}

func (service *userService) Handle(_ runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if command != userGetCommand && command != userBanCommand {
		return nil, runtime.ErrUnknownCommand
	}
	if len(args) != 1 {
		return nil, runtime.ErrInvalidCommandArguments
	}
	id, ok := args[0].(uint64)
	if !ok || id == 0 {
		return nil, runtime.ErrInvalidCommandArguments
	}
	if command == userGetCommand {
		return userRecord{ID: id, Banned: service.banned[id]}, nil
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
	users      runtime.ServiceID
	network    *cluster.Cluster
	remoteNode cluster.NodeID
	client     *cluster.ClusterClient
}

func (service *dashboardService) OnStart(_ context.Context, self runtime.ServiceContext) error {
	client, err := service.network.Client(self.Self())
	if err != nil {
		return err
	}
	service.client = client
	return nil
}

func (service *dashboardService) Handle(self runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if command != dashboardCommand {
		return nil, runtime.ErrUnknownCommand
	}
	if len(args) != 1 {
		return nil, runtime.ErrInvalidCommandArguments
	}
	id, ok := args[0].(uint64)
	if !ok || id == 0 {
		return nil, runtime.ErrInvalidCommandArguments
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	userValue, err := self.Call(ctx, service.users, userGetCommand, id)
	if err != nil {
		return nil, err
	}
	user, ok := userValue.(userRecord)
	if !ok {
		return nil, runtime.ErrInvalidCommandArguments
	}
	roomValue, err := service.client.Call(ctx, service.remoteNode,
		cluster.ServiceTarget{ServiceName: "rooms"}, roomCountCommand)
	if err != nil {
		return nil, err
	}
	players, ok := roomValue.(int64)
	if !ok {
		return nil, runtime.ErrInvalidCommandArguments
	}
	return dashboardRecord{User: user, RoomPlayers: players}, nil
}

func (*dashboardService) OnStop(context.Context) error {
	return nil
}
