package chatdebug //nolint:testpackage // Uses unexported recorder helpers.

import (
	"context"
	"slices"
	"sync"
	"testing"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

func TestAttemptSink_ThreadSafe(t *testing.T) {
	t.Parallel()

	const n = 256

	sink := &attemptSink{}
	var wg sync.WaitGroup

	for i := range n {
		wg.Go(func() {
			sink.record(Attempt{Number: i + 1, ResponseStatus: 200 + i})
		})
	}

	wg.Wait()

	attempts := sink.snapshot()
	require.Len(t, attempts, n)

	numbers := make([]int, 0, n)
	statuses := make([]int, 0, n)
	for _, attempt := range attempts {
		numbers = append(numbers, attempt.Number)
		statuses = append(statuses, attempt.ResponseStatus)
	}
	slices.Sort(numbers)
	slices.Sort(statuses)

	for i := range n {
		require.Equal(t, i+1, numbers[i])
		require.Equal(t, 200+i, statuses[i])
	}
}

func TestAttemptSinkContext(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	require.Nil(t, attemptSinkFromContext(ctx))

	sink := &attemptSink{}
	ctx = withAttemptSink(ctx, sink)
	require.Same(t, sink, attemptSinkFromContext(ctx))
}

func TestWrapModel_NilModel(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		WrapModel(nil, &Service{}, RecorderOptions{})
	})
}

func TestWrapModel_NilService(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{ProviderName: "provider", ModelName: "model"}
	wrapped := WrapModel(model, nil, RecorderOptions{})
	require.Same(t, model, wrapped)
}

func TestNextStepNumber_Concurrent(t *testing.T) {
	t.Parallel()

	const n = 256

	runID := uuid.New()
	t.Cleanup(func() { CleanupStepCounter(runID) })

	results := make([]int, n)
	var wg sync.WaitGroup

	for i := range n {
		wg.Go(func() {
			results[i] = int(nextStepNumber(runID))
		})
	}

	wg.Wait()

	slices.Sort(results)
	for i := range n {
		require.Equal(t, i+1, results[i])
	}
}

func TestStepFinalizeContext_StripsCancellation(t *testing.T) {
	t.Parallel()

	baseCtx, cancelBase := context.WithCancel(context.Background())
	cancelBase()
	require.ErrorIs(t, baseCtx.Err(), context.Canceled)

	finalizeCtx, cancelFinalize := stepFinalizeContext(baseCtx)
	defer cancelFinalize()

	require.NoError(t, finalizeCtx.Err())
	_, hasDeadline := finalizeCtx.Deadline()
	require.True(t, hasDeadline)
}

func TestSyncStepCounter_AdvancesCounter(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	t.Cleanup(func() { CleanupStepCounter(runID) })

	syncStepCounter(runID, 7)
	require.Equal(t, int32(8), nextStepNumber(runID))
}

func TestStepHandleFinish_NilHandle(t *testing.T) {
	t.Parallel()

	var handle *stepHandle
	handle.finish(context.Background(), StatusCompleted, nil, nil, nil, nil)
}

func TestBeginStep_NilService(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	handle, enriched := beginStep(ctx, nil, RecorderOptions{}, OperationGenerate, nil)
	require.Nil(t, handle)
	require.Nil(t, attemptSinkFromContext(enriched))
	_, ok := StepFromContext(enriched)
	require.False(t, ok)
}

func TestBeginStep_FallsBackToRunChatID(t *testing.T) {
	t.Parallel()

	runID := uuid.New()
	runChatID := uuid.New()
	ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: runChatID})

	handle, enriched := beginStep(ctx, &Service{}, RecorderOptions{}, OperationGenerate, nil)
	require.NotNil(t, handle)
	require.Equal(t, runChatID, handle.stepCtx.ChatID)

	stepCtx, ok := StepFromContext(enriched)
	require.True(t, ok)
	require.Equal(t, runChatID, stepCtx.ChatID)
}

func TestWrapModel_ReturnsDebugModel(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{ProviderName: "provider", ModelName: "model"}
	wrapped := WrapModel(model, &Service{}, RecorderOptions{})

	require.NotSame(t, model, wrapped)
	require.IsType(t, &debugModel{}, wrapped)
	require.Implements(t, (*fantasy.LanguageModel)(nil), wrapped)
	require.Equal(t, model.Provider(), wrapped.Provider())
	require.Equal(t, model.Model(), wrapped.Model())
}
