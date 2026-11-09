package gsr

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/cnooljaw/gsr/cluster"
	"github.com/cnooljaw/gsr/runtime"
)

// ServicePoolConfig 定义固定并行数及每个独立成员的有界队列。
// 池不保证整体执行顺序；业务自行保证可并行性及共享依赖的并发安全。
type ServicePoolConfig struct {
	// Size 是成员数，必须为正，不自动扩容或补员。
	Size int
	// MailboxCapacity 是每成员等待消息容量，零值沿用 Runtime 默认，负值拒绝。
	MailboxCapacity int
	// NewService 只组装非 nil 指针，资源初始化放 OnStart；不得复用有状态实例，成员须实现相同命令契约。
	NewService func() Service
}

type servicePool struct {
	members []runtime.ServiceID
	next    int
	ready   bool
}

// CreateServicePool 创建一组独立串行 Service，完成后才发布逻辑名称。
// 名称路由轮询一次，满载或投递失败不重选、不重放，Call 使用现有自动 Reply 路径。
// 创建失败会撤销名称并有界回收已创建成员；创建者退出不影响成员，节点停止统一收尾。
func (node *Node) CreateServicePool(ctx context.Context, name cluster.ServiceName, config ServicePoolConfig) (result error) {
	if ctx == nil {
		return runtime.ErrInvalidCallContext
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if node == nil || node.local == nil {
		return runtime.ErrRuntimeClosed
	}
	if name == "" || config.Size <= 0 || config.MailboxCapacity < 0 || config.NewService == nil {
		return fmt.Errorf("%w: pool requires a name, positive Size, nonnegative MailboxCapacity and NewService", runtime.ErrInvalidConfig)
	}
	pool := &servicePool{}
	node.mu.Lock()
	if _, exists := node.names[name]; exists || node.pools[name] != nil {
		node.mu.Unlock()
		return ErrServiceNameRegistered
	}
	if node.pools == nil {
		node.pools = make(map[cluster.ServiceName]*servicePool)
	}
	node.pools[name] = pool
	node.mu.Unlock()
	created := make([]runtime.ServiceID, 0, config.Size)
	defer func() {
		if result == nil {
			return
		}
		node.mu.Lock()
		if node.pools[name] == pool {
			delete(node.pools, name)
		}
		// 失败池的成员不能在同名重建后撤销新名称。
		for _, id := range created {
			node.removeNameFromID(id, name)
		}
		node.mu.Unlock()
		cleanupErr := node.local.StopServices(context.Background(), created...)
		result = errors.Join(result, cleanupErr)
	}()
	services, err := constructPoolMembers(ctx, config)
	if err != nil {
		return err
	}
	for _, service := range services {
		if err := ctx.Err(); err != nil {
			return err
		}
		wrapped, err := node.wrap(service)
		if err != nil {
			return err
		}
		id, err := node.local.CreateServiceWithConfig(ctx, wrapped, runtime.ServiceConfig{MailboxCapacity: config.MailboxCapacity})
		if err != nil {
			return err
		}
		created = append(created, id)
		node.mu.Lock()
		_, active := node.active[id]
		if active {
			pool.members = append(pool.members, id)
			node.namesByID[id] = append(node.namesByID[id], name)
		}
		node.mu.Unlock()
		if !active {
			return runtime.ErrServiceClosed
		}
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(pool.members) != config.Size {
		return runtime.ErrServiceClosed
	}
	if node.local.State() != runtime.RuntimeRunning {
		return runtime.ErrRuntimeStopping
	}
	pool.ready = true
	return nil
}

// removeNameFromID 在持有 node.mu 时移除失败池的反向索引，保留成员发布的其它名称。
func (node *Node) removeNameFromID(id runtime.ServiceID, name cluster.ServiceName) {
	names := node.namesByID[id]
	for i, current := range names {
		if current == name {
			node.namesByID[id] = append(names[:i], names[i+1:]...)
			return
		}
	}
}

func constructPoolMembers(ctx context.Context, config ServicePoolConfig) (services []Service, err error) {
	defer func() {
		if recover() != nil {
			services = nil
			err = fmt.Errorf("%w: pool factory panicked", runtime.ErrServicePanicked)
		}
	}()
	services = make([]Service, config.Size)
	seen := make(map[Service]struct{}, config.Size)
	for i := range services {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		service := config.NewService()
		value := reflect.ValueOf(service)
		if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
			return nil, runtime.ErrInvalidService
		}
		// Go 允许零大小对象复用地址；这类对象没有实例内可变状态。
		if value.Elem().Type().Size() != 0 {
			if _, exists := seen[service]; exists {
				return nil, fmt.Errorf("%w: pool factory reused an instance", runtime.ErrInvalidService)
			}
			seen[service] = struct{}{}
		}
		services[i] = service
	}
	return services, nil
}
