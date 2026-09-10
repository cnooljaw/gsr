package cluster

import (
	"errors"
	"testing"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证零值容量使用有界默认值，且不改写调用方配置或补充名称解析器。
func TestListenConfigDefaults(t *testing.T) {
	input := ListenConfig{Address: "127.0.0.1:0"}
	got, err := input.normalized()
	if err != nil {
		t.Fatal(err)
	}
	if got.Address != input.Address || got.MaxConnections != 128 || got.MaxConcurrentCalls != 32 || got.ResolveService != nil {
		t.Fatalf("unexpected defaults: %+v", got)
	}
	if input.MaxConnections != 0 || input.MaxConcurrentCalls != 0 {
		t.Fatal("input config changed")
	}
}

// 验证显式容量与外层名称解析函数原样保留，不在校验阶段执行解析器。
func TestListenConfigExplicitValues(t *testing.T) {
	called := false
	input := ListenConfig{
		Address: "0.0.0.0:7000", MaxConnections: 10, MaxConcurrentCalls: 2,
		ResolveService: func(name ServiceName) (runtime.ServiceID, error) {
			called = true
			if name != "player" {
				t.Fatalf("name=%q", name)
			}
			return 7, nil
		},
	}
	got, err := input.normalized()
	if err != nil || called || got.MaxConnections != 10 || got.MaxConcurrentCalls != 2 || got.Address != input.Address {
		t.Fatalf("config=%+v err=%v called=%v", got, err, called)
	}
	id, err := got.ResolveService("player")
	if id != 7 || err != nil || !called {
		t.Fatalf("resolver id=%d err=%v called=%v", id, err, called)
	}
}

// 验证监听地址校验不做 DNS 或绑定，允许动态端口但拒绝缺失地址及非法容量。
func TestListenConfigValidation(t *testing.T) {
	for _, address := range []string{"127.0.0.1:0", "0.0.0.0:7000", "[::1]:7000", "hall.invalid:7000", "127.0.0.1:65535"} {
		t.Run(address, func(t *testing.T) {
			if _, err := (ListenConfig{Address: address}).normalized(); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, address := range []string{"", ":7000", "127.0.0.1", "127.0.0.1:", "127.0.0.1:-1", "127.0.0.1:65536", "127.0.0.1:http", "127.0.0.1:+7", "bad host:7000", "unix/path:7000"} {
		t.Run("invalid-"+address, func(t *testing.T) {
			if _, err := (ListenConfig{Address: address}).normalized(); !errors.Is(err, ErrInvalidListenConfig) {
				t.Fatalf("err=%v", err)
			}
		})
	}
	for _, config := range []ListenConfig{
		{Address: "127.0.0.1:7000", MaxConnections: -1},
		{Address: "127.0.0.1:7000", MaxConcurrentCalls: -1},
	} {
		if _, err := config.normalized(); !errors.Is(err, ErrInvalidListenConfig) {
			t.Fatalf("config=%+v err=%v", config, err)
		}
	}
	if validTargetAddress("127.0.0.1:0") {
		t.Fatal("outbound route accepted dynamic port")
	}
}
