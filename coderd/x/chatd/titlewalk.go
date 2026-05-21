package chatd

import (
	"context"
	"errors"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
)

const (
	// titleAttemptTimeout bounds a single model call inside a title
	// walk. A slow or hung provider gets killed at this deadline so
	// the walker can fall through to the next candidate instead of
	// burning the overall budget.
	titleAttemptTimeout = 30 * time.Second
	// titleOverallTimeout bounds the entire candidate walk. Three
	// per-attempt timeouts can elapse without exceeding it, so the
	// walker has room to fall through transient failures and still
	// land a result.
	titleOverallTimeout = 90 * time.Second
)

// titleCandidate is a single (provider, model, runnable model) tuple
// the title walker can try.
//
// configID is set when the candidate was selected from a
// chat_model_configs row (manual title generation needs it to record
// usage). It is zero-valued for candidates built directly from
// preferredTitleModels or the chat's fallback model, where no DB
// row applies.
type titleCandidate struct {
	configID uuid.NullUUID
	provider string
	model    string
	lm       fantasy.LanguageModel
}

// titleWalkConfig parameterizes walkTitleCandidates.
type titleWalkConfig struct {
	// perAttempt bounds a single attempt against one candidate. Zero
	// disables it and lets ctx govern. Set this when multiple
	// candidates share a single ctx so a slow first candidate does
	// not starve later ones.
	perAttempt time.Duration

	// shouldFallThrough decides whether to advance to the next
	// candidate after attempt returns err != nil. Nil means "always
	// fall through". Manual title generation uses a stricter
	// policy (timeout + chatretry-retryable only) so non-retryable
	// provider errors like auth/config surface to the user instead
	// of silently moving on.
	shouldFallThrough func(err error) bool
}

// alwaysFallThrough is the policy used by best-effort short-text
// walkers (auto title, turn status label): every error advances to
// the next candidate so a slow or misconfigured first provider does
// not block other candidates from running. Both call sites swallow
// failures, so falling through on auth/config errors is safe.
func alwaysFallThrough(error) bool { return true }

// retryableOrTimeoutFallThrough is the manual-title policy: fall
// through only on per-attempt deadline expiry and chatretry-
// classified transient errors. Non-retryable errors (auth, config)
// stop the walk so the user sees the real failure surfaced as 500
// instead of silently trying every provider and timing out.
func retryableOrTimeoutFallThrough(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return chatretry.IsRetryable(err)
}

// walkTitleCandidates calls attempt against each candidate in
// sequence, applying a per-attempt deadline and a fall-through
// policy. It returns the first success along with the winning
// candidate's index. On exhaustion it returns the last attempt's
// index and error.
//
// When ctx is canceled or its deadline expires, walkTitleCandidates
// surfaces ctx.Err() so callers can distinguish "all candidates
// exhausted" from "caller asked us to stop" (the manual title
// handler maps the latter to 499/504).
func walkTitleCandidates[T any](
	ctx context.Context,
	candidates []titleCandidate,
	cfg titleWalkConfig,
	attempt func(ctx context.Context, cand titleCandidate) (T, error),
) (result T, used int, err error) {
	used = -1
	var lastErr error
	for i, cand := range candidates {
		// Surface a pre-loop or between-iterations ctx cancel so
		// the caller sees ctx.Err() rather than an empty result.
		// Without this, a parent-canceled ctx would exit with
		// lastErr == nil and the caller could treat it as "no
		// candidates tried" (manual title) or silently skip the
		// walk (auto/turn).
		if ctxErr := ctx.Err(); ctxErr != nil {
			if lastErr == nil {
				lastErr = ctxErr
				used = i
			}
			return result, used, lastErr
		}

		attemptResult, attemptErr := callTitleAttempt(ctx, cfg.perAttempt, cand, attempt)
		if attemptErr == nil {
			return attemptResult, i, nil
		}

		lastErr = attemptErr
		used = i

		// Caller-side cancellation wins over candidate errors.
		if ctx.Err() != nil {
			return result, used, lastErr
		}
		if i == len(candidates)-1 {
			break
		}
		if cfg.shouldFallThrough != nil && !cfg.shouldFallThrough(attemptErr) {
			break
		}
	}
	return result, used, lastErr
}

// callTitleAttempt wraps a single attempt with its per-attempt
// timeout so the deferred cancel runs at the end of each iteration
// rather than at function return.
func callTitleAttempt[T any](
	ctx context.Context,
	perAttempt time.Duration,
	cand titleCandidate,
	attempt func(ctx context.Context, cand titleCandidate) (T, error),
) (T, error) {
	attemptCtx := ctx
	if perAttempt > 0 {
		var cancel context.CancelFunc
		attemptCtx, cancel = context.WithTimeout(ctx, perAttempt)
		defer cancel()
	}
	return attempt(attemptCtx, cand)
}
