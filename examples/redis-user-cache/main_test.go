package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// TestRedisExampleRunsThroughService 验证可运行示例由 MainService 发起命令，业务 Service 将资料写入并读回 Redis。
func TestRedisExampleRunsThroughService(t *testing.T) {
	client := startTestRedis(t)
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := run(ctx, client.Options().Addr, &output); err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if got := output.String(); got != "user 42: 小明 (level 7)\n" {
		t.Fatalf("output = %q; want user profile", got)
	}
	fields, err := client.HGetAll(ctx, "gsr:example:user:42").Result()
	if err != nil || fields["name"] != "小明" || fields["level"] != "7" {
		t.Fatalf("HGetAll(example key) = %v, %v; want saved profile", fields, err)
	}
}

// TestRedisExampleReportsUnavailable 验证 Redis 不可用时可运行示例返回稳定错误，而不是输出伪造的缓存未命中。
func TestRedisExampleReportsUnavailable(t *testing.T) {
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := run(ctx, "127.0.0.1:0", &output); !errors.Is(err, errUserCacheUnavailable) {
		t.Fatalf("run(unavailable Redis) error = %v; want cache unavailable", err)
	}
	if output.Len() != 0 {
		t.Fatalf("output after Redis failure = %q; want empty", output.String())
	}
}
