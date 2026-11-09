package gsr

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cnooljaw/gsr/logging"
	"github.com/cnooljaw/gsr/runtime"
)

// 验证独立调用者并行进入不同成员、保留来源，且每成员队列满时及时拒绝。
func TestServicePoolParallelCallsAndBoundedAdmission(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	type observation struct{ id, source runtime.ServiceID }
	entered := make(chan observation, 4)
	node := startPoolTestNode(t, func(ctx context.Context, self ServiceContext) error {
		return self.Node().CreateServicePool(ctx, "workers", ServicePoolConfig{Size: 2, MailboxCapacity: 1, NewService: func() Service {
			return &probeService{handle: func(current CommandContext) (any, error) {
				entered <- observation{current.Self(), current.Source()}
				<-release
				return current.Self(), nil
			}}
		}})
	})
	callers := make([]runtime.ServiceID, 2)
	for i := range callers {
		id, err := node.local.CreateService(context.Background(), mustPoolTestWrap(t, node, &probeService{handle: func(current CommandContext) (any, error) {
			return current.CallName(context.Background(), "workers", 1)
		}}))
		if err != nil {
			t.Fatal(err)
		}
		callers[i] = id
	}
	results := make(chan error, 2)
	for _, id := range callers {
		go func() {
			_, err := node.local.Call(context.Background(), id, 1)
			results <- err
		}()
	}
	first := poolTestReceive(t, entered)
	second := poolTestReceive(t, entered)
	if first.id == second.id || first.source == second.source || (first.source != callers[0] && first.source != callers[1]) || (second.source != callers[0] && second.source != callers[1]) {
		t.Fatalf("observations: %+v %+v, callers=%v", first, second, callers)
	}
	in := &HTTPIngress{node: node}
	for i := 0; i < 2; i++ {
		if err := in.SendName("workers", 1); err != nil {
			t.Fatal(err)
		}
	}
	if err := in.SendName("workers", 1); !errors.Is(err, runtime.ErrMailboxFull) {
		t.Fatalf("full pool admission: %v", err)
	}
	unblock()
	for i := 0; i < 2; i++ {
		if err := poolTestReceive(t, results); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if got := poolTestReceive(t, entered); got.source != 0 {
			t.Fatalf("infrastructure source: %v", got)
		}
	}
}

// 验证 Call 超时不会重放或补员，未完成请求仍占原成员，另一成员可继续完成。
func TestServicePoolTimeoutDoesNotReplay(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	entered := make(chan struct{}, 1)
	var built, calls atomic.Int32
	node := startPoolTestNode(t, func(ctx context.Context, self ServiceContext) error {
		return self.Node().CreateServicePool(ctx, "workers", ServicePoolConfig{Size: 2, MailboxCapacity: 1, NewService: func() Service {
			index := built.Add(1)
			return &probeService{handle: func(current CommandContext) (any, error) {
				calls.Add(1)
				if index == 1 {
					entered <- struct{}{}
					<-release
				}
				return index, nil
			}}
		}})
	})
	in := &HTTPIngress{node: node}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := in.CallName(ctx, "workers", 1)
		result <- err
	}()
	poolTestReceive(t, entered)
	if err := poolTestReceive(t, result); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout: %v", err)
	}
	value, err := in.CallName(context.Background(), "workers", 1)
	if err != nil || value != int32(2) || calls.Load() != 2 || built.Load() != 2 {
		t.Fatalf("value=%v err=%v calls=%d built=%d", value, err, calls.Load(), built.Load())
	}
}

// 验证创建失败只回收本池成员、释放保留名；重复名称不调用工厂。
func TestServicePoolCreationRollback(t *testing.T) {
	failure := errors.New("start rejected")
	var stopped atomic.Int32
	node := startPoolTestNode(t, nil)
	made := 0
	err := node.CreateServicePool(context.Background(), "workers", ServicePoolConfig{Size: 2, NewService: func() Service {
		made++
		index := made
		return &probeService{start: func(_ context.Context, _ ServiceContext) error {
			if index == 2 {
				return failure
			}
			return nil
		}, stop: func(_ context.Context) error {
			stopped.Add(1)
			return nil
		}}
	}})
	if !errors.Is(err, failure) || stopped.Load() != 2 {
		t.Fatalf("err=%v stops=%d", err, stopped.Load())
	}
	in := &HTTPIngress{node: node}
	if _, err := in.CallName(context.Background(), "workers", 1); !errors.Is(err, runtime.ErrServiceNotFound) {
		t.Fatal(err)
	}
	config := ServicePoolConfig{Size: 1, NewService: func() Service { return &probeService{handle: func(_ CommandContext) (any, error) { return "ok", nil }} }}
	if err := node.CreateServicePool(context.Background(), "workers", config); err != nil {
		t.Fatal(err)
	}
	config.NewService = func() Service {
		t.Error("duplicate registration invoked factory")
		return &probeService{}
	}
	if err := node.CreateServicePool(context.Background(), "workers", config); !errors.Is(err, ErrServiceNameRegistered) {
		t.Fatal(err)
	}
	value, err := in.CallName(context.Background(), "workers", 1)
	if err != nil || value != "ok" {
		t.Fatalf("retry: %v %v", value, err)
	}
}

// 验证无效配置、重复对象和构造 panic 不发布半成品，也不调用成员 OnStart。
func TestServicePoolRejectsInvalidFactories(t *testing.T) {
	node := startPoolTestNode(t, nil)
	var starts atomic.Int32
	shared := &probeService{start: func(_ context.Context, _ ServiceContext) error {
		starts.Add(1)
		return nil
	}}
	cases := []ServicePoolConfig{
		{Size: 0, NewService: func() Service { return shared }},
		{Size: 1, MailboxCapacity: -1, NewService: func() Service { return shared }},
		{Size: 1},
		{Size: 1, NewService: func() Service { return (*probeService)(nil) }},
		{Size: 2, NewService: func() Service { return shared }},
		{Size: 1, NewService: func() Service { panic("private constructor detail") }},
	}
	for _, config := range cases {
		if err := node.CreateServicePool(context.Background(), "invalid", config); err == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
	if starts.Load() != 0 {
		t.Fatalf("invalid pool started %d members", starts.Load())
	}
	if _, err := node.resolve("invalid"); !errors.Is(err, runtime.ErrServiceNotFound) {
		t.Fatal(err)
	}
}

// 验证成员 panic 后只移除自身，幸存成员继续服务，最后成员退出后同名可重新发布。
func TestServicePoolMemberFailureAndExit(t *testing.T) {
	stops := make(chan int, 2)
	index := 0
	node := startPoolTestNode(t, func(ctx context.Context, self ServiceContext) error {
		return self.Node().CreateServicePool(ctx, "workers", ServicePoolConfig{Size: 2, NewService: func() Service {
			index++
			member := index
			return &probeService{handle: func(self CommandContext) (any, error) {
				if member == 1 {
					panic("member failed")
				}
				self.Exit()
				return "finished", nil
			}, stop: func(_ context.Context) error {
				stops <- member
				return nil
			}}
		}})
	})
	in := &HTTPIngress{node: node}
	if _, err := in.CallName(context.Background(), "workers", 1); !errors.Is(err, runtime.ErrServicePanicked) {
		t.Fatal(err)
	}
	if got := poolTestReceive(t, stops); got != 1 {
		t.Fatal(got)
	}
	value, err := in.CallName(context.Background(), "workers", 1)
	if err != nil || value != "finished" {
		t.Fatalf("survivor: %v %v", value, err)
	}
	if got := poolTestReceive(t, stops); got != 2 {
		t.Fatal(got)
	}
	if _, err := node.resolve("workers"); !errors.Is(err, runtime.ErrServiceNotFound) {
		t.Fatal(err)
	}
}

func startPoolTestNode(t *testing.T, configure func(context.Context, ServiceContext) error) *Node {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan *Node, 1)
	stopped := make(chan struct{})
	done := make(chan error, 1)
	entry := &probeService{start: func(ctx context.Context, self ServiceContext) error {
		if configure != nil {
			if err := configure(ctx, self); err != nil {
				return err
			}
		}
		ready <- self.Node()
		self.Exit()
		return nil
	}, stop: func(_ context.Context) error {
		close(stopped)
		return nil
	}}
	go func() {
		done <- Run(ctx, Config{Logging: logging.Config{Writer: io.Discard}, Runtime: RuntimeConfig{ShutdownTimeout: time.Second}}, entry)
	}()
	t.Cleanup(func() {
		cancel()
		if err := poolTestReceive(t, done); err != nil {
			t.Error(err)
		}
	})
	node := poolTestReceive(t, ready)
	poolTestReceive(t, stopped)
	return node
}

func poolTestReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(2 * time.Second):
		t.Fatal("service pool observation timed out")
		var zero T
		return zero
	}
}

func mustPoolTestWrap(t *testing.T, node *Node, service Service) runtime.Service {
	t.Helper()
	wrapped, err := node.wrap(service)
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

// 验证并发创建期间保留名称、拒绝半成品调用，且普通注册不能覆盖池。
func TestServicePoolNameReservation(t *testing.T) {
	node := startPoolTestNode(t, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	done := make(chan error, 1)
	go func() {
		done <- node.CreateServicePool(context.Background(), "reserved", ServicePoolConfig{Size: 1, NewService: func() Service {
			close(entered)
			<-release
			return &probeService{}
		}})
	}()
	poolTestReceive(t, entered)
	if _, err := (&HTTPIngress{node: node}).CallName(context.Background(), "reserved", 1); !errors.Is(err, runtime.ErrServiceStarting) {
		t.Fatalf("half-built pool: %v", err)
	}
	if err := node.CreateServicePool(context.Background(), "reserved", ServicePoolConfig{Size: 1, NewService: func() Service {
		t.Error("duplicate factory invoked")
		return &probeService{}
	}}); !errors.Is(err, ErrServiceNameRegistered) {
		t.Fatal(err)
	}
	id, err := node.local.CreateService(context.Background(), mustPoolTestWrap(t, node, &probeService{}))
	if err != nil {
		t.Fatal(err)
	}
	if err := node.RegisterService("reserved", id); !errors.Is(err, ErrServiceNameRegistered) {
		t.Fatal(err)
	}
	unblock()
	if err := poolTestReceive(t, done); err != nil {
		t.Fatal(err)
	}
}

// 验证最后成员启动期间取消会回收整个未发布池，取消不妨碍有界清理。
func TestServicePoolCancellationRollback(t *testing.T) {
	node := startPoolTestNode(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stopped atomic.Int32
	index := 0
	err := node.CreateServicePool(ctx, "cancelled", ServicePoolConfig{Size: 2, NewService: func() Service {
		index++
		ordinal := index
		return &probeService{start: func(_ context.Context, _ ServiceContext) error {
			if ordinal == 2 {
				cancel()
			}
			return nil
		}, stop: func(_ context.Context) error {
			stopped.Add(1)
			return nil
		}}
	}})
	if !errors.Is(err, context.Canceled) || stopped.Load() != 2 {
		t.Fatalf("err=%v stopped=%d", err, stopped.Load())
	}
	if _, err := node.resolve("cancelled"); !errors.Is(err, runtime.ErrServiceNotFound) {
		t.Fatal(err)
	}
}

// 验证无状态零大小类型即使共享 Go 地址，也能获得不同 ServiceID 并作为独立成员路由。
func TestServicePoolStatelessMembers(t *testing.T) {
	node := startPoolTestNode(t, func(ctx context.Context, self ServiceContext) error {
		return self.Node().CreateServicePool(ctx, "empty", ServicePoolConfig{Size: 2, NewService: func() Service { return &poolStatelessService{} }})
	})
	in := &HTTPIngress{node: node}
	first, err := in.CallName(context.Background(), "empty", 1)
	if err != nil {
		t.Fatal(err)
	}
	second, err := in.CallName(context.Background(), "empty", 1)
	if err != nil || first == second {
		t.Fatalf("independent IDs: %v %v %v", first, second, err)
	}
}

type poolStatelessService struct{}

var _ Service = (*poolStatelessService)(nil)

func (service *poolStatelessService) OnStart(_ context.Context, _ ServiceContext) error {
	return nil
}

func (service *poolStatelessService) Handle(self CommandContext, _ runtime.CommandID, _ ...any) (any, error) {
	return self.Self(), nil
}

func (service *poolStatelessService) OnStop(_ context.Context) error {
	return nil
}
