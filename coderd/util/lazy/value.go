// Package lazy provides a lazy value implementation.
// It's useful especially in global variable initialization to avoid
// slowing down the program startup time.
package lazy

import (
	"sync"
	"sync/atomic"
)

type Value[T any] struct {
	once   sync.Once
	fn     func() T
	cached atomic.Pointer[T]
}

func (v *Value[T]) Load() T {
	v.once.Do(func() {
		vv := v.fn()
		v.cached.Store(&vv)
	})
	return *v.cached.Load()
}

// New creates a new lazy value with the given load function.
func New[T any](fn func() T) *Value[T] {
	return &Value[T]{fn: fn}
}
