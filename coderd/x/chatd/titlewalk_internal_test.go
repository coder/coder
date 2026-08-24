package chatd

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/coderd/database"
)

func walkTestServer(t *testing.T) *Server {
	t.Helper()
	return &Server{
		logger: slogtest.Make(t, &slogtest.Options{IgnoreErrors: true}),
	}
}

// resolvedCandidate builds a candidate whose resolve step always succeeds with
// an empty model; the walker tests exercise attempt behavior, not real model
// construction.
func resolvedCandidate(model string) manualTitleCandidate {
	return manualTitleCandidate{
		config: database.ChatModelConfig{Model: model},
		resolve: func(context.Context) (resolvedModelCall, error) {
			return resolvedModelCall{}, nil
		},
	}
}

func TestWalkManualTitleCandidates(t *testing.T) {
	t.Parallel()

	t.Run("FirstCandidateWins", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		var calls int
		title, config, err := p.walkManualTitleCandidates(
			context.Background(),
			database.Chat{},
			[]manualTitleCandidate{resolvedCandidate("a"), resolvedCandidate("b")},
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				calls++
				return "Title A", nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, "Title A", title)
		require.Equal(t, "a", config.Model)
		require.Equal(t, 1, calls, "should stop after the first success")
	})

	t.Run("FallsThroughTimeoutToNextCandidate", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		var models []string
		title, config, err := p.walkManualTitleCandidates(
			context.Background(),
			database.Chat{},
			[]manualTitleCandidate{resolvedCandidate("slow"), resolvedCandidate("fast")},
			func(_ context.Context, cand manualTitleCandidate, _ resolvedModelCall) (string, error) {
				models = append(models, cand.config.Model)
				if cand.config.Model == "slow" {
					return "", xerrors.Errorf("generate manual title: %w", context.DeadlineExceeded)
				}
				return "Title Fast", nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, "Title Fast", title)
		require.Equal(t, "fast", config.Model)
		require.Equal(t, []string{"slow", "fast"}, models)
	})

	t.Run("StopsOnNonRetryableError", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		var calls int
		sentinel := xerrors.New("bad api key")
		_, config, err := p.walkManualTitleCandidates(
			context.Background(),
			database.Chat{},
			[]manualTitleCandidate{resolvedCandidate("first"), resolvedCandidate("second")},
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				calls++
				return "", sentinel
			},
		)
		require.ErrorIs(t, err, sentinel)
		require.Equal(t, "first", config.Model)
		require.Equal(t, 1, calls, "non-retryable error must not fall through")
	})

	t.Run("SkipsCandidateThatFailsToResolve", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		var attempted []string
		candidates := []manualTitleCandidate{
			{
				config: database.ChatModelConfig{Model: "unavailable"},
				resolve: func(context.Context) (resolvedModelCall, error) {
					return resolvedModelCall{}, xerrors.New("no credentials")
				},
			},
			resolvedCandidate("available"),
		}
		title, config, err := p.walkManualTitleCandidates(
			context.Background(),
			database.Chat{},
			candidates,
			func(_ context.Context, cand manualTitleCandidate, _ resolvedModelCall) (string, error) {
				attempted = append(attempted, cand.config.Model)
				return "Title", nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, "Title", title)
		require.Equal(t, "available", config.Model)
		require.Equal(t, []string{"available"}, attempted, "unresolvable candidate is skipped, not attempted")
	})

	t.Run("AllCandidatesTimeoutReturnsDeadlineExceeded", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		_, _, err := p.walkManualTitleCandidates(
			context.Background(),
			database.Chat{},
			[]manualTitleCandidate{resolvedCandidate("a"), resolvedCandidate("b")},
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				return "", xerrors.Errorf("generate manual title: %w", context.DeadlineExceeded)
			},
		)
		require.ErrorIs(t, err, context.DeadlineExceeded)
	})

	// Regression for the review finding: when the context is canceled after a
	// candidate has already failed, the walker must surface ctx.Err() rather
	// than the stale candidate error, so the handler maps it to 499/504 instead
	// of a stale 500.
	t.Run("CancellationAfterFailureSurfacesCtxErr", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		staleErr := xerrors.New("stale provider error")
		var calls int
		_, _, err := p.walkManualTitleCandidates(
			ctx,
			database.Chat{},
			[]manualTitleCandidate{resolvedCandidate("a"), resolvedCandidate("b")},
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				calls++
				// Simulate the caller disconnecting during this attempt.
				cancel()
				return "", staleErr
			},
		)
		require.ErrorIs(t, err, context.Canceled)
		require.False(t, errors.Is(err, staleErr), "must not surface the stale candidate error")
		require.Equal(t, 1, calls, "must not try the next candidate after cancellation")
	})

	t.Run("PreCanceledContextSurfacesCtxErr", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var calls int
		_, _, err := p.walkManualTitleCandidates(
			ctx,
			database.Chat{},
			[]manualTitleCandidate{resolvedCandidate("a")},
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				calls++
				return "Title", nil
			},
		)
		require.ErrorIs(t, err, context.Canceled)
		require.Zero(t, calls, "a pre-canceled context must not run any attempt")
	})

	// The overall walk budget expiring is a genuine title timeout, so the
	// walker must tag it with ErrManualTitleTimedOut for the handler's 504
	// mapping.
	t.Run("OverallDeadlineMarksTimeout", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		var calls int
		_, _, err := p.walkManualTitleCandidates(
			ctx,
			database.Chat{},
			[]manualTitleCandidate{resolvedCandidate("a")},
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				calls++
				return "Title", nil
			},
		)
		require.ErrorIs(t, err, ErrManualTitleTimedOut)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.Zero(t, calls, "an expired overall budget must not run any attempt")
	})

	// A candidate that skips itself at resolve time (e.g. the chat-model
	// fallback duplicating an already-attempted preferred candidate) must not
	// replace the earlier attempt's more meaningful error.
	t.Run("SkipCandidateKeepsEarlierError", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		candidates := []manualTitleCandidate{
			resolvedCandidate("preferred"),
			{
				resolve: func(context.Context) (resolvedModelCall, error) {
					return resolvedModelCall{}, xerrors.Errorf(
						"%w: duplicate of preferred",
						errManualTitleCandidateSkip,
					)
				},
			},
		}
		_, config, err := p.walkManualTitleCandidates(
			context.Background(),
			database.Chat{},
			candidates,
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				return "", xerrors.Errorf("generate manual title: %w", context.DeadlineExceeded)
			},
		)
		require.ErrorIs(t, err, context.DeadlineExceeded)
		require.False(t, errors.Is(err, errManualTitleCandidateSkip),
			"skip sentinel must not replace the attempt error")
		require.Equal(t, "preferred", config.Model)
	})

	t.Run("NoCandidates", func(t *testing.T) {
		t.Parallel()
		p := walkTestServer(t)
		_, _, err := p.walkManualTitleCandidates(
			context.Background(),
			database.Chat{},
			nil,
			func(context.Context, manualTitleCandidate, resolvedModelCall) (string, error) {
				return "Title", nil
			},
		)
		require.ErrorContains(t, err, "no manual title model candidates available")
	})
}
