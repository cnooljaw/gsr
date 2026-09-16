package main

import (
	"bytes"
	"errors"
	"io"
	"testing"

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
	// 本测试独占未启动实例，只验证命令处理函数；真实消息路径由 TestCounterExample 覆盖。
	counter := newCounterService()
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
		if _, err := counter.Handle(nil, command.id, command.args...); !errors.Is(err, command.want) {
			t.Fatalf("command=%d error=%v", command.id, err)
		}
	}
	value, err := counter.Handle(nil, getCommand)
	if err != nil || value != 0 {
		t.Fatalf("value=%v error=%v", value, err)
	}
}

// 验证输出失败不会因为演示完成通知而丢失，Run 仍返回原始错误。
func TestCounterOutputFailure(t *testing.T) {
	if err := run(failingOutput{}); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("output failure=%v", err)
	}
}

type failingOutput struct{}

func (writer failingOutput) Write(_ []byte) (int, error) {
	return 0, io.ErrClosedPipe
}
