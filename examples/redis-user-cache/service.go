package main

import (
	"context"
	"errors"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	cacheUserCommand runtime.CommandID = iota + 1
	loadUserCommand
)

var (
	errUserCacheUnavailable = runtime.NewStatusError(runtime.ErrorClassDomain, "user.cache_unavailable", "user cache unavailable")
	errUserCacheCorrupt     = runtime.NewStatusError(runtime.ErrorClassDomain, "user.cache_corrupt", "user cache is invalid")
)

// userLookup 把缓存未命中与 Redis 故障分开；未命中时 User 为零值。
type userLookup struct {
	User  userInfo
	Found bool
}

// userProfileService 处理用户资料业务命令，直接调用 Redis 缓存，而非另建 RedisService。
type userProfileService struct {
	dispatcher *gsr.Dispatcher
	cache      *userHashCache
}

func newUserProfileService(cache *userHashCache) *userProfileService {
	service := &userProfileService{cache: cache}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{
		cacheUserCommand: service.handleCache,
		loadUserCommand:  service.handleLoad,
	})
	return service
}

// Handle 是业务 Service 的命令入口，具体命令由 dispatcher 分发。
func (service *userProfileService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

// OnStart 只校验注入的依赖；Redis Client 的生命周期归应用所有。
func (service *userProfileService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	if service.cache == nil {
		return errors.New("user profile service requires a cache")
	}
	return nil
}

// handleCache 校验用户业务字段后写入缓存；命令参数类型由 Dispatcher 校验。
func (service *userProfileService) handleCache(_ gsr.CommandContext, user userInfo) error {
	if user.ID == 0 || user.Name == "" || user.Level < 0 {
		return runtime.ErrInvalidCommandArguments
	}
	if err := service.cache.Put(context.Background(), user); err != nil {
		return errUserCacheUnavailable
	}
	return nil
}

// handleLoad 将缓存未命中与损坏、不可用分开返回。
func (service *userProfileService) handleLoad(_ gsr.CommandContext, id uint64) (userLookup, error) {
	if id == 0 {
		return userLookup{}, runtime.ErrInvalidCommandArguments
	}
	user, found, err := service.cache.Get(context.Background(), id)
	if errors.Is(err, errInvalidUserHash) {
		return userLookup{}, errUserCacheCorrupt
	}
	if err != nil {
		return userLookup{}, errUserCacheUnavailable
	}
	return userLookup{User: user, Found: found}, nil
}

// OnStop 不关闭借用的 Redis Client；应用在 Runtime 收尾后关闭它。
func (service *userProfileService) OnStop(_ context.Context) error {
	return nil
}
