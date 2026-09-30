package gsr

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	dispatcherEchoCommand   runtime.CommandID = 1
	dispatcherNotifyCommand runtime.CommandID = 2
)

// TestDispatcherRoutesApplicationService 验证显式 Handle 委托强类型分发后仍正确处理 Call 和 Send。
func TestDispatcherRoutesApplicationService(t *testing.T) {
	service := newDispatcherTestService()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		caller := self.Self()
		id, err := self.CreateService(ctx, service)
		if err != nil {
			return err
		}
		result, err := self.Call(ctx, id, dispatcherEchoCommand, "hello")
		if err != nil || result != "hello" || service.lastSource != caller {
			return errors.New("dispatcher Call did not return the typed Handler result")
		}
		if err := self.Send(id, dispatcherNotifyCommand, "ready"); err != nil {
			return err
		}
		result, err = self.Call(ctx, id, dispatcherEchoCommand, "again")
		if err != nil || result != "again" || service.notification != "ready" {
			return errors.New("dispatcher did not preserve FIFO execution")
		}
		self.Exit()
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

// TestDispatcherHandlerPanicIsolation 验证强类型 Handler 经反射调用发生 panic 时，只终止目标 Service。
func TestDispatcherHandlerPanicIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	panicking := &dispatcherTestService{dispatcher: MustDispatcher(map[runtime.CommandID]any{
		dispatcherEchoCommand: func(CommandContext) (string, error) {
			panic("dispatcher handler panic")
		},
	})}
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		panickingID, err := self.CreateService(ctx, panicking)
		if err != nil {
			return err
		}
		healthyID, err := self.CreateService(ctx, newDispatcherTestService())
		if err != nil {
			return err
		}
		if _, err := self.Call(ctx, panickingID, dispatcherEchoCommand); !errors.Is(err, runtime.ErrServicePanicked) {
			return errors.New("panicking Handler did not return ErrServicePanicked")
		}
		result, err := self.Call(ctx, healthyID, dispatcherEchoCommand, "healthy")
		if err != nil || result != "healthy" {
			return errors.New("healthy Service stopped after another Handler panicked")
		}
		self.Exit()
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

// 验证 Dispatcher 在构造时拒绝非法签名，并在调用时拒绝未知命令或不匹配参数。
func TestDispatcherRejectsInvalidHandlersAndArguments(t *testing.T) {
	for _, handler := range []any{
		nil,
		"not a function",
		func(string) error { return nil },
		func(CommandContext) {},
		func(CommandContext) (string, string) { return "", "" },
		func(CommandContext, ...string) error { return nil },
	} {
		if _, err := NewDispatcher(map[runtime.CommandID]any{1: handler}); !errors.Is(err, runtime.ErrInvalidCommandHandler) {
			t.Fatalf("handler=%T error=%v, want ErrInvalidCommandHandler", handler, err)
		}
	}
	if _, err := NewDispatcher(map[runtime.CommandID]any{0: func(CommandContext) error { return nil }}); !errors.Is(err, runtime.ErrInvalidCommandHandler) {
		t.Fatalf("zero CommandID error=%v, want ErrInvalidCommandHandler", err)
	}
	called := 0
	dispatcher, err := NewDispatcher(map[runtime.CommandID]any{
		1: func(CommandContext, string, *int) error {
			called++
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]any{{}, {1, nil}, {"ok", 1}} {
		if _, err := dispatcher.Handle(dispatcherTestContext{}, 1, args...); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
			t.Fatalf("args=%v error=%v, want ErrInvalidCommandArguments", args, err)
		}
	}
	if _, err := dispatcher.Handle(nil, 1, "ok", nil); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
		t.Fatalf("nil context error=%v, want ErrInvalidCommandArguments", err)
	}
	var typedNilContext *dispatcherTestContext
	if _, err := dispatcher.Handle(typedNilContext, 1, "ok", nil); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
		t.Fatalf("typed nil context error=%v, want ErrInvalidCommandArguments", err)
	}
	if _, err := dispatcher.Handle(nil, 99); !errors.Is(err, runtime.ErrUnknownCommand) {
		t.Fatalf("unknown command error=%v, want ErrUnknownCommand", err)
	}
	if called != 0 {
		t.Fatalf("invalid messages called Handler %d times", called)
	}
}

// 验证分发器复制命令表，构造后外部修改不会改变已绑定的 Handler。
func TestDispatcherCopiesHandlers(t *testing.T) {
	handlers := map[runtime.CommandID]any{
		1: func(CommandContext) (string, error) { return "original", nil },
	}
	dispatcher, err := NewDispatcher(handlers)
	if err != nil {
		t.Fatal(err)
	}
	handlers[1] = func(CommandContext) (string, error) { return "changed", nil }
	result, err := dispatcher.Handle(dispatcherTestContext{}, 1)
	if err != nil || result != "original" {
		t.Fatalf("result=%v error=%v, want original", result, err)
	}
}

// 验证 error-only Handler 的成功值为空，业务错误保持可识别，nil 引用参数可按签名绑定。
func TestDispatcherPreservesHandlerResults(t *testing.T) {
	failure := errors.New("business failure")
	dispatcher := MustDispatcher(map[runtime.CommandID]any{
		1: func(_ CommandContext, value *int) error {
			if value != nil {
				return failure
			}
			return nil
		},
	})
	current := dispatcherTestContext{}
	result, err := dispatcher.Handle(current, 1, nil)
	if err != nil || result != nil {
		t.Fatalf("nil argument result=%v error=%v", result, err)
	}
	value := 1
	result, err = dispatcher.Handle(current, 1, &value)
	if result != nil || !errors.Is(err, failure) {
		t.Fatalf("business failure result=%v error=%v", result, err)
	}
}

// TestDispatcherFallbackFromCustomHandle 验证 Service 只对热点命令直调强类型 Handler，其余仍交给 Dispatcher。
func TestDispatcherFallbackFromCustomHandle(t *testing.T) {
	service := &dispatcherOverrideService{}
	service.dispatcher = MustDispatcher(map[runtime.CommandID]any{
		dispatcherEchoCommand: func(_ CommandContext, text string) (string, error) {
			return text, nil
		},
	})
	current := dispatcherTestContext{}
	result, err := service.Handle(current, dispatcherHotCommand, 7)
	if err != nil || result != 14 {
		t.Fatalf("hot Handle result=%v error=%v", result, err)
	}
	for _, args := range [][]any{nil, {"not an int"}} {
		if _, err := service.Handle(current, dispatcherHotCommand, args...); !errors.Is(err, runtime.ErrInvalidCommandArguments) {
			t.Fatalf("hot Handle(%v) error=%v, want ErrInvalidCommandArguments", args, err)
		}
	}
	result, err = service.Handle(current, dispatcherEchoCommand, "ordinary")
	if err != nil || result != "ordinary" {
		t.Fatalf("fallback result=%v error=%v", result, err)
	}
	if _, err := service.Handle(current, 99); !errors.Is(err, runtime.ErrUnknownCommand) {
		t.Fatalf("unknown command error=%v, want ErrUnknownCommand", err)
	}
}

// 验证 MustDispatcher 只在静态命令定义无效时 panic。
func TestMustDispatcherPanicsOnInvalidDefinition(t *testing.T) {
	defer func() {
		recovered := recover()
		err, ok := recovered.(error)
		if !ok || !errors.Is(err, runtime.ErrInvalidCommandHandler) {
			t.Fatalf("panic=%v, want ErrInvalidCommandHandler", recovered)
		}
	}()
	MustDispatcher(map[runtime.CommandID]any{1: "invalid"})
}

// BenchmarkDispatcher 测量已构造的强类型命令表每条消息的分发和参数绑定开销。
func BenchmarkDispatcher(b *testing.B) {
	dispatcher := MustDispatcher(map[runtime.CommandID]any{
		1: func(_ CommandContext, value int) (int, error) { return value * 2, nil },
	})
	current := dispatcherTestContext{}
	b.ReportAllocs()
	for range b.N {
		result, err := dispatcher.Handle(current, 1, 1)
		if err != nil {
			b.Fatal(err)
		}
		dispatcherBenchmarkResult = result
	}
}

// BenchmarkDispatcherHotOverride 测量同一 Service 手写热点分支的局部分发开销。
func BenchmarkDispatcherHotOverride(b *testing.B) {
	service := &dispatcherOverrideService{dispatcher: MustDispatcher(nil)}
	current := dispatcherTestContext{}
	b.ReportAllocs()
	for range b.N {
		result, err := service.Handle(current, dispatcherHotCommand, 1)
		if err != nil {
			b.Fatal(err)
		}
		dispatcherBenchmarkResult = result
	}
}

var dispatcherBenchmarkResult any

type dispatcherTestContext struct{ CommandContext }

const dispatcherHotCommand runtime.CommandID = 3

type dispatcherOverrideService struct{ dispatcher *Dispatcher }

func (service *dispatcherOverrideService) Handle(ctx CommandContext, command runtime.CommandID, args ...any) (any, error) {
	if command == dispatcherHotCommand {
		if len(args) != 1 {
			return nil, runtime.ErrInvalidCommandArguments
		}
		value, ok := args[0].(int)
		if !ok {
			return nil, runtime.ErrInvalidCommandArguments
		}
		return service.handleHot(ctx, value)
	}
	return service.dispatcher.Handle(ctx, command, args...)
}

func (service *dispatcherOverrideService) handleHot(_ CommandContext, value int) (int, error) {
	return value * 2, nil
}

type dispatcherTestService struct {
	dispatcher   *Dispatcher
	notification string
	lastSource   runtime.ServiceID
}

func newDispatcherTestService() *dispatcherTestService {
	service := &dispatcherTestService{}
	service.dispatcher = MustDispatcher(map[runtime.CommandID]any{
		dispatcherEchoCommand:   service.echo,
		dispatcherNotifyCommand: service.notify,
	})
	return service
}

func (service *dispatcherTestService) Handle(ctx CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

func (*dispatcherTestService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (*dispatcherTestService) OnStop(context.Context) error {
	return nil
}

func (service *dispatcherTestService) echo(ctx CommandContext, text string) (string, error) {
	service.lastSource = ctx.Source()
	return text, nil
}

func (service *dispatcherTestService) notify(_ CommandContext, message string) error {
	service.notification = message
	return nil
}
