package chatd //nolint:testpackage // Exercises unexported generation retry helpers.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
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

	t.Run("SuccessFirstTry", func(t *testing.T) {
		t.Parallel()
		starter := newGenerationPhaseTestStarter(t, quartz.NewMock(t).WithLogger(quartz.NoOpLogger))
		calls := 0
		got, err := retryGenerationPhase(context.Background(), starter, "prepare", func() (int, error) {
			calls++
			return 42, nil
		})
		require.NoError(t, err)
		require.Equal(t, 42, got)
		require.Equal(t, 1, calls)
	})

	t.Run("RetryThenSuccess", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
		timerTrap := clock.Trap().NewTimer("chatworker", "generation-phase-retry")
		defer timerTrap.Close()
		sink := testutil.NewFakeSink(t)
		starter := newGenerationPhaseTestStarter(t, clock)
		starter.opts.Logger = sink.Logger()
		ctx := testutil.Context(t, testutil.WaitLong)
		calls := 0
		done := make(chan phaseRetryResult[string], 1)
		go func() {
			got, err := retryGenerationPhase(ctx, starter, "prepare", func() (string, error) {
				calls++
				if calls < 2 {
					return "", xerrors.New("transient")
				}
				return "ok", nil
			})
			done <- phaseRetryResult[string]{value: got, err: err}
		}()

		timerTrap.MustWait(ctx).MustRelease(ctx)
		clock.Advance(generationPhaseBackoff(0)).MustWait(ctx)
		result := <-done
		require.NoError(t, result.err)
		require.Equal(t, "ok", result.value)
		require.Equal(t, 2, calls)
		entries := entriesWithMessage(sink, "chat generation phase retrying")
		require.Len(t, entries, 1)
		require.Equal(t, "prepare", sinkFieldValue(t, entries[0].Fields, "phase"))
		require.Equal(t, "1", sinkFieldValue(t, entries[0].Fields, "attempt"))
		require.Equal(t, generationPhaseBackoff(0).String(), sinkFieldValue(t, entries[0].Fields, "delay"))
	})

	t.Run("ExhaustsAndReturnsLastError", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
		timerTrap := clock.Trap().NewTimer("chatworker", "generation-phase-retry")
		defer timerTrap.Close()
		starter := newGenerationPhaseTestStarter(t, clock)
		ctx := testutil.Context(t, testutil.WaitLong)
		calls := 0
		done := make(chan error, 1)
		go func() {
			_, err := retryGenerationPhase(ctx, starter, "prepare", func() (int, error) {
				calls++
				return 0, xerrors.Errorf("attempt %d", calls)
			})
			done <- err
		}()

		timerTrap.MustWait(ctx).MustRelease(ctx)
		clock.Advance(generationPhaseBackoff(0)).MustWait(ctx)
		timerTrap.MustWait(ctx).MustRelease(ctx)
		clock.Advance(generationPhaseBackoff(1)).MustWait(ctx)
		require.EqualError(t, <-done, "attempt 3")
		require.Equal(t, generationPhaseMaxAttempts, calls)
	})

	t.Run("TerminalShortCircuits", func(t *testing.T) {
		t.Parallel()
		starter := newGenerationPhaseTestStarter(t, quartz.NewMock(t).WithLogger(quartz.NoOpLogger))
		calls := 0
		cause := xerrors.New("deterministic")
		_, err := retryGenerationPhase(context.Background(), starter, "prepare", func() (int, error) {
			calls++
			return 0, terminalGeneration(cause)
		})
		require.ErrorIs(t, err, cause)
		require.True(t, isTerminalGeneration(err))
		require.Equal(t, 1, calls)
	})

	t.Run("ContextCanceledExitsCleanly", func(t *testing.T) {
		t.Parallel()
		starter := newGenerationPhaseTestStarter(t, quartz.NewMock(t).WithLogger(quartz.NoOpLogger))
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		calls := 0
		_, err := retryGenerationPhase(ctx, starter, "prepare", func() (int, error) {
			calls++
			return 0, xerrors.New("transient")
		})
		require.ErrorIs(t, err, errTaskExpectedExit)
		require.Equal(t, 1, calls)
	})

	t.Run("WaitCancellationExitsCleanly", func(t *testing.T) {
		t.Parallel()
		clock := quartz.NewMock(t).WithLogger(quartz.NoOpLogger)
		timerTrap := clock.Trap().NewTimer("chatworker", "generation-phase-retry")
		defer timerTrap.Close()
		starter := newGenerationPhaseTestStarter(t, clock)
		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
		calls := 0
		done := make(chan error, 1)
		go func() {
			_, err := retryGenerationPhase(ctx, starter, "prepare", func() (int, error) {
				calls++
				return 0, xerrors.New("transient")
			})
			done <- err
		}()

		timerTrap.MustWait(ctx).MustRelease(ctx)
		cancel()
		require.ErrorIs(t, <-done, errTaskExpectedExit)
		require.Equal(t, 1, calls)
	})
}

type phaseRetryResult[T any] struct {
	value T
	err   error
}

func newGenerationPhaseTestStarter(t *testing.T, clock quartz.Clock) *taskStarter {
	t.Helper()
	require.NotNil(t, clock)
	return &taskStarter{opts: chatWorkerOptions{
		Clock:                   clock,
		Logger:                  testutil.NewFakeSink(t).Logger(),
		TaskRetryInitialBackoff: time.Millisecond,
		TaskRetryMaxBackoff:     time.Millisecond,
	}}
}
