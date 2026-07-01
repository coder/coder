package chatloop

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestStartCompactionDebugRun_DoesNotReportDebugErrors(t *testing.T) {
	t.Parallel()

	newParentContext := func(chatID uuid.UUID) context.Context {
		return chatdebug.ContextWithRun(context.Background(), &chatdebug.RunContext{
			RunID:               uuid.New(),
			ChatID:              chatID,
			RootChatID:          uuid.New(),
			ParentChatID:        uuid.New(),
			ModelConfigID:       uuid.New(),
			TriggerMessageID:    41,
			HistoryTipMessageID: 42,
			Kind:                chatdebug.KindChatTurn,
			Provider:            "fake-provider",
			Model:               "fake-model",
		})
	}

	t.Run("CreateRun", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		svc := chatdebug.NewService(db, testutil.Logger(t), nil)
		chatID := uuid.New()
		reportedErr := make(chan error, 1)

		db.EXPECT().InsertChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
		).Return(database.ChatDebugRun{}, xerrors.New("insert compaction debug run"))

		ctx := newParentContext(chatID)
		compactionCtx, finish := startCompactionDebugRun(ctx, CompactionOptions{
			DebugSvc: svc,
			ChatID:   chatID,
			OnError: func(err error) {
				reportedErr <- err
			},
		})
		require.Same(t, ctx, compactionCtx)
		finish(nil)
		select {
		case err := <-reportedErr:
			t.Fatalf("unexpected OnError callback: %v", err)
		default:
		}
	})

	t.Run("FinalizeRunAggregatesSummary", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		svc := chatdebug.NewService(db, testutil.Logger(t), nil)
		chatID := uuid.New()
		runID := uuid.New()
		usageJSON, err := json.Marshal(fantasy.Usage{InputTokens: 7, OutputTokens: 3})
		require.NoError(t, err)
		attemptsJSON, err := json.Marshal([]chatdebug.Attempt{{
			Status: "completed",
			Method: "POST",
			Path:   "/v1/messages",
		}})
		require.NoError(t, err)

		db.EXPECT().InsertChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
		).Return(database.ChatDebugRun{ //nolint:exhaustruct // Test only needs IDs.
			ID:     runID,
			ChatID: chatID,
		}, nil)
		db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return([]database.ChatDebugStep{{
			ID:       uuid.New(),
			RunID:    runID,
			ChatID:   chatID,
			Status:   string(chatdebug.StatusCompleted),
			Usage:    pqtype.NullRawMessage{RawMessage: usageJSON, Valid: true},
			Attempts: attemptsJSON,
		}}, nil)
		db.EXPECT().UpdateChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.UpdateChatDebugRunParams{}),
		).DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			require.Equal(t, chatID, params.ChatID)
			require.Equal(t, runID, params.ID)
			require.True(t, params.Summary.Valid)
			require.JSONEq(t, `{"endpoint_label":"POST /v1/messages","step_count":1,"total_input_tokens":7,"total_output_tokens":3}`,
				string(params.Summary.RawMessage))
			return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
		})

		ctx := newParentContext(chatID)
		compactionCtx, finish := startCompactionDebugRun(ctx, CompactionOptions{
			DebugSvc: svc,
			ChatID:   chatID,
		})
		require.NotSame(t, ctx, compactionCtx)
		finish(nil)
	})

	t.Run("FinalizeRun", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		db := dbmock.NewMockStore(ctrl)
		svc := chatdebug.NewService(db, testutil.Logger(t), nil)
		chatID := uuid.New()
		reportedErr := make(chan error, 1)
		runID := uuid.New()

		db.EXPECT().InsertChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
		).Return(database.ChatDebugRun{ //nolint:exhaustruct // Test only needs IDs.
			ID:     runID,
			ChatID: chatID,
		}, nil)
		db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, xerrors.New("aggregate compaction debug run"))
		db.EXPECT().UpdateChatDebugRun(
			gomock.Any(),
			gomock.AssignableToTypeOf(database.UpdateChatDebugRunParams{}),
		).Return(database.ChatDebugRun{}, xerrors.New("finalize compaction debug run"))

		ctx := newParentContext(chatID)
		compactionCtx, finish := startCompactionDebugRun(ctx, CompactionOptions{
			DebugSvc: svc,
			ChatID:   chatID,
			OnError: func(err error) {
				reportedErr <- err
			},
		})
		require.NotSame(t, ctx, compactionCtx)
		finish(nil)
		select {
		case err := <-reportedErr:
			t.Fatalf("unexpected OnError callback: %v", err)
		default:
		}
	})
}

// TestGenerateCompactionSummary_PanicFinalizesAsError verifies that a
// panic originating inside the model call during compaction is
// captured by the deferred debug-run finalizer so the run is recorded
// with StatusError rather than StatusCompleted. Without the recover
// hook the named `err` return is still nil when the defer fires and
// the row silently misclassifies the crash path.
func TestGenerateCompactionSummary_PanicFinalizesAsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	chatID := uuid.New()
	runID := uuid.New()

	status := make(chan string, 1)

	db.EXPECT().InsertChatDebugRun(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.InsertChatDebugRunParams{}),
	).Return(database.ChatDebugRun{
		ID:     runID,
		ChatID: chatID,
	}, nil)
	db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil)
	db.EXPECT().UpdateChatDebugRun(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.UpdateChatDebugRunParams{}),
	).DoAndReturn(func(_ context.Context, params database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
		status <- params.Status.String
		return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
	})

	model := &chattest.FakeModel{
		ProviderName: "fake",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			panic("compaction model crash")
		},
	}

	parentCtx := chatdebug.ContextWithRun(context.Background(), &chatdebug.RunContext{
		RunID:               uuid.New(),
		ChatID:              chatID,
		ModelConfigID:       uuid.New(),
		TriggerMessageID:    1,
		HistoryTipMessageID: 2,
		Kind:                chatdebug.KindChatTurn,
		Provider:            "fake",
		Model:               "fake-model",
	})

	require.PanicsWithValue(t, "compaction model crash", func() {
		_, _ = generateCompactionSummary(parentCtx, model,
			[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
			CompactionOptions{
				DebugSvc:      svc,
				ChatID:        chatID,
				SummaryPrompt: "summarize",
				Timeout:       time.Second,
			})
	})

	select {
	case s := <-status:
		require.Equal(t, string(chatdebug.StatusError), s,
			"panic path must finalize the debug run with StatusError")
	case <-time.After(testutil.WaitShort):
		t.Fatal("FinalizeRun never reached UpdateChatDebugRun on panic")
	}
}

func TestGenerateCompaction_ForceBelowThresholdPublishesManualSource(t *testing.T) {
	t.Parallel()

	var generateCalls int
	model := &chattest.FakeModel{
		ProviderName: "fake",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			generateCalls++
			return &fantasy.Response{
				Content: fantasy.ResponseContent{
					fantasy.TextContent{Text: "manual summary"},
				},
			}, nil
		},
	}

	baseOpts := GenerateCompactionOptions{
		Model: model,
		Messages: []fantasy.Message{{
			Role: fantasy.MessageRoleUser,
			Content: []fantasy.MessagePart{
				fantasy.TextPart{Text: "hello"},
			},
		}},
		ThresholdPercent:     70,
		ContextLimitFallback: 100,
		StepUsage:            fantasy.Usage{InputTokens: 10, TotalTokens: 10},
		ToolCallID:           "summary-1",
		ToolName:             "chat_summarized",
		Source:               CompactionSourceManual,
	}

	noForce, err := GenerateCompaction(context.Background(), baseOpts)
	require.NoError(t, err)
	require.Empty(t, noForce)
	require.Equal(t, 0, generateCalls,
		"below-threshold compaction must not call the model without Force")

	var published []codersdk.ChatMessagePart
	forcedOpts := baseOpts
	forcedOpts.Force = true
	forcedOpts.PublishMessagePart = func(_ codersdk.ChatMessageRole, part codersdk.ChatMessagePart) {
		published = append(published, part)
	}

	forced, err := GenerateCompaction(context.Background(), forcedOpts)
	require.NoError(t, err)
	require.Equal(t, 1, generateCalls,
		"Force must call the model even below the automatic threshold")
	require.Equal(t, "manual summary", forced.SummaryReport)
	require.Equal(t, float64(10), forced.UsagePercent)
	require.Len(t, published, 2,
		"compaction publishes a synthetic tool call and result")
	require.Equal(t, codersdk.ChatMessagePartTypeToolResult, published[1].Type)
	require.JSONEq(t, `{
		"summary": "manual summary",
		"source": "manual",
		"threshold_percent": 70,
		"usage_percent": 10,
		"context_tokens": 10,
		"context_limit_tokens": 100
	}`, string(published[1].Result))
}
