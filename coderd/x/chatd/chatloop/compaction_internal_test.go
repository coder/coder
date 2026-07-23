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

func TestGenerateCompactionSummary_UsesCallerContext(t *testing.T) {
	t.Parallel()

	type contextKey string
	testCtx := context.WithValue(context.Background(), contextKey("key"), "value")
	var ctxSeen context.Context
	model := &chattest.FakeModel{
		ProviderName: "fake",
		GenerateFn: func(ctx context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			ctxSeen = ctx
			return &fantasy.Response{
				Content: []fantasy.Content{
					fantasy.TextContent{Text: "summary"},
				},
			}, nil
		},
	}

	summary, err := generateCompactionSummary(testCtx, model,
		[]fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		CompactionOptions{SummaryPrompt: "summarize"},
	)
	require.NoError(t, err)
	require.Equal(t, "summary", summary)
	require.Same(t, testCtx, ctxSeen)
	require.NoError(t, ctxSeen.Err())
	_, ok := ctxSeen.Deadline()
	require.False(t, ok)
	require.Equal(t, "value", ctxSeen.Value(contextKey("key")))
}

// TestGenerateCompaction_ForceBypassesThresholdGates verifies the
// manual-compaction contract: Force runs the summary even when usage
// is below threshold, when usage is zero, and when threshold=100
// disables automatic compaction; without Force those gates return an
// empty result without calling the model.
func TestGenerateCompaction_ForceBypassesThresholdGates(t *testing.T) {
	t.Parallel()

	newModel := func(calls *int) *chattest.FakeModel {
		return &chattest.FakeModel{
			ProviderName: "fake",
			ModelName:    "fake-model",
			GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
				*calls++
				return &fantasy.Response{
					Content: []fantasy.Content{
						fantasy.TextContent{Text: "forced summary"},
					},
				}, nil
			},
		}
	}
	messages := []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")}

	cases := []struct {
		name string
		opts GenerateCompactionOptions
	}{
		{
			name: "below threshold",
			opts: GenerateCompactionOptions{
				ThresholdPercent: 70,
				ContextLimit:     1000,
				StepUsage:        fantasy.Usage{InputTokens: 10},
			},
		},
		{
			name: "zero usage",
			opts: GenerateCompactionOptions{
				ThresholdPercent: 70,
				ContextLimit:     1000,
			},
		},
		{
			name: "threshold disabled",
			opts: GenerateCompactionOptions{
				ThresholdPercent: 100,
				ContextLimit:     1000,
				StepUsage:        fantasy.Usage{InputTokens: 10},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Without Force the gate returns an empty result and
			// never calls the model.
			calls := 0
			opts := tc.opts
			opts.Model = newModel(&calls)
			opts.Messages = messages
			result, err := GenerateCompaction(context.Background(), opts)
			require.NoError(t, err)
			require.Empty(t, result.SummaryReport)
			require.Zero(t, calls, "gated run must not call the model")

			// With Force the summary is generated and labeled manual.
			opts.Force = true
			opts.Source = CompactionSourceManual
			result, err = GenerateCompaction(context.Background(), opts)
			require.NoError(t, err)
			require.Equal(t, "forced summary", result.SummaryReport)
			require.Equal(t, CompactionSourceManual, result.Source)
			require.Equal(t, 1, calls, "forced run calls the model once")
		})
	}
}

// TestGenerateCompaction_DefaultSourceAutomatic verifies an unforced
// over-threshold run reports the automatic source by default.
func TestGenerateCompaction_DefaultSourceAutomatic(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: "fake",
		ModelName:    "fake-model",
		GenerateFn: func(_ context.Context, _ fantasy.Call) (*fantasy.Response, error) {
			return &fantasy.Response{
				Content: []fantasy.Content{
					fantasy.TextContent{Text: "auto summary"},
				},
			}, nil
		},
	}
	result, err := GenerateCompaction(context.Background(), GenerateCompactionOptions{
		Model:            model,
		Messages:         []fantasy.Message{textMessage(fantasy.MessageRoleUser, "hello")},
		ThresholdPercent: 70,
		ContextLimit:     100,
		StepUsage:        fantasy.Usage{InputTokens: 90},
	})
	require.NoError(t, err)
	require.Equal(t, "auto summary", result.SummaryReport)
	require.Equal(t, CompactionSourceAutomatic, result.Source)
}
