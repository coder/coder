// Package chatretry provides retry logic for transient LLM provider
// errors. It classifies errors as retryable or permanent and uses
// exponential backoff with provider retry hints when available.
package chatretry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
)

const (
	// InitialDelay is the backoff duration for the first retry
	// attempt.
	InitialDelay = 1 * time.Second

	// MaxDelay is the upper bound for the exponential backoff
	// duration. Matches the cap used in coder/mux.
	MaxDelay = 60 * time.Second

	// MaxAttempts is the upper bound on retry attempts before
	// giving up. With a 60s max backoff this allows roughly
	// 25 minutes of retries, which is reasonable for transient
	// LLM provider issues.
	MaxAttempts = 25
)

type ClassifiedError = chaterror.ClassifiedError

// IsRetryable determines whether an error from an LLM provider is
// transient and worth retrying.
func IsRetryable(err error) bool {
	return chaterror.Classify(err).Retryable
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

// effectiveDelay returns the delay for the given 0-indexed attempt
// while honoring any provider-supplied minimum retry delay.
func effectiveDelay(attempt int, classified ClassifiedError) time.Duration {
	delay := Delay(attempt)
	if classified.RetryAfter > delay {
		return classified.RetryAfter
	}
	return delay
}

func contextError(ctx context.Context) error {
	if ctx.Err() == nil {
		return nil
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	return ctx.Err()
}

func normalizeAttemptError(err error, classified ClassifiedError) (error, ClassifiedError) {
	if classified.Retryable || classified.StatusCode != 0 || !errors.Is(err, context.Canceled) {
		return err, classified
	}
	err = fmt.Errorf("%w: %w", chaterror.ErrProviderTransportReset, err)
	return err, chaterror.Classify(err)
}

// RetryFn is the function to retry. It receives a context and returns
// an error. The context may be a child of the original with adjusted
// deadlines for individual attempts.
type RetryFn func(ctx context.Context) error

// OnRetryFn is called before each retry attempt with the attempt
// number (1-indexed), the raw error that triggered the retry, the
// normalized error payload, and the delay before the next attempt.
type OnRetryFn func(attempt int, err error, classified ClassifiedError, delay time.Duration)

// Retry calls fn repeatedly until it succeeds, returns a
// non-retryable error, ctx is canceled, or MaxAttempts is reached.
// Retries use exponential backoff capped at MaxDelay, unless the
// normalized error includes a longer provider Retry-After hint.
//
// The onRetry callback (if non-nil) is called before each retry
// attempt, giving the caller a chance to reset state, log, or
// publish status events.
func Retry(ctx context.Context, fn RetryFn, onRetry OnRetryFn) error {
	var attempt int
	for {
		if err := contextError(ctx); err != nil {
			return err
		}

		err := fn(ctx)
		if err == nil {
			return nil
		}

		// The attempt runs with ctx. If ctx is done after fn returns, its
		// cancellation is the authoritative error, and a bare context.Canceled
		// from fn must not be normalized as a provider transport reset.
		if ctxErr := contextError(ctx); ctxErr != nil {
			return ctxErr
		}

		classified := chaterror.Classify(err)
		err, classified = normalizeAttemptError(err, classified)
		if !classified.Retryable {
			return chaterror.WithClassification(err, classified)
		}

		if ctxErr := contextError(ctx); ctxErr != nil {
			return ctxErr
		}

		attempt++
		if attempt >= MaxAttempts {
			return chaterror.WithClassification(
				xerrors.Errorf("max retry attempts (%d) exceeded: %w", MaxAttempts, err),
				classified,
			)
		}

		delay := effectiveDelay(attempt-1, classified)

		if onRetry != nil {
			onRetry(attempt, err, classified, delay)
		}

		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return contextError(ctx)
		case <-timer.C:
		}
	}
}
