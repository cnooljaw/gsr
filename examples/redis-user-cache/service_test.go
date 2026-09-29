package main

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

// TestSlowRedisDoesNotBlockOtherService 验证一个 Service 等待 Redis 时，另一个 Service 仍能及时回复。
func TestSlowRedisDoesNotBlockOtherService(t *testing.T) {
	admin := startTestRedis(t)
	options := *admin.Options()
	options.ReadTimeout = time.Second
	client := redis.NewClient(&options)
	t.Cleanup(func() { _ = client.Close() })
	started := make(chan struct{}, 1)
	completed := make(chan struct{})
	client.AddHook(redisCommandStartHook{started: started, completed: completed})
	cache, err := newUserHashCache(client, time.Minute, 900*time.Millisecond)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	// Redis 6.0 支持不带模式参数的 CLIENT PAUSE，命令会立即确认暂停请求。
	if err := admin.Do(context.Background(), "CLIENT", "PAUSE", "2000").Err(); err != nil {
		t.Fatalf("CLIENT PAUSE error = %v", err)
	}
	err = runUserProfileScenario(cache, func(ctx context.Context, self gsr.ServiceContext, userID runtime.ServiceID) error {
		otherID, err := self.CreateService(ctx, &redisIndependentService{})
		if err != nil {
			return fmt.Errorf("CreateService(independent): %w", err)
		}
		if err := self.Send(userID, loadUserCommand, uint64(42)); err != nil {
			return fmt.Errorf("Send(load user): %w", err)
		}
		select {
		case <-started:
		case <-ctx.Done():
			return fmt.Errorf("wait for Redis read: %w", ctx.Err())
		}
		quick, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
		defer cancel()
		value, err := self.Call(quick, otherID, redisIndependentCommand)
		if err != nil || value != "ready" {
			return fmt.Errorf("Call(independent) = %v, %v; want ready while Redis is paused", value, err)
		}
		select {
		case <-completed:
			return errors.New("Redis read finished before the independent Service replied")
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestUserProfileServiceRecoversAfterRedisTimeout 验证 Redis 超时只使当前请求失败，恢复后同一 Service 仍能读取缓存。
func TestUserProfileServiceRecoversAfterRedisTimeout(t *testing.T) {
	client := startTestRedis(t)
	cache, err := newUserHashCache(client, time.Minute, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	user := userInfo{ID: 42, Name: "小明", Level: 7}
	if err := cache.Put(context.Background(), user); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := client.Do(context.Background(), "CLIENT", "PAUSE", "1500").Err(); err != nil {
		t.Fatalf("CLIENT PAUSE error = %v", err)
	}
	err = runUserProfileScenario(cache, func(ctx context.Context, self gsr.ServiceContext, id runtime.ServiceID) error {
		value, callErr := self.Call(ctx, id, loadUserCommand, user.ID)
		if value != nil || !errors.Is(callErr, errUserCacheUnavailable) {
			return fmt.Errorf("Call(load during pause) = %v, %v; want cache unavailable", value, callErr)
		}
		for {
			if err := client.Ping(ctx).Err(); err == nil {
				break
			}
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("wait for Redis recovery: %w", err)
			}
			time.Sleep(20 * time.Millisecond)
		}
		value, callErr = self.Call(ctx, id, loadUserCommand, user.ID)
		if callErr != nil || value != (userLookup{User: user, Found: true}) {
			return fmt.Errorf("Call(load after recovery) = %+v, %v; want cached user", value, callErr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type redisCommandStartHook struct {
	started   chan<- struct{}
	completed chan<- struct{}
}

func (hook redisCommandStartHook) DialHook(next redis.DialHook) redis.DialHook {
	return next
}

func (hook redisCommandStartHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, command redis.Cmder) error {
		if command.Name() == "hgetall" {
			defer close(hook.completed)
			select {
			case hook.started <- struct{}{}:
			default:
			}
		}
		return next(ctx, command)
	}
}

func (hook redisCommandStartHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return next
}

const redisIndependentCommand runtime.CommandID = 1

type redisIndependentService struct{}

func (service *redisIndependentService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *redisIndependentService) Handle(_ gsr.CommandContext, command runtime.CommandID, _ ...any) (any, error) {
	if command != redisIndependentCommand {
		return nil, runtime.ErrUnknownCommand
	}
	return "ready", nil
}

func (service *redisIndependentService) OnStop(_ context.Context) error {
	return nil
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
