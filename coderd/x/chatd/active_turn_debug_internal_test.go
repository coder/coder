package chatd

import (
	"context"
	"database/sql"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/testutil"
)

func TestRunnerDebugTurnEnsureCreatesOnce(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	runnerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	modelConfigID := uuid.New()
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	turn := newRunnerDebugTurn(runnerCtx, testutil.Logger(t))

	db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params database.InsertChatDebugRunParams) (database.ChatDebugRun, error) {
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, string(chatdebug.KindChatTurn), params.Kind)
			require.Equal(t, string(chatdebug.StatusInProgress), params.Status)
			require.Equal(t, sql.NullInt64{Int64: 123, Valid: true}, params.TriggerMessageID)
			return database.ChatDebugRun{
				ID:                  runID,
				ChatID:              chatID,
				ModelConfigID:       uuid.NullUUID{UUID: modelConfigID, Valid: true},
				TriggerMessageID:    sql.NullInt64{Int64: 123, Valid: true},
				HistoryTipMessageID: sql.NullInt64{Int64: 456, Valid: true},
				Kind:                string(chatdebug.KindChatTurn),
				Status:              string(chatdebug.StatusInProgress),
				Provider:            sql.NullString{String: "anthropic", Valid: true},
				Model:               sql.NullString{String: "claude", Valid: true},
			}, nil
		}).Times(1)

	debug := &generationDebug{
		Enabled:             true,
		Service:             svc,
		Provider:            "anthropic",
		Model:               "claude",
		TriggerMessageID:    123,
		HistoryTipMessageID: 456,
		TriggerLabel:        "hello",
		ModelConfig:         database.ChatModelConfig{ID: modelConfigID},
	}
	chat := database.Chat{ID: chatID}

	firstCtx := turn.Ensure(ctx, chat, debug)
	firstRun, ok := chatdebug.RunFromContext(firstCtx)
	require.True(t, ok)
	require.Equal(t, runID, firstRun.RunID)

	secondCtx := turn.Ensure(ctx, chat, debug)
	secondRun, ok := chatdebug.RunFromContext(secondCtx)
	require.True(t, ok)
	require.Equal(t, runID, secondRun.RunID)
}

func TestRunnerDebugTurnEnsureDisabledFirstAttemptStaysDisabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	turn := newRunnerDebugTurn(ctx, testutil.Logger(t))
	chat := database.Chat{ID: uuid.New()}

	firstCtx := turn.Ensure(ctx, chat, nil)
	_, ok := chatdebug.RunFromContext(firstCtx)
	require.False(t, ok)

	secondCtx := turn.Ensure(ctx, chat, &generationDebug{
		Enabled:          true,
		Service:          svc,
		TriggerMessageID: 1,
		ModelConfig:      database.ChatModelConfig{ID: uuid.New()},
	})
	_, ok = chatdebug.RunFromContext(secondCtx)
	require.False(t, ok)
}

func TestRunnerDebugTurnRecordOutcomePrecedence(t *testing.T) {
	t.Parallel()

	turn := newRunnerDebugTurn(context.Background(), testutil.Logger(t))
	turn.RecordOutcome(chatdebug.StatusCompleted)
	require.True(t, turn.statusSet)
	require.Equal(t, chatdebug.StatusCompleted, turn.status)

	turn.RecordOutcome(chatdebug.StatusInterrupted)
	require.Equal(t, chatdebug.StatusInterrupted, turn.status)

	turn.RecordOutcome(chatdebug.StatusCompleted)
	require.Equal(t, chatdebug.StatusInterrupted, turn.status)

	turn.RecordOutcome(chatdebug.StatusError)
	require.Equal(t, chatdebug.StatusError, turn.status)
}

func TestRunnerDebugTurnFinalizeOnce(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	runnerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	turn := newRunnerDebugTurn(runnerCtx, testutil.Logger(t))

	db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).
		Return(database.ChatDebugRun{
			ID:     runID,
			ChatID: chatID,
			Kind:   string(chatdebug.KindChatTurn),
			Status: string(chatdebug.StatusInProgress),
		}, nil).
		Times(1)
	db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil).Times(1)
	db.EXPECT().UpdateChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			require.Equal(t, runID, params.ID)
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, sql.NullString{String: string(chatdebug.StatusError), Valid: true}, params.Status)
			return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
		}).Times(1)

	turn.Ensure(ctx, database.Chat{ID: chatID}, &generationDebug{
		Enabled:          true,
		Service:          svc,
		TriggerMessageID: 1,
		ModelConfig:      database.ChatModelConfig{ID: uuid.New()},
	})
	turn.RecordOutcome(chatdebug.StatusError)
	turn.Finalize(ctx)
	turn.Finalize(ctx)
}
