package redisclient

import (
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// New 创建使用 Redis 6 兼容协议和有限 I/O 超时的客户端。
// New 不建立连接；返回的客户端由调用方在 Runtime 停止后关闭。
func New(address string) (*redis.Client, error) {
	if strings.TrimSpace(address) == "" {
		return nil, errors.New("redisclient: address is required")
	}
	return redis.NewClient(&redis.Options{
		Addr:                  address,
		Protocol:              2,
		DisableIdentity:       true,
		ContextTimeoutEnabled: true,
		MaxRetries:            -1,
		DialTimeout:           time.Second,
		ReadTimeout:           time.Second,
		WriteTimeout:          time.Second,
	}), nil
}
