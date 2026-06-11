package chatd //nolint:testpackage // Exercises unexported generation retry helpers.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestTerminalGeneration(t *testing.T) {
	t.Parallel()

	require.Nil(t, terminalGeneration(nil))

	cause := xerrors.New("boom")
	wrapped := terminalGeneration(cause)
	require.True(t, isTerminalGeneration(wrapped))
	require.ErrorIs(t, wrapped, cause)
	require.ErrorIs(t, wrapped, errTerminalGeneration)
	require.Equal(t, cause.Error(), wrapped.Error())

	require.False(t, isTerminalGeneration(cause))
	require.False(t, isTerminalGeneration(nil))
}

func TestGenerationPhaseBackoff(t *testing.T) {
	t.Parallel()

	require.Equal(t, generationPhaseBaseBackoff, generationPhaseBackoff(0))
	require.Equal(t, 2*generationPhaseBaseBackoff, generationPhaseBackoff(1))
	require.Equal(t, 4*generationPhaseBaseBackoff, generationPhaseBackoff(2))
}

func TestRetryGenerationPhase(t *testing.T) {
	t.Parallel()

	noopWait := func(context.Context, time.Duration) error { return nil }

	t.Run("SuccessFirstTry", func(t *testing.T) {
		t.Parallel()
		calls := 0
		waits := 0
		wait := func(context.Context, time.Duration) error {
			waits++
			return nil
		}
		got, err := retryGenerationPhase(context.Background(), wait, func() (int, error) {
			calls++
			return 42, nil
		})
		require.NoError(t, err)
		require.Equal(t, 42, got)
		require.Equal(t, 1, calls)
		require.Equal(t, 0, waits)
	})

	t.Run("RetryThenSuccess", func(t *testing.T) {
		t.Parallel()
		calls := 0
		waits := 0
		var delays []time.Duration
		wait := func(_ context.Context, d time.Duration) error {
			waits++
			delays = append(delays, d)
			return nil
		}
		got, err := retryGenerationPhase(context.Background(), wait, func() (string, error) {
			calls++
			if calls < 2 {
				return "", xerrors.New("transient")
			}
			return "ok", nil
		})
		require.NoError(t, err)
		require.Equal(t, "ok", got)
		require.Equal(t, 2, calls)
		require.Equal(t, 1, waits)
		require.Equal(t, []time.Duration{generationPhaseBackoff(0)}, delays)
	})

	t.Run("ExhaustsAndReturnsLastError", func(t *testing.T) {
		t.Parallel()
		calls := 0
		waits := 0
		wait := func(context.Context, time.Duration) error {
			waits++
			return nil
		}
		_, err := retryGenerationPhase(context.Background(), wait, func() (int, error) {
			calls++
			return 0, xerrors.Errorf("attempt %d", calls)
		})
		require.EqualError(t, err, "attempt 3")
		require.Equal(t, generationPhaseMaxAttempts, calls)
		require.Equal(t, generationPhaseMaxAttempts-1, waits)
	})

	t.Run("TerminalShortCircuits", func(t *testing.T) {
		t.Parallel()
		calls := 0
		waits := 0
		wait := func(context.Context, time.Duration) error {
			waits++
			return nil
		}
		cause := xerrors.New("deterministic")
		_, err := retryGenerationPhase(context.Background(), wait, func() (int, error) {
			calls++
			return 0, terminalGeneration(cause)
		})
		require.ErrorIs(t, err, cause)
		require.True(t, isTerminalGeneration(err))
		require.Equal(t, 1, calls)
		require.Equal(t, 0, waits)
	})

	t.Run("ContextCanceledExitsCleanly", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		_, err := retryGenerationPhase(ctx, noopWait, func() (int, error) {
			calls++
			return 0, xerrors.New("transient")
		})
		require.ErrorIs(t, err, errTaskExpectedExit)
		require.Equal(t, 1, calls)
	})

	t.Run("WaitCancellationExitsCleanly", func(t *testing.T) {
		t.Parallel()
		calls := 0
		waits := 0
		wait := func(context.Context, time.Duration) error {
			waits++
			return errTaskExpectedExit
		}
		_, err := retryGenerationPhase(context.Background(), wait, func() (int, error) {
			calls++
			return 0, xerrors.New("transient")
		})
		require.ErrorIs(t, err, errTaskExpectedExit)
		require.Equal(t, 1, calls)
		require.Equal(t, 1, waits)
	})
}
