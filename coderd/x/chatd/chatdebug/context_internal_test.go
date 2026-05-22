package chatdebug

import (
	"context"
	"runtime"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestReuseStep_PreservesExistingHolder(t *testing.T) {
	t.Parallel()

	ctx := ReuseStep(context.Background())
	first, ok := reuseHolderFromContext(ctx)
	require.True(t, ok)

	reused := ReuseStep(ctx)
	second, ok := reuseHolderFromContext(reused)
	require.True(t, ok)
	require.Same(t, first, second)
}

func TestContextWithRun_CleansUpStepCounterAfterGC(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	chatID := uuid.New()
	t.Cleanup(func() { CleanupStepCounter(runID) })

	func() {
		_ = ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})
		require.Equal(t, int32(1), nextStepNumber(runID))
		_, ok := stepCounters.Load(runID)
		require.True(t, ok)
	}()

	require.Eventually(t, func() bool {
		runtime.GC() //nolint:revive // Intentional GC to test cleanup finalizer.
		runtime.Gosched()
		_, ok := stepCounters.Load(runID)
		return !ok
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestContextWithRun_MultipleInstancesSameRunID(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	chatID := uuid.New()
	t.Cleanup(func() { CleanupStepCounter(runID) })

	// rc2 is the surviving instance that should keep the step counter alive.
	rc2 := &RunContext{RunID: runID, ChatID: chatID}
	_ = ContextWithRun(context.Background(), rc2)

	// Create a second RunContext with the same RunID and let it become
	// unreachable. Its GC cleanup must NOT delete the step counter
	// because rc2 is still alive.
	func() {
		rc1 := &RunContext{RunID: runID, ChatID: chatID}
		_ = ContextWithRun(context.Background(), rc1)
		require.Equal(t, int32(1), nextStepNumber(runID))
	}()

	// Force GC to collect rc1.
	for range 5 {
		runtime.GC() //nolint:revive // Intentional GC to test cleanup finalizer.
		runtime.Gosched()
	}

	// The step counter must still be present because rc2 is alive.
	_, ok := stepCounters.Load(runID)
	require.True(t, ok, "step counter was prematurely cleaned up while another RunContext is still alive")

	// Subsequent steps on the surviving context must continue numbering.
	require.Equal(t, int32(2), nextStepNumber(runID))

	// Keep rc2 alive past the GC cycles above so the runtime cleanup
	// finalizer does not fire prematurely.
	runtime.KeepAlive(rc2)
}

func TestContextWithRun_CleansUpStepCounterOnGCAfterCancel(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	chatID := uuid.New()
	t.Cleanup(func() { CleanupStepCounter(runID) })

	// Run in a closure so the RunContext becomes unreachable after
	// context cancellation, allowing GC to trigger the cleanup.
	func() {
		ctx, cancel := context.WithCancel(context.Background())
		ContextWithRun(ctx, &RunContext{RunID: runID, ChatID: chatID})

		require.Equal(t, int32(1), nextStepNumber(runID))

		_, ok := stepCounters.Load(runID)
		require.True(t, ok)

		cancel()
	}()

	// After the closure, the RunContext is unreachable.
	// runtime.AddCleanup fires during GC.
	require.Eventually(t, func() bool {
		runtime.GC() //nolint:revive // Intentional GC to test cleanup finalizer.
		runtime.Gosched()
		_, ok := stepCounters.Load(runID)
		return !ok
	}, testutil.WaitShort, testutil.IntervalFast)

	require.Equal(t, int32(1), nextStepNumber(runID))
}
