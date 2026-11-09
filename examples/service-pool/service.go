package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/cnooljaw/gsr"
	"github.com/cnooljaw/gsr/runtime"
)

const (
	workCommand runtime.CommandID = iota + 1
	requestCommand
	completedCommand
)

const poolName = "workers"

type workResult struct {
	job       int
	worker    runtime.ServiceID
	completed int
}

type completion struct {
	work workResult
	err  error
}

// mainService 只收集结果；等待工作结果的责任归独立调用者。
type mainService struct {
	dispatcher *gsr.Dispatcher
	output     io.Writer
	finished   context.CancelFunc
	received   int
	result     error
}

var _ gsr.Service = (*mainService)(nil)

func newMainService(output io.Writer, finished context.CancelFunc) *mainService {
	service := &mainService{output: output, finished: finished}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{completedCommand: service.completed})
	return service
}

func (service *mainService) OnStart(ctx context.Context, self gsr.ServiceContext) error {
	err := self.Node().CreateServicePool(ctx, poolName, gsr.ServicePoolConfig{
		Size:            2,
		MailboxCapacity: 1,
		NewService:      func() gsr.Service { return newWorkerService() },
	})
	if err != nil {
		return err
	}
	for job := 1; job <= 2; job++ {
		id, err := self.CreateService(ctx, newCallerService())
		if err != nil {
			return err
		}
		if err := self.Send(id, requestCommand, job); err != nil {
			return err
		}
	}
	return nil
}

func (service *mainService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

func (service *mainService) completed(self gsr.CommandContext, reply completion) error {
	service.received++
	service.result = errors.Join(service.result, reply.err)
	if reply.err == nil {
		_, err := fmt.Fprintf(service.output, "job=%d worker=%d completed=%d\n", reply.work.job, reply.work.worker, reply.work.completed)
		service.result = errors.Join(service.result, err)
	}
	if service.received == 2 {
		self.Exit()
	}
	return nil
}

func (service *mainService) OnStop(_ context.Context) error {
	service.finished()
	return nil
}

// callerService 使用普通同步 Call；不同调用者可同时等待不同池成员。
type callerService struct {
	dispatcher *gsr.Dispatcher
}

var _ gsr.Service = (*callerService)(nil)

func newCallerService() *callerService {
	service := &callerService{}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{requestCommand: service.request})
	return service
}

func (service *callerService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *callerService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

func (service *callerService) request(self gsr.CommandContext, job int) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	value, err := self.CallName(ctx, poolName, workCommand, job)
	reply := completion{err: err}
	if err == nil {
		work, ok := value.(workResult)
		if !ok {
			reply.err = errors.New("invalid worker result")
		} else {
			reply.work = work
		}
	}
	// 发送到本次请求的实际发起者，不持有其它 Service 的实现指针。
	if err := self.Send(self.Source(), completedCommand, reply); err != nil {
		return err
	}
	self.Exit()
	return nil
}

func (service *callerService) OnStop(_ context.Context) error {
	return nil
}

// workerService 独占自己的计数，池成员之间不共享可变业务状态。
type workerService struct {
	dispatcher *gsr.Dispatcher
	completed  int
}

var _ gsr.Service = (*workerService)(nil)

func newWorkerService() *workerService {
	service := &workerService{}
	service.dispatcher = gsr.MustDispatcher(map[runtime.CommandID]any{workCommand: service.work})
	return service
}

func (service *workerService) OnStart(_ context.Context, _ gsr.ServiceContext) error {
	return nil
}

func (service *workerService) Handle(ctx gsr.CommandContext, command runtime.CommandID, args ...any) (any, error) {
	return service.dispatcher.Handle(ctx, command, args...)
}

func (service *workerService) work(self gsr.CommandContext, job int) (workResult, error) {
	// 仅在演示中模拟阻塞 I/O；实际业务直接调用自己的数据库等依赖。
	time.Sleep(50 * time.Millisecond)
	service.completed++
	return workResult{job: job, worker: self.Self(), completed: service.completed}, nil
}

func (service *workerService) OnStop(_ context.Context) error {
	return nil
}
