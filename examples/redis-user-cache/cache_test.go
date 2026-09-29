package redisusercache

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// TestUserHashCacheRoundTrip 验证用户信息按 Redis Hash 保存、替换旧字段并带有过期时间。
func TestUserHashCacheRoundTrip(t *testing.T) {
	client := startTestRedis(t)
	cache, err := newUserHashCache(client, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	want := userInfo{ID: 42, Name: "小明", Level: 7}
	if err := cache.Put(context.Background(), want); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	got, found, err := cache.Get(context.Background(), want.ID)
	if err != nil || !found || got != want {
		t.Fatalf("Get() = %+v, %t, %v; want %+v, true, nil", got, found, err, want)
	}
	key := userHashKey(want.ID)
	if keyType, err := client.Type(context.Background(), key).Result(); err != nil || keyType != "hash" {
		t.Fatalf("TYPE = %q, %v; want hash", keyType, err)
	}
	if ttl, err := client.TTL(context.Background(), key).Result(); err != nil || ttl <= 0 {
		t.Fatalf("TTL = %v, %v; want positive expiration", ttl, err)
	}
	if err := client.HSet(context.Background(), key, "stale", "old").Err(); err != nil {
		t.Fatalf("HSet(stale field) error = %v", err)
	}
	want.Name = "小华"
	if err := cache.Put(context.Background(), want); err != nil {
		t.Fatalf("Put(replace) error = %v", err)
	}
	fields, err := client.HGetAll(context.Background(), key).Result()
	if err != nil || len(fields) != 2 || fields["name"] != "小华" || fields["level"] != "7" {
		t.Fatalf("HGetAll(after replace) = %v, %v; want only updated user fields", fields, err)
	}
	if _, found, err := cache.Get(context.Background(), 99); err != nil || found {
		t.Fatalf("Get(missing) = found %t, error %v; want miss", found, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, found, err := cache.Get(canceled, want.ID); !errors.Is(err, context.Canceled) || found {
		t.Fatalf("Get(canceled) = found %t, error %v; want context cancellation", found, err)
	}
}

// TestUserHashCacheRejectsCorruptHash 验证字段缺失或格式损坏不会被误判为正常缓存命中。
func TestUserHashCacheRejectsCorruptHash(t *testing.T) {
	client := startTestRedis(t)
	cache, err := newUserHashCache(client, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	for _, fields := range []map[string]any{
		{"name": "小明"},
		{"name": "小明", "level": "not-a-number"},
	} {
		for name, value := range fields {
			if err := client.HSet(context.Background(), userHashKey(42), name, value).Err(); err != nil {
				t.Fatalf("HSet(corrupt hash field) error = %v", err)
			}
		}
		if _, found, err := cache.Get(context.Background(), 42); !errors.Is(err, errInvalidUserHash) || found {
			t.Fatalf("Get(corrupt hash) = found %t, error %v; want invalid hash", found, err)
		}
	}
}

// TestUserHashCacheFailureIsNotMiss 验证 Redis 客户端故障返回错误，不与缓存未命中混淆。
func TestUserHashCacheFailureIsNotMiss(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0", ContextTimeoutEnabled: true})
	cache, err := newUserHashCache(client, time.Minute, time.Second)
	if err != nil {
		t.Fatalf("newUserHashCache() error = %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("Client.Close() error = %v", err)
	}
	if _, found, err := cache.Get(context.Background(), 42); err == nil || found {
		t.Fatalf("Get(closed client) = found %t, error %v; want Redis failure", found, err)
	}
}

// TestUserHashCacheRejectsInvalidConfig 验证缓存拒绝无客户端或无界等待、无过期时间的配置。
func TestUserHashCacheRejectsInvalidConfig(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0", ContextTimeoutEnabled: true})
	t.Cleanup(func() { _ = client.Close() })
	unboundedClient := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"})
	t.Cleanup(func() { _ = unboundedClient.Close() })
	for _, config := range []struct {
		client  *redis.Client
		ttl     time.Duration
		timeout time.Duration
	}{
		{nil, time.Minute, time.Second},
		{unboundedClient, time.Minute, time.Second},
		{client, 0, time.Second},
		{client, time.Minute, 0},
	} {
		if _, err := newUserHashCache(config.client, config.ttl, config.timeout); err == nil {
			t.Fatalf("newUserHashCache(%+v) error = nil, want invalid config", config)
		}
	}
}

func startTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	redisServer, err := exec.LookPath("redis-server")
	if err != nil {
		t.Skip("redis-server is not installed")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve Redis port: %v", err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release Redis port: %v", err)
	}
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("split Redis address: %v", err)
	}
	serverCtx, stopServer := context.WithCancel(context.Background())
	command := exec.CommandContext(serverCtx, redisServer,
		"--bind", "127.0.0.1",
		"--port", port,
		"--save", "",
		"--appendonly", "no",
		"--dir", t.TempDir(),
	)
	// Redis 8 在 macOS 上会拒绝宿主环境中的无效 locale；C 在各平台均可用。
	command.Env = append(os.Environ(), "LC_ALL=C")
	command.Stdout, command.Stderr = io.Discard, io.Discard
	if err := command.Start(); err != nil {
		stopServer()
		t.Fatalf("start redis-server: %v", err)
	}
	t.Cleanup(func() {
		stopServer()
		_ = command.Wait()
	})
	client := redis.NewClient(&redis.Options{
		Addr:                  address,
		Protocol:              2,
		DisableIdentity:       true,
		ContextTimeoutEnabled: true,
		MaxRetries:            -1,
		DialTimeout:           200 * time.Millisecond,
		ReadTimeout:           200 * time.Millisecond,
		WriteTimeout:          200 * time.Millisecond,
	})
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Errorf("Redis Client.Close() error = %v", err)
		}
	})
	deadline := time.Now().Add(3 * time.Second)
	for {
		probeCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		err := client.Ping(probeCtx).Err()
		cancel()
		if err == nil {
			return client
		}
		if time.Now().After(deadline) {
			t.Fatalf("redis-server did not become ready on port %s: %v", port, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
