package harness

import (
	"sync/atomic"
)

type Barrier struct {
	count atomic.Int64
	done  chan struct{}
}

// NewBarrier creates a new barrier that will block until `size` calls to Wait
// or `Cancel` have been made. It's the caller's responsibility to ensure this
// eventually happens.
func NewBarrier(size int) *Barrier {
	b := &Barrier{
		done: make(chan struct{}),
	}
	b.count.Store(int64(size))
	return b
}

// Wait blocks until the barrier count reaches zero.
func (b *Barrier) Wait() {
	b.Cancel()
	<-b.done
}

// Cancel decrements the barrier count, unblocking other Wait calls if it
// reaches zero.
func (b *Barrier) Cancel() {
	if b.count.Add(-1) == 0 {
		close(b.done)
	}
}
