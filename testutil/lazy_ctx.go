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

// lazyTimeoutContext implements context.Context with a timeout that starts on
// first use and resets when accessed from new locations in test files.
type lazyTimeoutContext struct {
	t       testing.TB
	timeout time.Duration

	mu            sync.Mutex // Protects following fields.
	testDone      bool       // True after cancel, prevents post-test logging.
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

// Deadline returns the current deadline. The deadline is set on first access
// and may be extended when accessed from new locations in test files.
func (c *lazyTimeoutContext) Deadline() (deadline time.Time, ok bool) {
	c.maybeResetForLocation()

	c.mu.Lock()
	defer c.mu.Unlock()
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

// Value returns nil. It does not trigger initialization or reset.
func (*lazyTimeoutContext) Value(any) any {
	return nil
}

// maybeResetForLocation starts the timer on first access and resets the
// deadline when called from a previously unseen location in a test file.
func (c *lazyTimeoutContext) maybeResetForLocation() {
	loc := callerLocation()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Already canceled.
	if c.err != nil {
		return
	}

	// First access, start timer.
	if c.timer == nil {
		c.startLocked()
		if loc != "" {
			c.seenLocations[loc] = struct{}{}
		}
		if testing.Verbose() && !c.testDone {
			c.t.Logf("lazyTimeoutContext: started timeout for location: %s", loc)
		}
		return
	}

	// Non-test location, ignore.
	if loc == "" {
		return
	}

	if _, seen := c.seenLocations[loc]; seen {
		return
	}
	c.seenLocations[loc] = struct{}{}

	// New location, reset deadline.
	c.deadline = time.Now().Add(c.timeout)
	if c.timer.Stop() {
		c.timer.Reset(c.timeout)
	}

	if testing.Verbose() && !c.testDone {
		c.t.Logf("lazyTimeoutContext: reset timeout for new location: %s", loc)
	}
}

// startLocked initializes the deadline and timer. It must be called with mu held.
func (c *lazyTimeoutContext) startLocked() {
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

// cancel stops the timer and marks the context as canceled. It is called by
// t.Cleanup when the test ends.
func (c *lazyTimeoutContext) cancel() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.testDone = true
	if c.timer != nil {
		c.timer.Stop()
	}
	if c.err == nil {
		c.err = context.Canceled
		close(c.done)
	}
}

// callerLocation returns the file:line of the first caller in a _test.go file,
// or the empty string if none is found.
func callerLocation() string {
	// Skip runtime.Callers, callerLocation, maybeResetForLocation, and the
	// context method (Done/Deadline/Err).
	pc := make([]uintptr, 50)
	n := runtime.Callers(4, pc)
	if n == 0 {
		return ""
	}

	frames := runtime.CallersFrames(pc[:n])
	for {
		frame, more := frames.Next()

		if strings.HasSuffix(frame.File, "_test.go") {
			return fmt.Sprintf("%s:%d", frame.File, frame.Line)
		}

		if !more {
			break
		}
	}

	return ""
}
