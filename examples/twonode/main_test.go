package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证示例配置拒绝未知字段、无效角色和缺失目标，调用端无需监听地址。
func TestDecodeConfig(t *testing.T) {
	valid := `{"role":"caller","log_directory":"./log/caller","target":"b","nodes":{"b":"127.0.0.1:19091"}}`
	if _, err := decodeConfig(strings.NewReader(valid)); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{}`, `{"role":"other"}`, valid + ` {}`, strings.Replace(valid, `"role"`, `"typo"`, 1)} {
		if _, err := decodeConfig(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid config: %s", input)
		}
	}
}

// 验证入口只负责创建 Echo，入口退出后 Echo 仍可通过公开 Runtime 基础设施入口接收请求。
func TestMainServiceCreatesEcho(t *testing.T) {
	logger, err := logging.New(logging.Config{Directory: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := logger.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	local, err := runtime.NewRuntime(runtime.Config{Logger: logger})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := local.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := local.Start(); err != nil {
		t.Fatal(err)
	}
	entry := &mainService{cfg: config{Role: "server"}, log: logger}
	id, err := local.CreateService(context.Background(), entry)
	if err != nil {
		t.Fatal(err)
	}
	if entry.echoID == 0 || entry.echoID == id {
		t.Fatalf("entry=%d echo=%d", id, entry.echoID)
	}
	if err := local.Send(id, echoCommand); !errors.Is(err, runtime.ErrServiceStopping) && !errors.Is(err, runtime.ErrServiceClosed) {
		t.Fatalf("entry still accepting messages: %v", err)
	}
	// 此处模拟 Cluster 入站桥接，业务代码不使用 Runtime.Call。
	value, err := local.Call(context.Background(), entry.echoID, echoCommand, "hello")
	if err != nil {
		t.Fatal(err)
	}
	args, ok := value.([]any)
	if !ok || len(args) != 1 || args[0] != "hello" {
		t.Fatalf("echo result: %v", value)
	}
}
