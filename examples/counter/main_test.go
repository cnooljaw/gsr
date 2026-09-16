package main

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证示例通过 Send 加值、Call 查询，再清零并查询，不依赖等待或直接读取 Service 状态。
func TestCounterExample(t *testing.T) {
	var output bytes.Buffer
	if err := run(&output); err != nil {
		t.Fatal(err)
	}
	if output.String() != "count = 5\ncount = 0\n" {
		t.Fatalf("output=%q", output.String())
	}
}

// 验证非法命令与参数不会修改计数，错误之后合法调用仍然成功。
func TestCounterRejectsInvalidCommands(t *testing.T) {
	local, err := runtime.NewRuntime(runtime.Config{})
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	id, err := local.CreateService(ctx, newCounterService())
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range []struct {
		id   runtime.CommandID
		args []any
		want error
	}{
		{addCommand, []any{"bad"}, runtime.ErrInvalidCommandArguments},
		{getCommand, []any{1}, runtime.ErrInvalidCommandArguments},
		{resetCommand, []any{1}, runtime.ErrInvalidCommandArguments},
		{99, nil, runtime.ErrUnknownCommand},
	} {
		if _, err := local.Call(ctx, id, command.id, command.args...); !errors.Is(err, command.want) {
			t.Fatalf("command=%d error=%v", command.id, err)
		}
	}
	value, err := local.Call(ctx, id, getCommand)
	if err != nil || value != 0 {
		t.Fatalf("value=%v error=%v", value, err)
	}
}
