package chatdebug //nolint:testpackage // Uses unexported recorder helpers.

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/testutil"
)

func TestBeginStepReuseStep(t *testing.T) {
	t.Parallel()

	t.Run("reuses handle under ReuseStep", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		ownerID := uuid.New()
		runID := uuid.New()
		t.Cleanup(func() { CleanupStepCounter(runID) })

		svc := NewService(nil, testutil.Logger(t), nil)
		ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})
		ctx = ReuseStep(ctx)
		opts := RecorderOptions{ChatID: chatID, OwnerID: ownerID}

		firstHandle, firstEnriched := beginStep(ctx, svc, opts, OperationStream, nil)
		secondHandle, secondEnriched := beginStep(ctx, svc, opts, OperationStream, nil)

		require.NotNil(t, firstHandle)
		require.Same(t, firstHandle, secondHandle)
		require.Same(t, firstHandle.stepCtx, secondHandle.stepCtx)
		require.Same(t, firstHandle.sink, secondHandle.sink)
		require.Equal(t, runID, firstHandle.stepCtx.RunID)
		require.Equal(t, chatID, firstHandle.stepCtx.ChatID)
		require.Equal(t, int32(1), firstHandle.stepCtx.StepNumber)
		require.Equal(t, OperationStream, firstHandle.stepCtx.Operation)
		require.NotEqual(t, uuid.Nil, firstHandle.stepCtx.StepID)

		firstStepCtx, ok := StepFromContext(firstEnriched)
		require.True(t, ok)
		secondStepCtx, ok := StepFromContext(secondEnriched)
		require.True(t, ok)
		require.Same(t, firstStepCtx, secondStepCtx)
		require.Same(t, firstHandle.stepCtx, firstStepCtx)
		require.Same(t, attemptSinkFromContext(firstEnriched), attemptSinkFromContext(secondEnriched))
	})

	t.Run("creates new handles without ReuseStep", func(t *testing.T) {
		t.Parallel()

		chatID := uuid.New()
		ownerID := uuid.New()
		runID := uuid.New()
		t.Cleanup(func() { CleanupStepCounter(runID) })

		svc := NewService(nil, testutil.Logger(t), nil)
		ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})
		opts := RecorderOptions{ChatID: chatID, OwnerID: ownerID}

		firstHandle, _ := beginStep(ctx, svc, opts, OperationStream, nil)
		secondHandle, _ := beginStep(ctx, svc, opts, OperationStream, nil)

		require.NotNil(t, firstHandle)
		require.NotNil(t, secondHandle)
		require.NotSame(t, firstHandle, secondHandle)
		require.NotSame(t, firstHandle.sink, secondHandle.sink)
		require.Equal(t, int32(1), firstHandle.stepCtx.StepNumber)
		require.Equal(t, int32(2), secondHandle.stepCtx.StepNumber)
		require.NotEqual(t, firstHandle.stepCtx.StepID, secondHandle.stepCtx.StepID)
	})
}
