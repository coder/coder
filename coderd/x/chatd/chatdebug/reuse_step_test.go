package chatdebug //nolint:testpackage // Uses unexported recorder helpers.

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/testutil"
)

func TestBeginStepReuseStep(t *testing.T) {
	t.Parallel()

	t.Run("reuses handle under ReuseStep", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		ownerID := uuid.New()
		runID := uuid.New()
		stepID := uuid.New()

		db.EXPECT().GetChatByID(gomock.Any(), chatID).Times(2).Return(database.Chat{
			ID:                       chatID,
			DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
		}, nil)
		db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).DoAndReturn(
			func(ctx context.Context, params database.InsertChatDebugStepParams) (database.ChatDebugStep, error) {
				require.EqualValues(t, 1, params.StepNumber)
				return database.ChatDebugStep{ID: stepID, RunID: runID, ChatID: chatID}, nil
			},
		)

		svc := NewService(db, testutil.Logger(t), nil)
		ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})
		ctx = ReuseStep(ctx)
		opts := RecorderOptions{ChatID: chatID, OwnerID: ownerID}

		firstHandle, firstEnriched := beginStep(ctx, svc, opts, OperationStream, nil)
		secondHandle, secondEnriched := beginStep(ctx, svc, opts, OperationStream, nil)

		require.NotNil(t, firstHandle)
		require.Same(t, firstHandle, secondHandle)
		require.Same(t, firstHandle.stepCtx, secondHandle.stepCtx)
		require.Same(t, firstHandle.sink, secondHandle.sink)

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

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		chatID := uuid.New()
		ownerID := uuid.New()
		runID := uuid.New()
		stepIDs := []uuid.UUID{uuid.New(), uuid.New()}
		insertCalls := 0

		db.EXPECT().GetChatByID(gomock.Any(), chatID).Times(2).Return(database.Chat{
			ID:                       chatID,
			DebugLogsEnabledOverride: sql.NullBool{Bool: true, Valid: true},
		}, nil)
		db.EXPECT().InsertChatDebugStep(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
			func(ctx context.Context, params database.InsertChatDebugStepParams) (database.ChatDebugStep, error) {
				insertCalls++
				require.EqualValues(t, insertCalls, params.StepNumber)
				return database.ChatDebugStep{ID: stepIDs[insertCalls-1], RunID: runID, ChatID: chatID}, nil
			},
		)

		svc := NewService(db, testutil.Logger(t), nil)
		ctx := ContextWithRun(context.Background(), &RunContext{RunID: runID, ChatID: chatID})
		opts := RecorderOptions{ChatID: chatID, OwnerID: ownerID}

		firstHandle, _ := beginStep(ctx, svc, opts, OperationStream, nil)
		secondHandle, _ := beginStep(ctx, svc, opts, OperationStream, nil)

		require.NotNil(t, firstHandle)
		require.NotNil(t, secondHandle)
		require.NotSame(t, firstHandle, secondHandle)
		require.NotEqual(t, firstHandle.stepCtx.StepID, secondHandle.stepCtx.StepID)
	})
}
