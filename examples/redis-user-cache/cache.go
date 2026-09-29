package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var errInvalidUserHash = errors.New("user cache contains an invalid hash")

// userInfo 是示例中的缓存快照，不代表权威用户资料。
type userInfo struct {
	ID    uint64
	Name  string
	Level int
}

// userHashCache 只封装本例的 Redis Hash 字段和 TTL，不拥有 Redis Client。
type userHashCache struct {
	client  *redis.Client
	ttl     time.Duration
	timeout time.Duration
}

func newUserHashCache(client *redis.Client, ttl, timeout time.Duration) (*userHashCache, error) {
	if client == nil || ttl <= 0 || timeout <= 0 {
		return nil, errors.New("user cache requires a client, positive TTL and finite timeout")
	}
	if !client.Options().ContextTimeoutEnabled {
		return nil, errors.New("user cache requires Redis context deadlines to be enabled")
	}
	return &userHashCache{client: client, ttl: ttl, timeout: timeout}, nil
}

func (cache *userHashCache) Put(ctx context.Context, user userInfo) error {
	if ctx == nil || user.ID == 0 || user.Name == "" || user.Level < 0 {
		return errors.New("user cache requires a valid context and user")
	}
	bounded, cancel := context.WithTimeout(ctx, cache.timeout)
	defer cancel()
	key := userHashKey(user.ID)
	_, err := cache.client.TxPipelined(bounded, func(pipe redis.Pipeliner) error {
		pipe.Del(bounded, key)
		pipe.HSet(bounded, key, "name", user.Name)
		pipe.HSet(bounded, key, "level", user.Level)
		pipe.Expire(bounded, key, cache.ttl)
		return nil
	})
	if err != nil {
		return fmt.Errorf("write user cache: %w", err)
	}
	return nil
}

func (cache *userHashCache) Get(ctx context.Context, id uint64) (userInfo, bool, error) {
	if ctx == nil || id == 0 {
		return userInfo{}, false, errors.New("user cache requires a valid context and ID")
	}
	bounded, cancel := context.WithTimeout(ctx, cache.timeout)
	defer cancel()
	fields, err := cache.client.HGetAll(bounded, userHashKey(id)).Result()
	if err != nil {
		return userInfo{}, false, fmt.Errorf("read user cache: %w", err)
	}
	if len(fields) == 0 {
		return userInfo{}, false, nil
	}
	name := fields["name"]
	levelText, exists := fields["level"]
	if name == "" || !exists {
		return userInfo{}, false, errInvalidUserHash
	}
	level, err := strconv.Atoi(levelText)
	if err != nil || level < 0 {
		return userInfo{}, false, errInvalidUserHash
	}
	return userInfo{ID: id, Name: name, Level: level}, true, nil
}

func userHashKey(id uint64) string {
	return "gsr:example:user:" + strconv.FormatUint(id, 10)
}
