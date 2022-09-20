package gsync

import "sync"

// Once is a value that must only be generated once. All future
// invocations of Do return the same value.
type Once[T any] struct {
	value T
	done  bool
	mu    sync.Mutex
}

func (o *Once[T]) Do(f func() T) T {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.done {
		return o.value
	}
	o.value = f()
	o.done = true
	return o.value
}
