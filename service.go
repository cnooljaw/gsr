package gsr

import (
	"context"
	"net/http"
	"reflect"
	"time"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// Service 是应用的运行单元，所有业务状态由其串行 Handler 管理。
type Service interface {
	OnStart(context.Context, ServiceContext) error
	Handle(CommandContext, runtime.CommandID, ...any) (any, error)
	OnStop(context.Context) error
}

// ClientMessageHandler 是 Agent 等应用 Service 可选的客户端消息处理能力。
// 只有实现它的 Service 才能接收 Gate 经 Runtime 投递的完整 Envelope 字节。
// 客户端消息与普通 Command 共用该 Service 的 Mailbox，由 GSR runner 逐条执行。
type ClientMessageHandler interface {
	HandleClientMessage(CommandContext, []byte) error
}

// ServiceContext 组合本地能力与当前 Service 绑定的 Cluster、日志入口。
// 仅在当前生命周期回调或 Handler 内使用，不借给其它 Service 或异步任务。
type ServiceContext interface {
	Self() runtime.ServiceID
	CreateService(context.Context, Service) (runtime.ServiceID, error)
	Exit()
	Register(cluster.ServiceName, runtime.ServiceID) error
	Send(runtime.ServiceID, runtime.CommandID, ...any) error
	Call(context.Context, runtime.ServiceID, runtime.CommandID, ...any) (any, error)
	SendName(cluster.ServiceName, runtime.CommandID, ...any) error
	CallName(context.Context, cluster.ServiceName, runtime.CommandID, ...any) (any, error)
	OpenHTTP(string, func(*HTTPIngress) http.Handler) error
	OpenDebugGate(context.Context, DebugGateConfig) (string, error)
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
	id      runtime.ServiceID
	cluster *Cluster
	logger  *logging.Logger
}

type clientMessageAdapter struct {
	*serviceAdapter
	handler ClientMessageHandler
}

func (node *application) wrap(service Service) (runtime.Service, error) {
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
	adapter := &serviceAdapter{service: service, node: node}
	if handler, ok := service.(ClientMessageHandler); ok {
		return &clientMessageAdapter{serviceAdapter: adapter, handler: handler}, nil
	}
	return adapter, nil
}

func (adapter *serviceAdapter) OnStart(ctx context.Context, self runtime.ServiceContext) error {
	adapter.id = self.Self()
	adapter.node.activateID(adapter.id)
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
		serviceContext: &serviceContext{ServiceContext: ctx, adapter: adapter},
		source:         ctx.Source(),
	}, command, args...)
}

func (adapter *clientMessageAdapter) HandleClientMessage(ctx runtime.CommandContext, envelope []byte) error {
	return adapter.handler.HandleClientMessage(&commandContext{
		serviceContext: &serviceContext{ServiceContext: ctx, adapter: adapter.serviceAdapter},
		source:         ctx.Source(),
	}, envelope)
}

func (adapter *serviceAdapter) OnStop(ctx context.Context) error {
	adapter.node.unregisterID(adapter.id)
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

// Register 发布本节点 Service 名称，供本地和跨节点调用共同解析。
func (self *serviceContext) Register(name cluster.ServiceName, id runtime.ServiceID) error {
	return self.adapter.node.register(name, id)
}

// SendName 按本节点名称解析目标，随后沿用当前 Service 的本地 Send 路径。
func (self *serviceContext) SendName(name cluster.ServiceName, command runtime.CommandID, args ...any) error {
	id, err := self.adapter.node.resolve(name)
	if err != nil {
		return err
	}
	return self.ServiceContext.Send(id, command, args...)
}

// CallName 按本节点名称解析目标，随后沿用当前 Service 的本地 Call 路径。
func (self *serviceContext) CallName(ctx context.Context, name cluster.ServiceName, command runtime.CommandID, args ...any) (any, error) {
	if ctx == nil {
		return nil, runtime.ErrInvalidCallContext
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	id, err := self.adapter.node.resolve(name)
	if err != nil {
		return nil, err
	}
	return self.ServiceContext.Call(ctx, id, command, args...)
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
