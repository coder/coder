package testutil

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var _ context.Context = (*lazyTimeoutContext)(nil)

// lazyTimeoutContext is a context.Context that resets its timeout when accessed
// from new locations in the test file. The timeout does not begin until the
// context is first used.
type lazyTimeoutContext struct {
	t       testing.TB
	timeout time.Duration

	mu            sync.Mutex
	started       bool
	deadline      time.Time
	timer         *time.Timer
	done          chan struct{}
	err           error
	seenLocations map[string]struct{}
}

func newLazyTimeoutContext(t testing.TB, timeout time.Duration) context.Context {
	ctx := &lazyTimeoutContext{
		t:             t,
		timeout:       timeout,
		done:          make(chan struct{}),
		seenLocations: make(map[string]struct{}),
	}
	t.Cleanup(ctx.cancel)
	return ctx
}

// Deadline returns the current deadline, if any. The deadline is set lazily
// on first access and may be extended when accessed from new locations.
func (c *lazyTimeoutContext) Deadline() (deadline time.Time, ok bool) {
	c.maybeResetForLocation()

	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.started {
		return time.Time{}, false
	}
	return c.deadline, true
}

// Done returns a channel that's closed when the context is canceled.
func (c *lazyTimeoutContext) Done() <-chan struct{} {
	c.maybeResetForLocation()
	return c.done
}

// Err returns the error indicating why this context was canceled.
func (c *lazyTimeoutContext) Err() error {
	c.maybeResetForLocation()

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

// Value returns nil; this context carries no values.
// Note: Value() does NOT trigger lazy initialization or timeout reset.
func (*lazyTimeoutContext) Value(any) any {
	return nil
}

// maybeResetForLocation starts the timer on first access and resets it when
// accessed from a new location in the test file.
func (c *lazyTimeoutContext) maybeResetForLocation() {
	loc := callerLocation()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Don't reset if already canceled.
	if c.err != nil {
		return
	}

	// Always start the timer on first access, regardless of location.
	if !c.started {
		c.startLocked()
		if loc != "" {
			c.seenLocations[loc] = struct{}{}
		}
		if testing.Verbose() {
			c.t.Logf("lazyTimeoutContext: started timeout for location: %s", loc)
		}
		return
	}

	// Only reset for known test file locations.
	if loc == "" {
		return
	}

	if _, seen := c.seenLocations[loc]; seen {
		return
	}
	c.seenLocations[loc] = struct{}{}

	// Reset deadline.
	c.deadline = time.Now().Add(c.timeout)
	if c.timer != nil && c.timer.Stop() {
		c.timer.Reset(c.timeout)
	}

	if testing.Verbose() {
		c.t.Logf("lazyTimeoutContext: reset timeout for new location: %s", loc)
	}
}

// startLocked initializes the timer. Must be called with mu held.
func (c *lazyTimeoutContext) startLocked() {
	c.started = true
	c.deadline = time.Now().Add(c.timeout)
	c.timer = time.AfterFunc(c.timeout, func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.err == nil {
			c.err = context.DeadlineExceeded
			close(c.done)
		}
	})
}

// cancel stops the timer and marks the context as canceled.
func (c *lazyTimeoutContext) cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer != nil {
		c.timer.Stop()
	}
	if c.err == nil {
		c.err = context.Canceled
		close(c.done)
	}
}

// callerLocation walks the stack to find the line in a test file that
// initiated the call. Returns empty string if not called from a test file.
func callerLocation() string {
	// Skip: runtime.Callers, callerLocation, maybeResetForLocation,
	// Done/Deadline/Err, and we want to find the caller of those.
	pc := make([]uintptr, 50)
	n := runtime.Callers(4, pc)
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pc[:n])
	for {
		frame, more := frames.Next()

		// Look for frames in _test.go files.
		if strings.HasSuffix(frame.File, "_test.go") {
			return fmt.Sprintf("%s:%d", frame.File, frame.Line)
		}

		if !more {
			break
		}
	}

	return ""
}
