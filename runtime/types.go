package runtime

import "sync/atomic"

// ServiceID identifies a Service within one Runtime lifetime.
type ServiceID uint64

// SessionID correlates a Call with its one Reply completion.
type SessionID uint64

// TimerID identifies a Timer owned by a Service.
type TimerID uint64

// CommandID identifies a command handled by a Service and matches the 32-bit wire command field.
type CommandID uint32

// idAllocator issues monotonically increasing, non-zero uint64 values.
//
// A zero next value is the permanent exhausted state. The allocator never
// scans, reuses or wraps an issued value.
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
