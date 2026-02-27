package chatretry

import (
	"context"
	"time"
)

// RetryFn is the function to retry. It receives a context and returns
// an error. The context may be a child of the original with adjusted
// deadlines for individual attempts.
type RetryFn func(ctx context.Context) error

// OnRetryFn is called before each retry attempt with the attempt
// number (1-indexed), the error that triggered the retry, and the
// delay before the next attempt.
type OnRetryFn func(attempt int, err error, delay time.Duration)

// Retry calls fn repeatedly until it succeeds, returns a
// non-retryable error, or ctx is canceled. There is no max attempt
// limit — retries continue indefinitely with exponential backoff
// (capped at 60s), matching the behavior of coder/mux.
//
// The onRetry callback (if non-nil) is called before each retry
// attempt, giving the caller a chance to reset state, log, or
// publish status events.
func Retry(ctx context.Context, fn RetryFn, onRetry OnRetryFn) error {
	var attempt int
	for {
		err := fn(ctx)
		if err == nil {
			return nil
		}

		if !IsRetryable(err) {
			return err
		}

		// If the caller's context is already done, return the
		// context error so cancellation propagates cleanly.
		if ctx.Err() != nil {
			return ctx.Err()
		}

		delay := Delay(attempt)

		if onRetry != nil {
			onRetry(attempt+1, err, delay)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}

		attempt++
	}
}
