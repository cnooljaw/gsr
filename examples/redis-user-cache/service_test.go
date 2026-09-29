package redisusercache

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
	"github.com/redis/go-redis/v9"
)

// TestUserProfileServiceCallsRedis 验证 MainService 通过 Command 调用业务 Service，由后者直接读写 Redis 缓存。
func TestUserProfileServiceCallsRedis(t *testing.T) {
	client := startTestRedis(t)
	cache, err := newUserHashCache(client, time.Minute, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	user := userInfo{ID: 42, Name: "小明", Level: 7}
	err = runUserProfileScenario(cache, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		value, err := self.Call(ctx, id, loadUserCommand, user.ID)
		if err != nil || value != (userLookup{}) {
			return fmt.Errorf("Call(load missing) = %+v, %v; want cache miss", value, err)
		}
		if _, err := self.Call(ctx, id, cacheUserCommand, user); err != nil {
			return fmt.Errorf("Call(cache user): %w", err)
		}
		value, err = self.Call(ctx, id, loadUserCommand, user.ID)
		if err != nil || value != (userLookup{User: user, Found: true}) {
			return fmt.Errorf("Call(load cached) = %+v, %v; want cached user", value, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUserProfileServiceSeparatesRedisFailure 验证 Redis 故障不被当成未命中，且不会泄露底层错误文本。
func TestUserProfileServiceSeparatesRedisFailure(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0", ContextTimeoutEnabled: true})
	cache, err := newUserHashCache(client, time.Minute, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Client.Close() error = %v", err)
	}
	err = runUserProfileScenario(cache, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		value, callErr := self.Call(ctx, id, loadUserCommand, uint64(42))
		if value != nil || !errors.Is(callErr, errUserCacheUnavailable) {
			return fmt.Errorf("Call(load with failed Redis) = %v, %v; want cache unavailable", value, callErr)
		}
		if strings.Contains(callErr.Error(), "closed") {
			return fmt.Errorf("Call(load with failed Redis) exposed client state: %v", callErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUserProfileServiceRejectsCorruptHash 验证损坏的缓存字段返回稳定错误，不伪装成未命中。
func TestUserProfileServiceRejectsCorruptHash(t *testing.T) {
	client := startTestRedis(t)
	cache, err := newUserHashCache(client, time.Minute, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	if err := client.HSet(context.Background(), userHashKey(42), "name", "小明").Err(); err != nil {
		t.Fatalf("HSet(corrupt hash) error = %v", err)
	}
	err = runUserProfileScenario(cache, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		value, callErr := self.Call(ctx, id, loadUserCommand, uint64(42))
		if value != nil || !errors.Is(callErr, errUserCacheCorrupt) {
			return fmt.Errorf("Call(load corrupt hash) = %v, %v; want cache corrupt", value, callErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUserProfileServiceRejectsInvalidCommand 验证非法参数和未知命令在访问 Redis 前被拒绝。
func TestUserProfileServiceRejectsInvalidCommand(t *testing.T) {
	service := &userProfileService{}
	for _, check := range []struct {
		command runtime.CommandID
		args    []any
		want    error
	}{
		{cacheUserCommand, []any{"not a user"}, runtime.ErrInvalidCommandArguments},
		{loadUserCommand, []any{uint64(0)}, runtime.ErrInvalidCommandArguments},
		{runtime.CommandID(99), nil, runtime.ErrUnknownCommand},
	} {
		if _, err := service.Handle(nil, check.command, check.args...); !errors.Is(err, check.want) {
			t.Fatalf("Handle(%d, %v) error = %v; want %v", check.command, check.args, err, check.want)
		}
	}
}

func runUserProfileScenario(cache *userHashCache, check func(context.Context, gsr.ServiceContext, runtime.ServiceID) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	entry := &redisTestMainService{cache: cache, check: check, finished: cancel}
	return gsr.Run(ctx, gsr.Config{Logging: logging.Config{Writer: io.Discard}}, entry)
}

type redisTestMainService struct {
	cache    *userHashCache
	check    func(context.Context, gsr.ServiceContext, runtime.ServiceID) error
	finished context.CancelFunc
}

func (service *redisTestMainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	id, err := self.CreateService(ctx, &userProfileService{cache: service.cache})
	if err != nil {
		return err
	}
	if err := service.check(ctx, self, id); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (service *redisTestMainService) Handle(_ gsr.CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *redisTestMainService) OnStop(_ context.Context) error {
	service.finished()
	return nil
}
