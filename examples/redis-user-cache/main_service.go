package main

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

// mainService 是普通业务入口，创建用户资料 Service 并通过 Command 调用它。
type mainService struct {
	cache    *userHashCache
	output   io.Writer
	finished context.CancelFunc
}

var _ gsr.Service = (*mainService)(nil)

// OnStart 演示一次缓存写入和读取；Redis I/O 在目标 Service 的 Handler 内完成。
func (service *mainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	id, err := self.CreateService(ctx, newUserProfileService(service.cache))
	if err != nil {
		return err
	}
	user := userInfo{ID: 42, Name: "小明", Level: 7}
	if _, err := self.Call(ctx, id, cacheUserCommand, user); err != nil {
		return err
	}
	value, err := self.Call(ctx, id, loadUserCommand, user.ID)
	if err != nil {
		return err
	}
	lookup, ok := value.(userLookup)
	if !ok || !lookup.Found {
		return errors.New("example did not find the cached user")
	}
	if _, err := fmt.Fprintf(service.output, "user %d: %s (level %d)\n", lookup.User.ID, lookup.User.Name, lookup.User.Level); err != nil {
		return err
	}
	self.Exit()
	return nil
}

// Handle 不接收业务命令；示例入口完成创建和调用后退出。
func (service *mainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

// OnStop 通知宿主演示结束，不直接关闭 Runtime 或 Redis Client。
func (service *mainService) OnStop(_ context.Context) error {
	service.finished()
	return nil
}
