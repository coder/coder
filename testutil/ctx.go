package testutil

import (
	"context"
	"testing"
	"time"
)

// Context returns a context with a timeout that starts on first use and resets
// when accessed from new lines in test files. Each call to Done, Deadline, or
// Err from a new line resets the deadline.
//
// To prevent resets, store the Done channel or wrap with a child context:
//
//	done := ctx.Done()
//	<-done // Uses stored channel, no reset.
func Context(t testing.TB, timeout time.Duration) context.Context {
	return newLazyTimeoutContext(t, timeout)
}

// ContextFixed returns a context with a timeout that starts immediately and
// does not reset.
func ContextFixed(t testing.TB, dur time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	t.Cleanup(cancel)
	return ctx
}
