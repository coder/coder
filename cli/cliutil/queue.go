package cliutil

import (
	"sync"

	"golang.org/x/xerrors"
)

// Queue is a FIFO queue with a fixed size.  If the size is exceeded, the first
// item is dropped.
type Queue[T any] struct {
	cond   *sync.Cond
	items  []T
	mu     sync.Mutex
	size   int
	closed bool
}

// NewQueue creates a queue with the given size.
func NewQueue[T any](size int) *Queue[T] {
	q := &Queue[T]{
		items: make([]T, 0, size),
		size:  size,
	}
	q.cond = sync.NewCond(&q.mu)
	return q
}

// Close aborts any pending pops and makes future pushes error.
func (q *Queue[T]) Close() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.closed = true
	q.cond.Broadcast()
}

// Push adds an item to the queue.  If closed, returns an error.
func (q *Queue[T]) Push(x T) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return xerrors.New("queue has been closed")
	}
	if len(q.items) >= q.size {
		q.items = q.items[1:]
	}
	q.items = append(q.items, x)
	q.cond.Broadcast()
	return nil
}

// Pop removes and returns the first item from the queue, waiting until there is
// something to pop if necessary.  If closed, returns false.
func (q *Queue[T]) Pop() (T, bool) {
	var head T
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.items) == 0 && !q.closed {
		q.cond.Wait()
	}
	if q.closed {
		return head, false
	}
	head, q.items = q.items[0], q.items[1:]
	return head, true
}

func (q *Queue[T]) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.items)
}
