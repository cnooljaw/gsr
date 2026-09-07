package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// lifecycleComponent 是 Runtime 所管理组件的内部契约。
//
// 协调器按生命周期计划的显式顺序调用组件。
// 各组件管理自身资源，协调器只安排启动与停止的顺序。
type lifecycleComponent interface {
	lifecycleName() string
	// start 和 stop 负责组件的接纳边界、goroutine 与资源；协调器只安排操作顺序。
	start(context.Context) error
	stop(context.Context) error
}

// lifecyclePlan 在组装处明确展示启动与停止顺序。
// 每个启动组件都必须出现在这两份顺序表中。
type lifecyclePlan struct {
	startOrder []lifecycleComponent
	stopOrder  []lifecycleComponent
}

// lifecycleCoordinator 协调 Runtime 所管理组件的执行顺序。
type lifecycleCoordinator struct {
	plan lifecyclePlan

	mu          sync.Mutex
	started     []lifecycleComponent
	stopStarted bool
	stopDone    chan struct{}
	stopResult  error
}

func newLifecycleCoordinator(plan lifecyclePlan) *lifecycleCoordinator {
	validateLifecyclePlan(plan)
	return &lifecycleCoordinator{
		plan:     plan,
		stopDone: make(chan struct{}),
	}
}

// newCoreLifecycleCoordinator 组装当前 Core 生命周期计划。
// 显式传入组件，让每个 Core 切片的资源归属与关闭顺序清晰可见，
// 无需引入动态插件注册表。
func newCoreLifecycleCoordinator(components ...lifecycleComponent) *lifecycleCoordinator {
	return newLifecycleCoordinator(lifecyclePlan{
		startOrder: append([]lifecycleComponent(nil), components...),
		stopOrder:  append([]lifecycleComponent(nil), components...),
	})
}

func validateLifecyclePlan(plan lifecyclePlan) {
	startNames := make(map[string]struct{}, len(plan.startOrder))
	for _, component := range plan.startOrder {
		name := component.lifecycleName()
		if name == "" {
			panic("runtime lifecycle component name is empty")
		}
		if _, exists := startNames[name]; exists {
			panic("runtime lifecycle start component is duplicated: " + name)
		}
		startNames[name] = struct{}{}
	}

	stopNames := make(map[string]struct{}, len(plan.stopOrder))
	for _, component := range plan.stopOrder {
		name := component.lifecycleName()
		if name == "" {
			panic("runtime lifecycle component name is empty")
		}
		if _, exists := stopNames[name]; exists {
			panic("runtime lifecycle stop component is duplicated: " + name)
		}
		stopNames[name] = struct{}{}
	}

	for name := range startNames {
		if _, exists := stopNames[name]; !exists {
			panic("runtime lifecycle component is missing from stop order: " + name)
		}
	}
	for name := range stopNames {
		if _, exists := startNames[name]; !exists {
			panic("runtime lifecycle stop component is missing from start order: " + name)
		}
	}
}

func (coordinator *lifecycleCoordinator) start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	started := make([]lifecycleComponent, 0, len(coordinator.plan.startOrder))
	for _, component := range coordinator.plan.startOrder {
		if err := invokeLifecycleStart(component, ctx); err != nil {
			rollbackErr := coordinator.stopComponents(ctx, reverseComponents(started))
			return errors.Join(err, rollbackErr)
		}
		started = append(started, component)
	}

	coordinator.mu.Lock()
	coordinator.started = started
	coordinator.mu.Unlock()
	return nil
}

func (coordinator *lifecycleCoordinator) stop(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	coordinator.mu.Lock()
	if coordinator.stopStarted {
		done := coordinator.stopDone
		coordinator.mu.Unlock()
		select {
		case <-done:
		case <-ctx.Done():
			return ctx.Err()
		}
		coordinator.mu.Lock()
		result := coordinator.stopResult
		coordinator.mu.Unlock()
		return result
	}
	coordinator.stopStarted = true
	started := append([]lifecycleComponent(nil), coordinator.started...)
	coordinator.started = nil
	coordinator.mu.Unlock()

	result := coordinator.stopComponents(ctx, coordinator.componentsInStopOrder(started))

	coordinator.mu.Lock()
	coordinator.stopResult = result
	close(coordinator.stopDone)
	coordinator.mu.Unlock()
	return result
}

func (coordinator *lifecycleCoordinator) componentsInStopOrder(started []lifecycleComponent) []lifecycleComponent {
	startedByName := make(map[string]lifecycleComponent, len(started))
	for _, component := range started {
		startedByName[component.lifecycleName()] = component
	}

	ordered := make([]lifecycleComponent, 0, len(started))
	for _, component := range coordinator.plan.stopOrder {
		if startedComponent, exists := startedByName[component.lifecycleName()]; exists {
			ordered = append(ordered, startedComponent)
		}
	}
	return ordered
}

func (coordinator *lifecycleCoordinator) stopComponents(ctx context.Context, components []lifecycleComponent) error {
	var result error
	for _, component := range components {
		if err := invokeLifecycleStop(component, ctx); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

func invokeLifecycleStart(component lifecycleComponent, ctx context.Context) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("runtime lifecycle start %s panicked: %v", component.lifecycleName(), recovered)
		}
	}()

	if err := component.start(ctx); err != nil {
		return fmt.Errorf("runtime lifecycle start %s: %w", component.lifecycleName(), err)
	}
	return nil
}

func invokeLifecycleStop(component lifecycleComponent, ctx context.Context) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("runtime lifecycle stop %s panicked: %v", component.lifecycleName(), recovered)
		}
	}()

	if err := component.stop(ctx); err != nil {
		return fmt.Errorf("runtime lifecycle stop %s: %w", component.lifecycleName(), err)
	}
	return nil
}

func reverseComponents(components []lifecycleComponent) []lifecycleComponent {
	reversed := make([]lifecycleComponent, len(components))
	for index, component := range components {
		reversed[len(components)-index-1] = component
	}
	return reversed
}
