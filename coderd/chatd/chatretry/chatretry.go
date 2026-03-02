// Package chatretry provides retry logic for transient LLM provider
// errors. It classifies errors as retryable or permanent and
// implements exponential backoff matching the behavior of coder/mux.
package chatretry

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	// InitialDelay is the backoff duration for the first retry
	// attempt.
	InitialDelay = 1 * time.Second

	// MaxDelay is the upper bound for the exponential backoff
	// duration. Matches the cap used in coder/mux.
	MaxDelay = 60 * time.Second
)

// nonRetryablePatterns are substrings that indicate a permanent error
// which should not be retried. These are checked first so that
// ambiguous messages (e.g. "bad request: rate limit") are correctly
// classified as non-retryable.
var nonRetryablePatterns = []string{
	"context canceled",
	"context deadline exceeded",
	"authentication",
	"unauthorized",
	"forbidden",
	"invalid api key",
	"invalid_api_key",
	"invalid model",
	"model not found",
	"model_not_found",
	"context length exceeded",
	"context_exceeded",
	"maximum context length",
	"quota",
	"billing",
}

// retryablePatterns are substrings that indicate a transient error
// worth retrying.
var retryablePatterns = []string{
	"overloaded",
	"rate limit",
	"rate_limit",
	"too many requests",
	"server error",
	"status 500",
	"status 502",
	"status 503",
	"status 529",
	"connection reset",
	"connection refused",
	"eof",
	"broken pipe",
	"timeout",
	"unavailable",
	"service unavailable",
}

// IsRetryable determines whether an error from an LLM provider is
// transient and worth retrying. It inspects the error message and
// any wrapped HTTP status codes for known retryable patterns.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// context.Canceled is always non-retryable regardless of
	// wrapping.
	if errors.Is(err, context.Canceled) {
		return false
	}

	lower := strings.ToLower(err.Error())

	// Check non-retryable patterns first so they take precedence.
	for _, p := range nonRetryablePatterns {
		if strings.Contains(lower, p) {
			return false
		}
	}

	for _, p := range retryablePatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}

	return false
}

// StatusCodeRetryable returns true for HTTP status codes that
// indicate a transient failure worth retrying.
func StatusCodeRetryable(code int) bool {
	switch code {
	case 429, 500, 502, 503, 529:
		return true
	default:
		return false
	}
}

// Delay returns the backoff duration for the given 0-indexed attempt.
// Uses exponential backoff: min(InitialDelay * 2^attempt, MaxDelay).
// Matches the backoff curve used in coder/mux.
func Delay(attempt int) time.Duration {
	d := InitialDelay
	for range attempt {
		d *= 2
		if d >= MaxDelay {
			return MaxDelay
		}
	}
	return d
}

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
