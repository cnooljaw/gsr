package gsr

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证应用上下文拒绝无效子实例并隔离启动错误或 panic，后续创建与调用仍正常且来源不串。
func TestServiceChildStartupFailureIsolation(t *testing.T) {
	startFailure := errors.New("child startup failed")
	for _, mode := range []string{"error", "panic", "typed_nil"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			var failedStops, healthyStops atomic.Int32
			var failedSelf runtime.ServiceID
			wantError := startFailure
			failed := &probeService{
				start: func(_ context.Context, self ServiceContext) error {
					failedSelf = self.Self()
					if mode == "panic" {
						panic("child startup panic")
					}
					return startFailure
				},
				stop: func(_ context.Context) error {
					failedStops.Add(1)
					return nil
				},
			}
			if mode == "panic" {
				wantError = runtime.ErrServicePanicked
			}
			if mode == "typed_nil" {
				failed = nil
				wantError = runtime.ErrInvalidService
			}
			called := false
			entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
				id, err := self.CreateService(ctx, failed)
				if id != 0 || !errors.Is(err, wantError) {
					return errors.New("child startup failure was not preserved")
				}
				parentID := self.Self()
				id, err = self.CreateService(ctx, &probeService{
					handle: func(current CommandContext) (any, error) {
						if current.Source() != parentID {
							return nil, errors.New("healthy child received wrong source")
						}
						return current.Self(), nil
					},
					stop: func(_ context.Context) error {
						healthyStops.Add(1)
						return nil
					},
				})
				if err != nil {
					return err
				}
				if id == 0 || id == failedSelf {
					return errors.New("healthy child reused failed identity")
				}
				value, err := self.Call(ctx, id, 1)
				if err != nil {
					return err
				}
				if value != id {
					return errors.New("healthy child call returned wrong identity")
				}
				called = true
				cancel()
				return nil
			}}
			if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
				t.Fatal(err)
			}
			wantStops := int32(1)
			if mode == "typed_nil" {
				wantStops = 0
			}
			if !called || failedStops.Load() != wantStops || healthyStops.Load() != 1 {
				t.Fatalf("called=%v failed stops=%d healthy stops=%d", called, failedStops.Load(), healthyStops.Load())
			}
		})
	}
}

// 验证框架自动绑定各 Service 自身身份，转调时不继承收到消息的 Source。
func TestServiceClusterBindingUsesSelf(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		for i := 0; i < 2; i++ {
			id, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
				if current.Source() == current.Self() {
					return nil, errors.New("unexpected self call")
				}
				if current.Cluster().client.Source() != current.Self() {
					return nil, errors.New("source binding incorrect")
				}
				return current.Self(), nil
			}})
			if err != nil {
				return err
			}
			value, err := self.Call(ctx, id, 1)
			if err != nil {
				return err
			}
			if value != id {
				return errors.New("child identity incorrect")
			}
		}
		self.Exit()
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

// 验证业务入口失败触发统一清理，OnStop 仅一次且原始错误仍可识别。
func TestRunStartupFailure(t *testing.T) {
	failure := errors.New("startup failed")
	stops := 0
	entry := &probeService{start: func(context.Context, ServiceContext) error { return failure }, stop: func(context.Context) error {
		stops++
		return nil
	}}
	if err := Run(context.Background(), Config{Logging: logging.Config{Writer: io.Discard}}, entry); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if stops != 1 {
		t.Fatalf("stops=%d", stops)
	}
}

type probeService struct {
	start  func(context.Context, ServiceContext) error
	handle func(CommandContext) (any, error)
	stop   func(context.Context) error
}

func (service *probeService) OnStart(ctx context.Context, self ServiceContext) error {
	if service.start != nil {
		return service.start(ctx, self)
	}
	return nil
}

func (service *probeService) Handle(ctx CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	if service.handle != nil {
		return service.handle(ctx)
	}
	return nil, nil
}

func (service *probeService) OnStop(ctx context.Context) error {
	if service.stop != nil {
		return service.stop(ctx)
	}
	return nil
}
