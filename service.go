package gsr

import (
	"context"
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
// 只有实现它的 Service 才能接收 Gate 经 Runtime 投递的不透明客户端字节消息。
// 客户端消息与普通 Command 共用该 Service 的 Mailbox，由 GSR runner 逐条执行。
type ClientMessageHandler interface {
	HandleClientMessage(CommandContext, []byte) error
}

// ServiceContext 组合当前 Service 的本地与来源绑定能力，以及所属节点的组装入口。
// 仅在当前生命周期回调或 Handler 内使用，不借给其它 Service 或异步任务。
type ServiceContext interface {
	Self() runtime.ServiceID
	// CreateService 将实例交给 Runtime；ctx 仅控制启动，创建者退出不停止新 Service。
	CreateService(context.Context, Service) (runtime.ServiceID, error)
	// Exit 请求当前回调返回后自身退出，不中断 Go 控制流，调用后应返回。
	Exit()
	// Send 成功仅表示消息被接纳，不代表目标已经执行。
	Send(runtime.ServiceID, runtime.CommandID, ...any) error
	// Call 有限等待目标结果，期间当前 Service 不处理下一条消息；取消等待不保证目标未执行。
	Call(context.Context, runtime.ServiceID, runtime.CommandID, ...any) (any, error)
	SendName(cluster.ServiceName, runtime.CommandID, ...any) error
	CallName(context.Context, cluster.ServiceName, runtime.CommandID, ...any) (any, error)
	After(time.Duration, runtime.CommandID, ...any) (runtime.TimerID, error)
	CancelTimer(runtime.TimerID) bool
	Node() *Node
	Cluster() *Cluster
	Logger() *logging.ServiceLogger
}

// CommandContext 补充本地消息来源；HTTP、Timer、跨节点等入站来源为零，不是通用身份凭据。
// 新调用的来源仍是 Self，不继承上一跳 Source。
type CommandContext interface {
	ServiceContext
	Source() runtime.ServiceID
}

type serviceAdapter struct {
	service Service
	node    *Node
	id      runtime.ServiceID
	cluster *Cluster
	logger  *logging.ServiceLogger
}

var _ runtime.Service = (*serviceAdapter)(nil)

type clientMessageAdapter struct {
	*serviceAdapter
	handler ClientMessageHandler
}

var (
	_ runtime.Service              = (*clientMessageAdapter)(nil)
	_ runtime.ClientMessageHandler = (*clientMessageAdapter)(nil)
)

func (node *Node) wrap(service Service) (runtime.Service, error) {
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
	adapter.cluster = &Cluster{client: client}
	adapter.logger = adapter.node.logger.WithService(uint64(self.Self()))
	return adapter.service.OnStart(ctx, &serviceContext{ServiceContext: self, adapter: adapter})
}

func (adapter *serviceAdapter) Handle(ctx runtime.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return adapter.service.Handle(&commandContext{
		serviceContext: &serviceContext{ServiceContext: ctx, adapter: adapter},
		source:         ctx.Source(),
	}, command, args...)
}

func (adapter *clientMessageAdapter) HandleClientMessage(ctx runtime.CommandContext, frame []byte) error {
	return adapter.handler.HandleClientMessage(&commandContext{
		serviceContext: &serviceContext{ServiceContext: ctx, adapter: adapter.serviceAdapter},
		source:         ctx.Source(),
	}, frame)
}

func (adapter *serviceAdapter) OnStop(ctx context.Context) error {
	adapter.node.unregisterID(adapter.id)
	return adapter.service.OnStop(ctx)
}

type serviceContext struct {
	runtime.ServiceContext
	adapter *serviceAdapter
}

var _ ServiceContext = (*serviceContext)(nil)

func (self *serviceContext) CreateService(ctx context.Context, service Service) (runtime.ServiceID, error) {
	wrapped, err := self.adapter.node.wrap(service)
	if err != nil {
		return 0, err
	}
	return self.ServiceContext.CreateService(ctx, wrapped)
}

// Node 返回所属 Run 的唯一节点对象，业务只借用其组装能力，不接管生命周期。
func (self *serviceContext) Node() *Node {
	return self.adapter.node
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

func (self *serviceContext) Logger() *logging.ServiceLogger {
	return self.adapter.logger
}

type commandContext struct {
	*serviceContext
	source runtime.ServiceID
}

var _ CommandContext = (*commandContext)(nil)

func (ctx *commandContext) Source() runtime.ServiceID {
	return ctx.source
}
