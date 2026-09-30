package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证配置拒绝未知字段、无效角色和缺失目标，接收端使用节点名称开放监听。
func TestDecodeConfig(t *testing.T) {
	caller := `{"role":"caller","log_directory":"./log/caller","target":"b","nodes":{"b":"127.0.0.1:19091"}}`
	server := `{"role":"server","log_directory":"./log/server","listen":"b","nodes":{"b":"127.0.0.1:19091"}}`
	for _, input := range []string{caller, server} {
		if _, err := decodeConfig(strings.NewReader(input)); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []string{`{}`, `{"role":"other"}`, caller + ` {}`, strings.Replace(caller, `"role"`, `"typo"`, 1), strings.Replace(server, `"listen":"b"`, `"listen":"missing"`, 1)} {
		if _, err := decodeConfig(strings.NewReader(input)); err == nil {
			t.Fatalf("accepted invalid config: %s", input)
		}
	}
}

// TestEchoServiceRejectsInvalidArguments 验证普通业务命令由强类型分发拒绝错误参数。
func TestEchoServiceRejectsInvalidArguments(t *testing.T) {
	service := newEchoService()
	current := echoTestCommandContext{}
	for _, check := range []struct {
		command runtime.CommandID
		args    []any
		want    error
	}{
		{notifyCommand, nil, runtime.ErrInvalidCommandArguments},
		{echoCommand, []any{"not an operation"}, runtime.ErrInvalidCommandArguments},
		{runtime.CommandID(99), nil, runtime.ErrUnknownCommand},
	} {
		if _, err := service.Handle(current, check.command, check.args...); !errors.Is(err, check.want) {
			t.Fatalf("Handle(%d, %v) error = %v; want %v", check.command, check.args, err, check.want)
		}
	}
}

// TestEchoServiceReturnsTypedResult 验证 Echo Call 返回强类型操作编号，Notify 不产生回复值。
func TestEchoServiceReturnsTypedResult(t *testing.T) {
	service := newEchoService()
	current := echoTestCommandContext{}
	value, err := service.Handle(current, echoCommand, uint64(42))
	if err != nil || value != uint64(42) {
		t.Fatalf("Handle(echo) = %v, %v; want operation 42", value, err)
	}
	value, err = service.Handle(current, notifyCommand, uint64(42))
	if err != nil || value != nil {
		t.Fatalf("Handle(notify) = %v, %v; want nil reply", value, err)
	}
}

// TestCallerServiceRejectsTickArguments 验证定时命令不接收多余参数。
func TestCallerServiceRejectsTickArguments(t *testing.T) {
	service := newCallerService("target")
	if _, err := service.Handle(echoTestCommandContext{}, tickCommand, 1); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
		t.Fatalf("Handle(tick, extra argument) error = %v; want ErrInvalidCommandArguments", err)
	}
}

type echoTestCommandContext struct{ gsr.CommandContext }

func (echoTestCommandContext) Logger() *logging.Logger {
	return nil
}
