package gsr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证只有实现客户端处理接口的应用 Service 接纳客户端消息，并保留自身绑定的调用能力。
func TestApplicationClientMessageCapability(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	observations := make(chan clientMessageObservation, 1)
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		local := self.(*serviceContext).adapter.node.local
		plainID, err := self.CreateService(ctx, &probeService{})
		if err != nil {
			return err
		}
		if err := local.DeliverClientMessage(plainID, []byte("not supported")); !errors.Is(err, runtime.ErrClientMessageUnsupported) {
			return fmt.Errorf("plain Service accepted client message: %v", err)
		}
		echoID, err := self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
			return current.Source(), nil
		}})
		if err != nil {
			return err
		}
		if err := self.Node().RegisterService("echo", echoID); err != nil {
			return err
		}
		agentID, err := self.CreateService(ctx, &clientMessageProbeService{observations: observations})
		if err != nil {
			return err
		}
		if err := local.DeliverClientMessage(agentID, []byte("client envelope")); err != nil {
			return err
		}
		select {
		case observed := <-observations:
			if observed.envelope != "client envelope" || observed.source != 0 ||
				observed.self != agentID || observed.callSource != agentID || observed.err != nil {
				return fmt.Errorf("client observation = %+v", observed)
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		cancel()
		return nil
	}}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}}, entry); err != nil {
		t.Fatal(err)
	}
}

type clientMessageObservation struct {
	envelope   string
	source     runtime.ServiceID
	self       runtime.ServiceID
	callSource runtime.ServiceID
	err        error
}

type clientMessageProbeService struct {
	observations chan<- clientMessageObservation
}

var (
	_ Service              = (*clientMessageProbeService)(nil)
	_ ClientMessageHandler = (*clientMessageProbeService)(nil)
)

func (service *clientMessageProbeService) OnStart(context.Context, ServiceContext) error {
	return nil
}

func (service *clientMessageProbeService) Handle(CommandContext, runtime.CommandID, ...any) (any, error) {
	return nil, runtime.ErrUnknownCommand
}

func (service *clientMessageProbeService) HandleClientMessage(self CommandContext, envelope []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := self.CallName(ctx, "echo", 1)
	observation := clientMessageObservation{envelope: string(envelope), source: self.Source(), self: self.Self(), err: err}
	if err == nil {
		observation.callSource, _ = value.(runtime.ServiceID)
	}
	service.observations <- observation
	return err
}

func (service *clientMessageProbeService) OnStop(context.Context) error {
	return nil
}

// 验证 Timer 消息来源为零，但回调发起的新调用与日志仍使用当前 Service 身份。
func TestServiceTimerPreservesOwnIdentity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var output bytes.Buffer
	var target, owner runtime.ServiceID
	result := make(chan error, 1)
	entry := &probeService{
		start: func(ctx context.Context, self ServiceContext) error {
			owner = self.Self()
			var err error
			target, err = self.CreateService(ctx, &probeService{handle: func(current CommandContext) (any, error) {
				return current.Source(), nil
			}})
			if err != nil {
				return err
			}
			_, err = self.After(0, 1)
			return err
		},
		handle: func(current CommandContext) (any, error) {
			defer cancel()
			var err error
			if current.Source() != 0 || current.Self() != owner {
				err = errors.New("timer confused source and self")
			} else if current.Cluster().client.Source() != owner {
				err = errors.New("timer callback cluster client lost owner")
			} else {
				var source any
				source, err = current.Call(ctx, target, 1)
				if err == nil && source != owner {
					err = errors.New("timer callback forwarded zero source")
				}
			}
			err = errors.Join(err, current.Logger().Info("timer handled"))
			result <- err
			return nil, err
		},
	}
	if err := Run(ctx, Config{Logging: logging.Config{Writer: &output}}, entry); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("timer handler did not complete")
	}
	decoder := json.NewDecoder(&output)
	found := false
	for {
		var record struct {
			Service runtime.ServiceID `json:"service"`
			Message string            `json:"message"`
		}
		if err := decoder.Decode(&record); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if record.Message == "timer handled" {
			found = true
			if record.Service != owner || owner == 0 {
				t.Fatalf("log service=%d, want %d", record.Service, owner)
			}
		}
	}
	if !found {
		t.Fatal("timer log was not drained")
	}
}

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
