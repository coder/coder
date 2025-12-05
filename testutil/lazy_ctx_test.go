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

	time.Sleep(50 * time.Millisecond) // Longer than timeout.

	// Timer hasn't started, context should be valid.
	select {
	case <-ctx.Done():
		t.Fatal("context should not be done yet - timer should not have started")
	default:
	}

	// First select started the timer, wait for expiration.
	select {
	case <-ctx.Done():
	case <-time.After(50 * time.Millisecond):
		t.Fatal("context should have expired")
	}
}

func TestLazyTimeoutContext_ValueDoesNotTriggerStart(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 10*time.Millisecond)

	_ = ctx.Value("key") // Must not start timer.

	time.Sleep(50 * time.Millisecond)

	select {
	case <-ctx.Done():
		t.Fatal("Value() should not start timer")
	default:
	}
}

func TestLazyTimeoutContext_Expiration(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 5*time.Millisecond)

	done := ctx.Done() // Store to avoid reset in select.

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

	done := ctx.Done()                // Store to check expiration.
	time.Sleep(30 * time.Millisecond) // 60% of timeout.
	_ = ctx.Done()                    // New line, resets timeout.
	time.Sleep(30 * time.Millisecond) // 60% again, would be 120% without reset.

	select {
	case <-done:
		t.Fatal("timeout should have been reset")
	default:
	}

	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("context should have expired")
	}
}

func TestLazyTimeoutContext_NoResetOnSameLocation(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, 50*time.Millisecond)

	var done <-chan struct{}
	// Same line, no reset. 5*15ms = 75ms > 50ms timeout.
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

	_ = ctx.Err() // New location, must not resurrect.

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
	// Relies on -race to detect issues.
	for i := range numGoroutines {
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
