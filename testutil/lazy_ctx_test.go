package testutil_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestLazyTimeoutContext_LazyStart(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 10*time.Millisecond)

	// Wait longer than the timeout without accessing the context.
	time.Sleep(50 * time.Millisecond)

	// The context should still be valid because the timer only starts on first
	// access to Done(), Deadline(), or Err().
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done yet - timer should not have started")
	default:
	}

	// After accessing Done(), the timer starts and the context should expire.
	select {
	case <-ctx.Done():
	case <-time.After(50 * time.Millisecond):
		t.Fatal("context should have expired after Done() was called")
	}
}

func TestLazyTimeoutContext_ValueDoesNotTriggerStart(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 10*time.Millisecond)

	// Value() is often used for tracing/logging and should not affect timeout
	// behavior, so it must not start the timer.
	_ = ctx.Value("key")

	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatal("context should not be done - Value() should not start timer")
	default:
	}
}

func TestLazyTimeoutContext_Expiration(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 5*time.Millisecond)

	// Store the channel to avoid triggering a reset in the select statement.
	done := ctx.Done()

	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("context should have expired")
	}

	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

func TestLazyTimeoutContext_ResetOnNewLocation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 50*time.Millisecond)

	// Store channel to check expiration without triggering additional resets.
	done := ctx.Done()

	// Wait for 60% of the timeout.
	time.Sleep(30 * time.Millisecond)

	// This access is from a different line, so it should reset the timeout.
	_ = ctx.Done()

	// Wait another 30ms. Without the reset, we'd be at 60ms total (expired).
	// With the reset, we're only 30ms into the new 50ms timeout window.
	time.Sleep(30 * time.Millisecond)

	select {
	case <-done:
		t.Fatal("context should not be done yet - should have been reset")
	default:
	}

	// Eventually the context should expire after no more resets.
	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("context should have expired eventually")
	}
}

func TestLazyTimeoutContext_NoResetOnSameLocation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 50*time.Millisecond)

	var done <-chan struct{}

	// All iterations access Done() from the same line, so no reset occurs.
	// After 5 * 15ms = 75ms, the 50ms timeout should have expired.
	for i := 0; i < 5; i++ {
		done = ctx.Done()
		time.Sleep(15 * time.Millisecond)
	}

	select {
	case <-done:
	default:
		t.Fatal("context should be done - same location should not reset")
	}
}

func TestLazyTimeoutContext_AlreadyExpiredNoResurrection(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 5*time.Millisecond)

	<-ctx.Done()
	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)

	// Accessing from a new location should not bring the context back to life.
	_ = ctx.Err()

	select {
	case <-ctx.Done():
	default:
		t.Fatal("expired context should not be resurrected")
	}

	require.ErrorIs(t, ctx.Err(), context.DeadlineExceeded)
}

func TestLazyTimeoutContext_ThreadSafety(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 100*time.Millisecond)

	var wg sync.WaitGroup
	const numGoroutines = 10

	// We don't assert on return values because this test relies on the race
	// detector (-race flag) to catch synchronization issues.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = ctx.Done()
				_, _ = ctx.Deadline()
				_ = ctx.Err()
				_ = ctx.Value("key")
			}
		}()
	}

	wg.Wait()
}

func TestLazyTimeoutContext_WithChildContext(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 50*time.Millisecond)

	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	select {
	case <-childCtx.Done():
		t.Fatal("child context should not be done yet")
	default:
	}

	cancel()

	select {
	case <-childCtx.Done():
	case <-time.After(50 * time.Millisecond):
		t.Fatal("child context should be done after cancel")
	}
}

func TestLazyTimeoutContext_ErrBeforeExpiration(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 50*time.Millisecond)

	err := ctx.Err()
	assert.NoError(t, err, "Err() should return nil before expiration")
}

func TestLazyTimeoutContext_DeadlineReturnsCorrectValue(t *testing.T) {
	t.Parallel()

	timeout := 50 * time.Millisecond
	before := time.Now()
	ctx := testutil.Context(t, timeout)

	deadline, ok := ctx.Deadline()
	after := time.Now()

	require.True(t, ok, "deadline should be set after Deadline() call")
	require.False(t, deadline.IsZero(), "deadline should not be zero")
	require.True(t, deadline.After(before.Add(timeout-time.Millisecond)))
	require.True(t, deadline.Before(after.Add(timeout+10*time.Millisecond)))
}
