// Package watchdog provides the settle-once cancellation timer
// shared by chatd's stream-silence and task-attempt watchdogs.
package watchdog

import (
	"context"
	"sync"
	"time"

	"github.com/coder/quartz"
)

// Timer cancels an operation with a fixed cause when its window
// elapses without a Reset. Exactly one outcome wins: the timer
// fires and cancels the operation, or Disarm stops it. Reset and
// a late fire are safe no-ops once either outcome has settled.
type Timer struct {
	mu      sync.Mutex
	clock   quartz.Clock
	timer   *quartz.Timer
	cancel  context.CancelCauseFunc
	cause   error
	timeout time.Duration
	tags    []string
	// deadline is when the current window elapses; Reset moves it
	// forward. A fire that observes now before the deadline is
	// stale: a Reset re-armed the timer while the callback was
	// waiting on mu, and the re-armed timer fires later.
	deadline time.Time
	settled  bool
}

// New arms a watchdog that calls cancel(cause) once timeout
// elapses without a Reset. tags label the underlying quartz
// timer so tests can trap it.
func New(
	clock quartz.Clock,
	timeout time.Duration,
	cancel context.CancelCauseFunc,
	cause error,
	tags ...string,
) *Timer {
	t := &Timer{
		clock:    clock,
		cancel:   cancel,
		cause:    cause,
		timeout:  timeout,
		tags:     tags,
		deadline: clock.Now().Add(timeout),
	}
	t.timer = clock.AfterFunc(timeout, t.onTimeout, tags...)
	return t
}

func (t *Timer) settle() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.settled {
		return false
	}
	t.settled = true
	return true
}

func (t *Timer) onTimeout() {
	t.mu.Lock()
	if t.settled || t.clock.Now().Before(t.deadline) {
		// Settled, or a Reset re-armed the timer while this
		// callback waited on the lock; the re-armed timer owns
		// the new deadline.
		t.mu.Unlock()
		return
	}
	t.settled = true
	t.mu.Unlock()
	t.cancel(t.cause)
}

// Reset restarts the window. It is a no-op once the timer fired
// or Disarm was called.
func (t *Timer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.settled {
		return
	}
	t.deadline = t.clock.Now().Add(t.timeout)
	t.timer.Reset(t.timeout, t.tags...)
}

// Disarm stops the timer permanently.
func (t *Timer) Disarm() {
	if !t.settle() {
		return
	}
	t.timer.Stop()
}
