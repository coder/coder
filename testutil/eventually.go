package testutil

import (
	"context"
	"testing"
	"time"
)

// Eventually is like require.Eventually except it takes a context.
// If ctx times out, the test will fail.
// ctx must have a deadline set or this will panic.
func Eventually(ctx context.Context, t testing.TB, condition func() bool, tick time.Duration) bool {
	t.Helper()

	if _, ok := ctx.Deadline(); !ok {
		panic("developer error: must set deadline on ctx")
	}

	ch := make(chan bool, 1)
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for tick := ticker.C; ; {
		select {
		case <-ctx.Done():
			t.Errorf("Await timed out")
			return false
		case <-tick:
			tick = nil
			go func() { ch <- condition() }()
		case v := <-ch:
			if v {
				return true
			}
			tick = ticker.C
		}
	}
}

// EventuallyShort is a convenience function that runs Eventually with
// IntervalFast and times out after WaitShort.
func EventuallyShort(t testing.TB, condition func() bool) bool {
	//nolint: gocritic
	ctx, cancel := context.WithTimeout(context.Background(), WaitShort)
	defer cancel()
	return Eventually(ctx, t, condition, IntervalFast)
}

// EventuallyMedium is a convenience function that runs Eventually with
// IntervalMedium and times out after WaitMedium.
func EventuallyMedium(t testing.TB, condition func() bool) bool {
	//nolint: gocritic
	ctx, cancel := context.WithTimeout(context.Background(), WaitMedium)
	defer cancel()
	return Eventually(ctx, t, condition, IntervalMedium)
}

// EventuallyLong is a convenience function that runs Eventually with
// IntervalSlow and times out after WaitLong.
func EventuallyLong(t testing.TB, condition func() bool) bool {
	//nolint: gocritic
	ctx, cancel := context.WithTimeout(context.Background(), WaitLong)
	defer cancel()
	return Eventually(ctx, t, condition, IntervalSlow)
}
