package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// 验证错误归一化安全处理带类型的 nil，并按 Class 与 Code 共同识别 panic。
func TestNormalizeCallErrorIdentity(t *testing.T) {
	var typedNil *StatusError
	domain := NewStatusError(ErrorClassDomain, ErrorCodeServicePanicked, "domain error")
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "nil"},
		{name: "typed nil", err: typedNil, want: ErrHandlerFailed},
		{name: "wrapped typed nil", err: fmt.Errorf("wrapped: %w", typedNil), want: ErrHandlerFailed},
		{name: "domain with same code", err: domain, want: domain},
		{name: "wrapped domain", err: fmt.Errorf("wrapped: %w", domain), want: domain},
		{name: "runtime panic", err: ErrServicePanicked, want: ErrServicePanicked},
		{name: "wrapped runtime panic", err: fmt.Errorf("private detail: %w", ErrServicePanicked), want: ErrServicePanicked},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := normalizeCallError(test.err)
			if !errors.Is(got, test.want) {
				t.Fatalf("normalized error = %v, want %v", got, test.want)
			}
		})
	}
}

// 验证错误的 Is/As/Unwrap 扩展 panic 只停止当前 Service，安全完成 Call 并留下诊断。
func TestHandlerErrorClassificationPanicContained(t *testing.T) {
	for _, hook := range []string{"Is", "As", "Unwrap"} {
		for _, call := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/Call=%t", hook, call), func(t *testing.T) {
				local := newCallTestRuntime(t, Config{})
				id, err := local.CreateService(context.Background(), &errorBoundaryService{err: classificationPanicError{hook: hook}})
				if err != nil {
					t.Fatal(err)
				}
				if call {
					_, err := local.Call(context.Background(), id, 1)
					if !errors.Is(err, ErrServicePanicked) || strings.Contains(err.Error(), "private") {
						t.Fatalf("Call error = %v", err)
					}
				} else if err := local.Send(id, 1); err != nil {
					t.Fatal(err)
				}
				event := awaitDiagnostic(t, local)
				if event.Code != ErrorCodeServicePanicked || event.Stack == "" {
					t.Fatalf("diagnostic = %+v", event)
				}
				waitExitedService(t, local, id)
				healthyID, err := local.CreateService(context.Background(), &errorBoundaryService{})
				if err != nil {
					t.Fatal(err)
				}
				if value, err := local.Call(context.Background(), healthyID, 2); err != nil || value != "alive" {
					t.Fatalf("healthy Call = %v, %v", value, err)
				}
				inspection := local.Inspect()
				if inspection.Metrics.Panics != 1 || inspection.Metrics.HandlerErrors != 1 || inspection.PendingCallCount != 0 {
					t.Fatalf("inspection = %+v", inspection)
				}
			})
		}
	}
}

type classificationPanicError struct{ hook string }

func (failure classificationPanicError) Error() string {
	return "private business error"
}

func (failure classificationPanicError) Is(error) bool {
	if failure.hook == "Is" {
		panic("private Is panic")
	}
	return false
}

func (failure classificationPanicError) As(any) bool {
	if failure.hook == "As" {
		panic("private As panic")
	}
	return false
}

func (failure classificationPanicError) Unwrap() error {
	if failure.hook == "Unwrap" {
		panic("private Unwrap panic")
	}
	return nil
}

// 验证 Send 与 Call 的特殊错误不会误停 Service，后续消息仍执行且 panic 计数不增加。
func TestHandlerErrorBoundaryKeepsServiceRunning(t *testing.T) {
	var typedNil *StatusError
	domain := NewStatusError(ErrorClassDomain, ErrorCodeServicePanicked, "domain error")
	for _, test := range []struct {
		name string
		err  error
		want error
	}{
		{name: "typed nil", err: typedNil, want: ErrHandlerFailed},
		{name: "wrapped typed nil", err: fmt.Errorf("wrapped: %w", typedNil), want: ErrHandlerFailed},
		{name: "domain with same code", err: domain, want: domain},
		{name: "wrapped domain", err: fmt.Errorf("wrapped: %w", domain), want: domain},
	} {
		t.Run(test.name, func(t *testing.T) {
			runtime := newCallTestRuntime(t, Config{})
			id, err := runtime.CreateService(context.Background(), &errorBoundaryService{err: test.err})
			if err != nil {
				t.Fatal(err)
			}
			if err := runtime.Send(id, 1); err != nil {
				t.Fatal(err)
			}
			if _, err := runtime.Call(context.Background(), id, 1); !errors.Is(err, test.want) {
				t.Fatalf("Call error = %v, want %v", err, test.want)
			}
			// 同一 Mailbox 的后续 Call 是前两条消息完成诊断处理的屏障。
			value, err := runtime.Call(context.Background(), id, 2)
			if err != nil || value != "alive" {
				t.Fatalf("subsequent Call = %v, %v", value, err)
			}
			inspection := runtime.Inspect()
			if inspection.Metrics.HandlerErrors != 2 || inspection.Metrics.Panics != 0 || inspection.PendingCallCount != 0 {
				t.Fatalf("inspection = %+v", inspection)
			}
			if len(inspection.Services) != 1 || inspection.Services[0].State != ServiceStateRunning {
				t.Fatalf("Service no longer running: %+v", inspection.Services)
			}
		})
	}
}

type errorBoundaryService struct {
	err error
}

func (*errorBoundaryService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *errorBoundaryService) Handle(_ CommandContext, command CommandID, _ ...any) (any, error) {
	if command == 1 {
		return nil, service.err
	}
	return "alive", nil
}

func (*errorBoundaryService) OnStop(context.Context) error {
	return nil
}
