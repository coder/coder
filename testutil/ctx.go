package testutil

import (
	"context"
	"testing"
	"time"
)

// Context returns a context that resets its timeout when accessed from new
// locations in the test file. The timeout does not begin until the context is
// first used.
//
// This is useful for integration tests that pass contexts through many
// subsystems, where each subsystem should get a fresh timeout window.
//
// Note: Each call to Done(), Deadline(), or Err() from a new line in the test
// file resets the timeout. If you need to prevent resets (e.g., to test actual
// timeout behavior), store the channel:
//
//	done := ctx.Done()  // Timeout starts, channel stored
//	// ... do work ...
//	select {
//	case <-done:  // No reset, using stored channel
//	    // handle timeout
//	}
//
// Wrapping with a child context (e.g., context.WithCancel) will also prevent
// resets since the child's methods don't call through to the parent.
func Context(t testing.TB, timeout time.Duration) context.Context {
	return newLazyTimeoutContext(t, timeout)
}

// ContextFixed returns a context with a fixed timeout that starts immediately.
// Use Context() instead for contexts that should reset on new package access.
func ContextFixed(t testing.TB, dur time.Duration) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), dur)
	t.Cleanup(cancel)
	return ctx
}
