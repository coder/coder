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
		ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})
		handle, _ := beginStep(ctx, &Service{}, RecorderOptions{ChatID: chatID}, OperationGenerate, nil)
		require.NotNil(t, handle)
		_, ok := stepCounters.Load(runID)
		require.True(t, ok)
	}()

	require.Eventually(t, func() bool {
		runtime.GC()
		runtime.Gosched()
		_, ok := stepCounters.Load(runID)
		return !ok
	}, testutil.WaitShort, testutil.IntervalFast)
}

func TestContextWithRun_CleansUpStepCounterOnCancel(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	chatID := uuid.New()
	t.Cleanup(func() { CleanupStepCounter(runID) })

	ctx, cancel := context.WithCancel(context.Background())
	ctx = ContextWithRun(ctx, &RunContext{RunID: runID, ChatID: chatID})

	handle, _ := beginStep(ctx, &Service{}, RecorderOptions{ChatID: chatID}, OperationGenerate, nil)
	require.NotNil(t, handle)
	require.Equal(t, int32(1), handle.stepCtx.StepNumber)

	_, ok := stepCounters.Load(runID)
	require.True(t, ok)

	cancel()

	require.Eventually(t, func() bool {
		_, ok := stepCounters.Load(runID)
		return !ok
	}, testutil.WaitShort, testutil.IntervalFast)

	freshCtx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})
	freshHandle, _ := beginStep(freshCtx, &Service{}, RecorderOptions{ChatID: chatID}, OperationGenerate, nil)
	require.NotNil(t, freshHandle)
	require.Equal(t, int32(1), freshHandle.stepCtx.StepNumber)
}
