package pointer

import (
	"context"

	"go.uber.org/atomic"
)

// New constructs a Handle with an initialized value.
func New[T any](value T) *Handle[T] {
	h := &Handle[T]{
		key: struct{}{},
		ptr: atomic.Pointer[T]{},
	}
	h.Store(value)
	return h
}

// Handle loads the stored value into a context, and returns
// a context with the attached value. It's intention is to
// hold a single handle for the lifecycle of a request.
type Handle[T any] struct {
	key struct{}
	ptr atomic.Pointer[T]
}

func (p *Handle[T]) Load(ctx context.Context) (context.Context, T) {
	value, ok := ctx.Value(&p.key).(T)
	if !ok {
		ctx = context.WithValue(ctx, &p.key, *p.ptr.Load())
		return p.Load(ctx)
	}
	return ctx, value
}

func (p *Handle[T]) Store(t T) {
	p.ptr.Store(&t)
}
