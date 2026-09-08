package chatd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
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

func TestRunnerDebugTurnRecordMCPConnectSummaries(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	runnerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	chatID := uuid.New()
	runID := uuid.New()
	configID := uuid.New()
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	turn := newRunnerDebugTurn(runnerCtx, testutil.Logger(t))

	var seededSummary []byte
	db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params database.InsertChatDebugRunParams) (database.ChatDebugRun, error) {
			require.True(t, params.Summary.Valid)
			seededSummary = params.Summary.RawMessage
			return database.ChatDebugRun{
				ID:     runID,
				ChatID: chatID,
				Kind:   string(chatdebug.KindChatTurn),
				Status: string(chatdebug.StatusInProgress),
			}, nil
		}).
		Times(1)
	db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil).Times(1)
	var finalSummary []byte
	db.EXPECT().UpdateChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			require.True(t, params.Summary.Valid)
			finalSummary = params.Summary.RawMessage
			return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
		}).Times(1)

	debug := generationDebug{
		Enabled:          true,
		Service:          svc,
		TriggerMessageID: 1,
		ModelConfig:      database.ChatModelConfig{ID: uuid.New()},
	}
	first := []mcpclient.ConnectSummary{{
		ConfigID:   configID,
		Slug:       "registry",
		Outcome:    mcpclient.ConnectOutcomeConnected,
		DurationMS: 17,
		ToolCount:  1,
	}}
	second := []mcpclient.ConnectSummary{{
		ConfigID:   configID,
		Slug:       "registry",
		Outcome:    mcpclient.ConnectOutcomeTimeout,
		DurationMS: 10000,
		Error:      "connect: context deadline exceeded",
	}}

	// Preparation records each attempt as soon as its connect phase
	// completes; the first record creates the run with the outcome
	// already seeded, and a later Ensure must not create a second
	// run.
	turn.RecordMCPConnectSummaries(ctx, database.Chat{ID: chatID}, &debug, first)
	turn.Ensure(ctx, database.Chat{ID: chatID}, &debug)
	// A later generation step reconnects and reports a degraded
	// outcome for the same server; its action may never reach
	// Ensure, and the outcome must still survive to the finalized
	// summary.
	turn.RecordMCPConnectSummaries(ctx, database.Chat{ID: chatID}, &debug, second)
	turn.RecordOutcome(chatdebug.StatusCompleted)
	turn.Finalize(ctx)

	var seeded struct {
		MCPConnect []mcpclient.ConnectSummary `json:"mcp_connect"`
	}
	require.NoError(t, json.Unmarshal(seededSummary, &seeded))
	require.Len(t, seeded.MCPConnect, 1)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, seeded.MCPConnect[0].Outcome)

	var summary struct {
		MCPConnect []mcpclient.ConnectSummary `json:"mcp_connect"`
	}
	require.NoError(t, json.Unmarshal(finalSummary, &summary))
	require.Len(t, summary.MCPConnect, 2)
	require.Equal(t, mcpclient.ConnectOutcomeConnected, summary.MCPConnect[0].Outcome)
	require.Equal(t, mcpclient.ConnectOutcomeTimeout, summary.MCPConnect[1].Outcome)
}

func TestRunnerDebugTurnBoundsMCPConnectSummaries(t *testing.T) {
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

	var seededSummary []byte
	db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params database.InsertChatDebugRunParams) (database.ChatDebugRun, error) {
			require.True(t, params.Summary.Valid)
			seededSummary = params.Summary.RawMessage
			return database.ChatDebugRun{
				ID:     runID,
				ChatID: chatID,
				Kind:   string(chatdebug.KindChatTurn),
				Status: string(chatdebug.StatusInProgress),
			}, nil
		}).
		Times(1)
	db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil).Times(1)
	var finalSummary []byte
	db.EXPECT().UpdateChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			require.True(t, params.Summary.Valid)
			finalSummary = params.Summary.RawMessage
			return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
		}).Times(1)

	debug := generationDebug{
		Enabled:          true,
		Service:          svc,
		TriggerMessageID: 1,
		ModelConfig:      database.ChatModelConfig{ID: uuid.New()},
	}
	// 25 preparations against 5 servers produce 125 outcomes,
	// overflowing the cap. The first record creates the run.
	for prep := 0; prep < 25; prep++ {
		batch := make([]mcpclient.ConnectSummary, 5)
		for server := range batch {
			batch[server] = mcpclient.ConnectSummary{
				ConfigID:   uuid.New(),
				Slug:       fmt.Sprintf("server-%d", server),
				Outcome:    mcpclient.ConnectOutcomeConnected,
				DurationMS: int64(prep),
			}
		}
		turn.RecordMCPConnectSummaries(ctx, database.Chat{ID: chatID}, &debug, batch)
	}

	type boundedSummary struct {
		MCPConnect        []mcpclient.ConnectSummary `json:"mcp_connect"`
		MCPConnectDropped int                        `json:"mcp_connect_dropped"`
	}
	// The run was created by the first record, so the seed carries
	// only that preparation's outcomes.
	var seeded boundedSummary
	require.NoError(t, json.Unmarshal(seededSummary, &seeded))
	require.Len(t, seeded.MCPConnect, 5)
	require.Zero(t, seeded.MCPConnectDropped)

	// One more preparation still respects the cap and grows the
	// dropped count.
	turn.RecordMCPConnectSummaries(ctx, database.Chat{ID: chatID}, &debug, []mcpclient.ConnectSummary{{
		ConfigID:   uuid.New(),
		Slug:       "server-0",
		Outcome:    mcpclient.ConnectOutcomeTimeout,
		DurationMS: 10000,
	}})
	turn.RecordOutcome(chatdebug.StatusCompleted)
	turn.Finalize(ctx)

	var final boundedSummary
	require.NoError(t, json.Unmarshal(finalSummary, &final))
	require.Len(t, final.MCPConnect, maxMCPConnectSummaryEntries)
	require.Equal(t, 26, final.MCPConnectDropped)
	// The newest outcomes win: the oldest five preparations were
	// dropped, so the retained history starts at preparation 5 and
	// ends with the timeout recorded above.
	require.Equal(t, int64(5), final.MCPConnect[0].DurationMS)
	require.Equal(t, mcpclient.ConnectOutcomeTimeout, final.MCPConnect[maxMCPConnectSummaryEntries-1].Outcome)
}
