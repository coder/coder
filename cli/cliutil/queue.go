package cliutil

import (
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

// Queue is a FIFO queue with a fixed size.  If the size is exceeded, the first
// item is dropped.
type Queue[T any] struct {
	cond   *sync.Cond
	items  []T
	mu     sync.Mutex
	size   int
	closed bool
	pred   func(x T) (T, bool)
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

// WithPredicate adds the given predicate function, which can control what is
// pushed to the queue.
func (q *Queue[T]) WithPredicate(pred func(x T) (T, bool)) *Queue[T] {
	q.pred = pred
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
	// Potentially mutate or skip the push using the predicate.
	if q.pred != nil {
		var ok bool
		x, ok = q.pred(x)
		if !ok {
			return nil
		}
	}
	// Remove the first item from the queue if it has gotten too big.
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

type reportTask struct {
	link         string
	messageID    int64
	selfReported bool
	state        codersdk.WorkspaceAppStatusState
	summary      string
}

// statusQueue is a Queue that:
// 1. Only pushes items that are not duplicates.
// 2. Preserves the existing message and URI when one a message is not provided.
// 3. Ignores "working" updates from the status watcher.
type StatusQueue struct {
	Queue[reportTask]
	// lastMessageID is the ID of the last *user* message that we saw.  A user
	// message only happens when interacting via the API (as opposed to
	// interacting with the terminal directly).
	lastMessageID int64
}

func (q *StatusQueue) Push(report reportTask) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return xerrors.New("queue has been closed")
	}
	var lastReport reportTask
	if len(q.items) > 0 {
		lastReport = q.items[len(q.items)-1]
	}
	// Use "working" status if this is a new user message.  If this is not a new
	// user message, and the status is "working" and not self-reported (meaning it
	// came from the screen watcher), then it means one of two things:
	// 1. The LLM is still working, in which case our last status will already
	//    have been "working", so there is nothing to do.
	// 2. The user has interacted with the terminal directly.  For now, we are
	//    ignoring these updates.  This risks missing cases where the user
	//    manually submits a new prompt and the LLM becomes active and does not
	//    update itself, but it avoids spamming useless status updates as the user
	//    is typing, so the tradeoff is worth it.  In the future, if we can
	//    reliably distinguish between user and LLM activity, we can change this.
	if report.messageID > q.lastMessageID {
		report.state = codersdk.WorkspaceAppStatusStateWorking
	} else if report.state == codersdk.WorkspaceAppStatusStateWorking && !report.selfReported {
		q.mu.Unlock()
		return nil
	}
	// Preserve previous message and URI if there was no message.
	if report.summary == "" {
		report.summary = lastReport.summary
		if report.link == "" {
			report.link = lastReport.link
		}
	}
	// Avoid queueing duplicate updates.
	if report.state == lastReport.state &&
		report.link == lastReport.link &&
		report.summary == lastReport.summary {
		return nil
	}
	// Drop the first item if the queue has gotten too big.
	if len(q.items) >= q.size {
		q.items = q.items[1:]
	}
	q.items = append(q.items, report)
	q.cond.Broadcast()
	return nil
}
