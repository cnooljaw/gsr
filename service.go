package gsr

import (
	"context"
	"reflect"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// Service 是应用的运行单元，所有业务状态由其串行 Handler 管理。
type Service interface {
	OnStart(context.Context, ServiceContext) error
	Handle(CommandContext, runtime.CommandID, ...any) (any, error)
	OnStop(context.Context) error
}

// ServiceContext 组合本地能力与当前 Service 绑定的 Cluster、日志入口。
// 仅在当前生命周期回调或 Handler 内使用，不借给其它 Service 或异步任务。
type ServiceContext interface {
	Self() runtime.ServiceID
	CreateService(context.Context, Service) (runtime.ServiceID, error)
	Exit()
	Send(runtime.ServiceID, runtime.CommandID, ...any) error
	Call(context.Context, runtime.ServiceID, runtime.CommandID, ...any) (any, error)
	After(time.Duration, runtime.CommandID, ...any) (runtime.TimerID, error)
	CancelTimer(runtime.TimerID) bool
	Cluster() *Cluster
	Logger() *logging.Logger
}

// CommandContext 补充收到消息的来源；新调用的来源仍是 Self，不是上一跳 Source。
type CommandContext interface {
	ServiceContext
	Source() runtime.ServiceID
}

type serviceAdapter struct {
	service Service
	node    *application
	cluster *Cluster
	logger  *logging.Logger
}

func (node *application) wrap(service Service) (*serviceAdapter, error) {
	if service == nil {
		return nil, runtime.ErrInvalidService
	}
	value := reflect.ValueOf(service)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		if value.IsNil() {
			return nil, runtime.ErrInvalidService
		}
	}
	return &serviceAdapter{service: service, node: node}, nil
}

func (adapter *serviceAdapter) OnStart(ctx context.Context, self runtime.ServiceContext) error {
	client, err := adapter.node.cluster.Client(self.Self())
	if err != nil {
		return err
	}
	adapter.cluster = &Cluster{node: adapter.node, client: client}
	adapter.logger = adapter.node.logger.WithService(uint64(self.Self()))
	return adapter.service.OnStart(ctx, &serviceContext{ServiceContext: self, adapter: adapter})
}

func (adapter *serviceAdapter) Handle(ctx runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return adapter.service.Handle(&commandContext{
		serviceContext: &serviceContext{ServiceContext: ctx, adapter: adapter}, source: ctx.Source(),
	}, command, args...)
}

func (adapter *serviceAdapter) OnStop(ctx context.Context) error {
	return adapter.service.OnStop(ctx)
}

type serviceContext struct {
	runtime.ServiceContext
	adapter *serviceAdapter
}

func (self *serviceContext) CreateService(ctx context.Context, service Service) (runtime.ServiceID, error) {
	wrapped, err := self.adapter.node.wrap(service)
	if err != nil {
		return 0, err
	}
	return self.ServiceContext.CreateService(ctx, wrapped)
}

func (self *serviceContext) Cluster() *Cluster {
	return self.adapter.cluster
}

func (self *serviceContext) Logger() *logging.Logger {
	return self.adapter.logger
}

type commandContext struct {
	*serviceContext
	source runtime.ServiceID
}

func (ctx *commandContext) Source() runtime.ServiceID {
	return ctx.source
}
