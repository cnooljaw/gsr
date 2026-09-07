package runtime

import "sync/atomic"

// ServiceID 在单个 Runtime 生命周期内标识一个 Service。
type ServiceID uint64

// SessionID 将一次 Call 与其唯一的 Reply 完成结果关联。
type SessionID uint64

// TimerID 标识归属于某个 Service 的 Timer。
type TimerID uint64

// CommandID 标识 Service 处理的命令，与线协议的 32 位命令字段一致。
type CommandID uint32

// idAllocator 分配单调递增的非零 uint64 标识。
//
// next 为零表示永久耗尽；分配器不会扫描、复用或回绕已发放的标识。
type idAllocator struct {
	nextValue atomic.Uint64
}

func newIDAllocator(next uint64) *idAllocator {
	allocator := &idAllocator{}
	allocator.nextValue.Store(next)
	return allocator
}

func (allocator *idAllocator) next() (uint64, error) {
	if allocator == nil {
		return 0, ErrIDExhausted
	}
	for {
		current := allocator.nextValue.Load()
		if current == 0 {
			return 0, ErrIDExhausted
		}
		if allocator.nextValue.CompareAndSwap(current, current+1) {
			return current, nil
		}
	}
}
