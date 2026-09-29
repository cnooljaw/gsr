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
	cache *userHashCache
}

// OnStart 只校验注入的依赖；Redis Client 的生命周期归应用所有。
func (service *userProfileService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	if service.cache == nil {
		return errors.New("user profile service requires a cache")
	}
	return nil
}

// Handle 串行处理业务命令；Redis I/O 有缓存对象设置的有限等待时间。
func (service *userProfileService) Handle(_ gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	switch command {
	case cacheUserCommand:
		if len(args) != 1 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		user, ok := args[0].(userInfo)
		if !ok || user.ID == 0 || user.Name == "" || user.Level < 0 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		if err := service.cache.Put(context.Background(), user); err != nil {
			return nil, errUserCacheUnavailable
		}
		return nil, nil
	case loadUserCommand:
		if len(args) != 1 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		id, ok := args[0].(uint64)
		if !ok || id == 0 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		user, found, err := service.cache.Get(context.Background(), id)
		if errors.Is(err, errInvalidUserHash) {
			return nil, errUserCacheCorrupt
		}
		if err != nil {
			return nil, errUserCacheUnavailable
		}
		return userLookup{User: user, Found: found}, nil
	default:
		return nil, runtime.ErrUnknownCommand
	}
}

// OnStop 不关闭借用的 Redis Client；应用在 Runtime 收尾后关闭它。
func (service *userProfileService) OnStop(_ context.Context) error {
	return nil
}
