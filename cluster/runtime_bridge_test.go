package cluster

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/cnooljaw/gsr/runtime"
)

// 验证入站 Cluster Call 通过真实 Runtime 的 Mailbox 执行，保留错误身份且 Source 为零。
func TestClusterCallBridgesToRuntime(t *testing.T) {
	domainErr := runtime.NewStatusError(runtime.ErrorClassDomain, "room.full", "room is full")
	for _, testCase := range []struct {
		name string
		err  error
	}{{name: "success"}, {name: "domain error", err: domainErr}} {
		t.Run(testCase.name, func(t *testing.T) {
			service := &runtimeBridgeService{handlerErr: testCase.err, sources: make(chan runtime.ServiceID, 2)}
			link := newRoundTripLink(t, service.Handle)
			// 先消耗本地 Session，避免偶然依赖本地与 Cluster Session 相等。
			if _, err := link.local.Call(context.Background(), link.serviceID, roundTripCommand); !errors.Is(err, testCase.err) {
				t.Fatal(err)
			}
			value, err := link.client.Call(context.Background(), "node-b", ServiceTarget{ServiceName: "room"}, roundTripCommand, "hello", int32(3))
			if !errors.Is(err, testCase.err) {
				t.Fatalf("Cluster Call error = %v", err)
			}
			if err == nil && !reflect.DeepEqual(value, []any{"hello", int64(3)}) {
				t.Fatalf("result = %#v", value)
			}
			for i := 0; i < 2; i++ {
				select {
				case source := <-service.sources:
					if source != 0 {
						t.Fatalf("Source = %d", source)
					}
				default:
					t.Fatal("missing Handler invocation")
				}
			}
		})
	}
}

type runtimeBridgeService struct {
	handlerErr error
	sources    chan runtime.ServiceID
}

func (*runtimeBridgeService) OnStart(context.Context, runtime.ServiceContext) error {
	return nil
}

func (service *runtimeBridgeService) Handle(ctx runtime.CommandContext, _ runtime.CommandID, args ...any) (any, error) {
	service.sources <- ctx.Source()
	return args, service.handlerErr
}

func (*runtimeBridgeService) OnStop(context.Context) error {
	return nil
}
