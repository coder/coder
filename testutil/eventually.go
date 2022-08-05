package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Eventually is like require.Eventually except it allows passing
// a context into the condition. It is safe to use with `require.*`.
//
// If ctx times out, the test will fail, but not immediately.
// It is the caller's responsibility to exit early if required.
//
// It is the caller's responsibility to ensure that ctx has a
// deadline or timeout set. Eventually will panic if this is not
// the case in order to avoid potentially waiting forever.
//
// condition is not run in a goroutine; use the provided
// context argument for cancellation if required.
func Eventually(ctx context.Context, t testing.TB, condition func(context.Context) bool, tick time.Duration) bool {
	t.Helper()

	if _, ok := ctx.Deadline(); !ok {
		panic("developer error: must set deadline or timeout on ctx")
	}

	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for tick := ticker.C; ; {
		select {
		case <-ctx.Done():
			assert.NoError(t, ctx.Err(), "Eventually timed out")
			return false
		case <-tick:
			if condition(ctx) {
				return true
			}
			tick = ticker.C
		}
	}
}

// EventuallyShort is a convenience function that runs Eventually with
// IntervalFast and times out after WaitShort.
func EventuallyShort(t testing.TB, condition func(context.Context) bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), WaitShort)
	defer cancel()
	return Eventually(ctx, t, condition, IntervalFast)
}

// EventuallyMedium is a convenience function that runs Eventually with
// IntervalMedium and times out after WaitMedium.
func EventuallyMedium(t testing.TB, condition func(context.Context) bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), WaitMedium)
	defer cancel()
	return Eventually(ctx, t, condition, IntervalMedium)
}

// EventuallyLong is a convenience function that runs Eventually with
// IntervalSlow and times out after WaitLong.
func EventuallyLong(t testing.TB, condition func(context.Context) bool) bool {
	ctx, cancel := context.WithTimeout(context.Background(), WaitLong)
	defer cancel()
	return Eventually(ctx, t, condition, IntervalSlow)
}
