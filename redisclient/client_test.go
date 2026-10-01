package redisclient

import (
	"testing"
	"time"
)

// TestNewKeepsRedisOperationsBounded 验证默认客户端使用有限超时与 Redis 6 兼容协议。
func TestNewKeepsRedisOperationsBounded(t *testing.T) {
	client, err := New("127.0.0.1:6379")
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer client.Close()
	options := client.Options()
	if options.Protocol != 2 || !options.ContextTimeoutEnabled || !options.DisableIdentity || options.MaxRetries != 0 {
		t.Fatalf("New() options = %+v; want bounded Redis 6 compatible defaults", options)
	}
	if options.DialTimeout != time.Second || options.ReadTimeout != time.Second || options.WriteTimeout != time.Second {
		t.Fatalf("New() timeouts = %v, %v, %v; want one second", options.DialTimeout, options.ReadTimeout, options.WriteTimeout)
	}
	if _, err := New(" "); err == nil {
		t.Fatal("New(blank address) succeeded")
	}
}
