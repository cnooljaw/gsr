package gsr

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

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
