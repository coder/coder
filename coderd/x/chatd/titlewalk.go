package chatd

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatretry"
)

const (
	// titleAttemptTimeout bounds a single model call for manual title
	// generation (applied inside generateManualTitle). A slow or hung
	// provider is killed at this deadline so the candidate walk can fall
	// through to the next model instead of burning the overall budget.
	titleAttemptTimeout = 30 * time.Second
	// titleOverallTimeout bounds the entire manual title candidate walk so a
	// slow first provider cannot starve the fallbacks. Multiple per-attempt
	// deadlines fit within it.
	titleOverallTimeout = 90 * time.Second
)

// ErrManualTitleTimedOut marks a manual title failure caused by an expired
// title deadline (the per-attempt timeout or the overall walk budget), as
// opposed to a provider error whose chain merely contains an unrelated
// transport deadline. The API handler maps this sentinel to a friendly 504.
var ErrManualTitleTimedOut = xerrors.New("manual title generation timed out")

// errManualTitleCandidateSkip marks a candidate that turned out to be
// redundant at resolve time, for example the chat-model fallback resolving to
// a preferred config that was already attempted. The walker skips it without
// replacing an earlier attempt's more meaningful error.
var errManualTitleCandidateSkip = xerrors.New("manual title candidate skipped")

// markManualTitleTimeout tags err with ErrManualTitleTimedOut when it stems
// from an expired context deadline, so the handler can distinguish a real
// title timeout from a provider failure that wraps one.
func markManualTitleTimeout(err error) error {
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.Join(ErrManualTitleTimedOut, err)
}

// manualTitleCandidate is one model the manual title walk can try. resolve
// builds the runnable model lazily so the common case (the first candidate
// succeeds) never constructs clients it does not use, and unit tests that
// only exercise the primary candidate do not force fallback resolution.
type manualTitleCandidate struct {
	config  database.ChatModelConfig
	resolve func(ctx context.Context) (resolvedModelCall, error)
}

// manualTitleFallThrough reports whether a failed manual title attempt should
// advance to the next candidate. Only per-attempt deadline expiry and
// chatretry-classified transient errors fall through; non-retryable errors
// (auth, config) stop the walk so the real failure surfaces instead of
// silently trying every provider until the overall budget is exhausted.
func manualTitleFallThrough(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return chatretry.IsRetryable(err)
}

// walkManualTitleCandidates tries each candidate in order, falling through on
// transient or per-attempt-timeout failures per manualTitleFallThrough. It
// returns the first success along with the winning candidate's config.
//
// When ctx is canceled or its overall deadline expires, walkManualTitleCandidates
// surfaces ctx.Err() rather than the last candidate's (stale) error, so the
// handler maps caller cancellation to 499 and overall-budget expiry to 504
// instead of leaking a wrapped provider 500. This includes the window where a
// candidate has already failed and ctx is canceled before the next attempt.
func (p *Server) walkManualTitleCandidates(
	ctx context.Context,
	chat database.Chat,
	candidates []manualTitleCandidate,
	attempt func(ctx context.Context, cand manualTitleCandidate, resolved resolvedModelCall) (string, error),
) (string, database.ChatModelConfig, error) {
	var lastErr error
	var lastConfig database.ChatModelConfig
	for _, cand := range candidates {
		// Overall budget exhausted or caller canceled between attempts.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", lastConfig, markManualTitleTimeout(ctxErr)
		}

		resolved, err := cand.resolve(ctx)
		if err != nil {
			// Model construction is best-effort: log and try the next
			// candidate rather than failing the whole request.
			p.logger.Debug(ctx, "manual title candidate unavailable",
				slog.F("chat_id", chat.ID),
				slog.F("model", cand.config.Model),
				slog.Error(err),
			)
			if errors.Is(err, errManualTitleCandidateSkip) {
				// Redundant candidate; keep the earlier, more
				// meaningful error.
				continue
			}
			lastErr = err
			lastConfig = cand.config
			continue
		}

		title, err := attempt(ctx, cand, resolved)
		if err == nil {
			return title, cand.config, nil
		}
		lastErr = err
		lastConfig = cand.config

		// Caller-side cancellation or overall-budget expiry wins over the
		// candidate's own error so the handler maps to 499/504 instead of a
		// stale provider 500. Checked here (not only at the top of the loop)
		// to cover cancellation in the window after this attempt failed.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", lastConfig, markManualTitleTimeout(ctxErr)
		}
		if !manualTitleFallThrough(err) {
			return "", lastConfig, lastErr
		}
	}
	if lastErr == nil {
		lastErr = xerrors.New("no manual title model candidates available")
	}
	return "", lastConfig, lastErr
}

// newResolvedManualTitleCandidate wraps an already-resolved model as a
// candidate whose resolve step is a no-op.
func newResolvedManualTitleCandidate(resolved resolvedModelCall) manualTitleCandidate {
	return manualTitleCandidate{
		config: resolved.dbConfig,
		resolve: func(context.Context) (resolvedModelCall, error) {
			return resolved, nil
		},
	}
}

// newChatModelFallbackManualTitleCandidate returns the chat's own model as a
// final walk candidate so the request can still succeed when every preferred
// short-text model fails to resolve or is unavailable. Resolution is lazy and
// skips itself (errManualTitleCandidateSkip) when the chat's model resolves to
// a config that was already attempted as a preferred candidate.
func (p *Server) newChatModelFallbackManualTitleCandidate(
	chat database.Chat,
	modelOpts modelBuildOptions,
	attempted map[uuid.UUID]bool,
) manualTitleCandidate {
	return manualTitleCandidate{
		resolve: func(ctx context.Context) (resolvedModelCall, error) {
			config, err := p.resolveModelConfig(ctx, chat)
			if err != nil {
				return resolvedModelCall{}, xerrors.Errorf(
					"resolve fallback manual title model config: %w",
					err,
				)
			}
			if config.ID != uuid.Nil && attempted[config.ID] {
				return resolvedModelCall{}, xerrors.Errorf(
					"%w: chat model %q already attempted as a preferred candidate",
					errManualTitleCandidateSkip,
					config.Model,
				)
			}
			resolved, err := p.resolveModelCall(ctx, modelCallSpec{
				purpose:        "title",
				chat:           chat,
				explicitConfig: &config,
				buildOptions:   modelOpts,
			})
			if err != nil {
				return resolvedModelCall{}, xerrors.Errorf(
					"create fallback manual title model: %w",
					err,
				)
			}
			return resolved, nil
		},
	}
}

// newLazyManualTitleCandidate builds a candidate whose model is constructed on
// first use, so fallback providers are only dialed when an earlier candidate
// fails.
func (p *Server) newLazyManualTitleCandidate(
	chat database.Chat,
	config database.ChatModelConfig,
	modelOpts modelBuildOptions,
) manualTitleCandidate {
	return manualTitleCandidate{
		config: config,
		resolve: func(ctx context.Context) (resolvedModelCall, error) {
			return p.resolveModelCall(ctx, modelCallSpec{
				purpose:        "title",
				chat:           chat,
				explicitConfig: &config,
				buildOptions:   modelOpts,
			})
		},
	}
}
