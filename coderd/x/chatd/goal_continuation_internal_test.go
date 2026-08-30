package chatd //nolint:testpackage // Exercises unexported goal continuation helpers.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

func TestGoalHistoryEnabledRequiresGoalRow(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, _ := setupGoalTurn(ctx, t, f)
	starter := newGoalTaskStarter(t, f)

	enabled, err := starter.goalHistoryEnabled(ctx, f.db, chat)
	require.NoError(t, err)
	require.False(t, enabled, "a chat that never had a goal must skip the hidden-history read")

	insertActiveGoal(ctx, t, f, chat.ID)
	enabled, err = starter.goalHistoryEnabled(ctx, f.db, chat)
	require.NoError(t, err)
	require.True(t, enabled)
}

func TestFinishTurnContinuesActiveGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.finishGenerationTurn(ctx, machine, input, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusActive, current.Status)
	require.Equal(t, int64(1), current.ContinuationCount)

	hidden, err := f.db.GetChatHiddenUserMessagesByChatID(ctx, chat.ID)
	require.NoError(t, err)
	var continuations []database.ChatMessage
	for _, msg := range hidden {
		continuationGoalID, continuation, err := parseGoalContinuationMessage(msg)
		require.NoError(t, err)
		if continuation {
			require.Equal(t, goal.ID, continuationGoalID)
			continuations = append(continuations, msg)
		}
	}
	require.Len(t, continuations, 1)
	parts, err := chatprompt.ParseContent(continuations[0])
	require.NoError(t, err)
	text := textFromParts(parts)
	require.Contains(t, text, "complete_goal")
	require.Contains(t, text, "block_goal")
}

func TestFinishTurnPausesGoalAtContinuationCap(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	for range maxGoalContinuationTurns {
		_, err := f.db.IncrementChatGoalContinuationCount(dbauthz.AsSystemRestricted(ctx), database.IncrementChatGoalContinuationCountParams{
			RootChatID: chat.ID,
			ID:         goal.ID,
		})
		require.NoError(t, err)
	}
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.finishGenerationTurn(ctx, machine, input, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonTurnLimit), current.PausedReason.String)
}

func TestFinishTurnPausesGoalWhenNoModelConfig(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	// Disabling the only model config leaves the continuation with no
	// resolvable model. That is a durable configuration state, so the
	// goal must pause instead of rolling back the finished turn and
	// retrying the same dead end forever.
	_, err := f.db.UpdateChatModelConfig(dbauthz.AsSystemRestricted(ctx), database.UpdateChatModelConfigParams{
		ID:                   f.model.ID,
		Model:                f.model.Model,
		DisplayName:          f.model.DisplayName,
		UpdatedBy:            f.model.UpdatedBy,
		Enabled:              false,
		IsDefault:            f.model.IsDefault,
		ContextLimit:         f.model.ContextLimit,
		CompressionThreshold: f.model.CompressionThreshold,
		Options:              f.model.Options,
		AIProviderID:         f.model.AIProviderID,
	})
	require.NoError(t, err)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err = starter.finishGenerationTurn(ctx, machine, input, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusWaiting, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, goal.ID, current.ID)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonError), current.PausedReason.String)
}

func TestFinishTurnPromotedMessageWinsOverGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		_, err := tx.SendMessage(chatstate.SendMessageInput{
			Message:      userTextMessage(t, "queued while running", f.user.ID, f.model.ID, f.apiKey.ID),
			BusyBehavior: chatstate.BusyBehaviorQueue,
		})
		return err
	}))
	starter := newGoalTaskStarter(t, f)

	err := starter.finishGenerationTurn(ctx, machine, input, generationDecision{
		kind:         generationActionFinishTurn,
		finishReason: generationFinishReasonComplete,
	}, generationAttemptNotRequired)
	require.NoError(t, err)

	// The queued message was promoted; the goal is untouched and no
	// continuation message was inserted.
	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, goal.ID, current.ID)
	require.Equal(t, database.ChatGoalStatusActive, current.Status)
	require.Equal(t, int64(0), current.ContinuationCount)
	requireNoGoalContinuationMessages(ctx, t, f, chat.ID)
}

func TestFinishTurnLeavesNonActiveGoalIdle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, ctx context.Context, f *workerTestFixture, chat database.Chat, goal database.ChatGoal)
	}{
		{
			name: "PausedGoal",
			setup: func(t *testing.T, ctx context.Context, f *workerTestFixture, chat database.Chat, goal database.ChatGoal) {
				_, err := f.db.PauseChatGoalByID(dbauthz.AsSystemRestricted(ctx), database.PauseChatGoalByIDParams{
					RootChatID:   chat.ID,
					ID:           goal.ID,
					PausedReason: string(codersdk.ChatGoalPausedReasonUser),
				})
				require.NoError(t, err)
			},
		},
		{
			name: "PlanMode",
			setup: func(t *testing.T, ctx context.Context, f *workerTestFixture, chat database.Chat, _ database.ChatGoal) {
				_, err := f.db.UpdateChatPlanModeByID(dbauthz.AsSystemRestricted(ctx), database.UpdateChatPlanModeByIDParams{
					ID: chat.ID,
					PlanMode: database.NullChatPlanMode{
						ChatPlanMode: database.ChatPlanModePlan,
						Valid:        true,
					},
				})
				require.NoError(t, err)
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			f := newWorkerTestFixture(t)
			chat, input := setupGoalTurn(ctx, t, f)
			goal := insertActiveGoal(ctx, t, f, chat.ID)
			tc.setup(t, ctx, f, chat, goal)
			starter := newGoalTaskStarter(t, f)
			machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

			err := starter.finishGenerationTurn(ctx, machine, input, generationDecision{
				kind:         generationActionFinishTurn,
				finishReason: generationFinishReasonComplete,
			}, generationAttemptNotRequired)
			require.NoError(t, err)

			latest, err := f.db.GetChatByID(ctx, chat.ID)
			require.NoError(t, err)
			require.Equal(t, database.ChatStatusWaiting, latest.Status)
			requireNoGoalContinuationMessages(ctx, t, f, chat.ID)
		})
	}
}

func TestFinishErrorPausesActiveGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.finishGenerationError(ctx, machine, input, xerrors.New("model exploded"), generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonError), current.PausedReason.String)
}

func TestFinishErrorPausesActiveGoalOnUsageLimit(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.finishGenerationError(ctx, machine, input, xerrors.New("status 403: AI budget of US$10.00 exceeded"), generationAttemptNotRequired)
	require.NoError(t, err)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonUsageLimit), current.PausedReason.String)
}

// A fail-closed hook error finishes the chat in error inside the step
// commit, bypassing finishGenerationError; the goal must still pause.
func TestCommitStepFailClosedPausesActiveGoal(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	insertActiveGoal(ctx, t, f, chat.ID)
	starter := newGoalTaskStarter(t, f)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)

	err := starter.commitGenerationStep(ctx, machine, input, input.GenerationAttempt, generationActionGenerateAssistant, stepMessagesForCommit{
		Messages: []chatstate.Message{assistantTextMessage(t, "partial step", f.model.ID)},
	}, generationCommitHooks{PostCommitError: xerrors.New("hook dispatch failed")})
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusError, latest.Status)

	current, err := currentChatGoal(dbauthz.AsSystemRestricted(ctx), f.db, chat.ID)
	require.NoError(t, err)
	require.NotNil(t, current)
	require.Equal(t, database.ChatGoalStatusPaused, current.Status)
	require.Equal(t, string(codersdk.ChatGoalPausedReasonError), current.PausedReason.String)
}

// A goal continuation turn starts at a hidden boundary row. The stop
// hook must key its nudge claim from the same history the next
// generation task consumes, or the claimed nudge never buys the
// follow-up generation.
func TestStopHookNudgeKeyMatchesGoalTurn(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	f := newWorkerTestFixture(t)
	chat, input := setupGoalTurn(ctx, t, f)
	goal := insertActiveGoal(ctx, t, f, chat.ID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, _ database.Store) error {
		boundary, err := goalContinuationMessage(goal.ID, f.model.ID)
		if err != nil {
			return err
		}
		_, err = tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{boundary, assistantTextMessage(t, "goal turn answer", f.model.ID)},
		})
		return err
	}))
	chatRow, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	input.HistoryVersion = chatRow.HistoryVersion
	input.GenerationAttempt = chatRow.GenerationAttempt
	input.Status = chatRow.Status

	consumer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request agenthooks.Request
		require.NoError(t, json.NewDecoder(r.Body).Decode(&request))
		if request.Type != agenthooks.EventStop {
			_, err := w.Write([]byte(`{}`))
			require.NoError(t, err)
			return
		}
		_, err := w.Write([]byte(`{"model_context":"continue please"}`))
		require.NoError(t, err)
	}))
	t.Cleanup(consumer.Close)
	starter := newGoalTaskStarter(t, f)
	starter.server.hooks = chathooks.NewTrigger(dispatch.New(
		testutil.Logger(t),
		consumer.Client(),
		consumer.URL,
		false,
		"test-hook-secret-32-bytes-minimum!!",
		time.Second,
		"test-deployment",
		"test-version",
		prometheus.NewRegistry(),
	))

	err = starter.finishGenerationTurn(ctx, machine, input, generationDecision{}, generationAttemptNotRequired)
	require.NoError(t, err)

	latest, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	require.Equal(t, database.ChatStatusRunning, latest.Status)

	decisionMessages, err := loadDecisionMessages(ctx, f.db, chat.ID, true)
	require.NoError(t, err)
	require.True(t, input.StopNudges.consume(stopNudgeKey(decisionMessages)),
		"stop nudge claim must match the decision-path turn key")
}

func requireNoGoalContinuationMessages(ctx context.Context, t *testing.T, f *workerTestFixture, chatID uuid.UUID) {
	t.Helper()
	hidden, err := f.db.GetChatHiddenUserMessagesByChatID(ctx, chatID)
	require.NoError(t, err)
	for _, msg := range hidden {
		_, continuation, err := parseGoalContinuationMessage(msg)
		require.NoError(t, err)
		require.False(t, continuation, "unexpected continuation message %d", msg.ID)
	}
}

func setupGoalTurn(ctx context.Context, t *testing.T, f *workerTestFixture) (database.Chat, chatWorkerTaskStartInput) {
	t.Helper()
	chat := f.createRunningChat(t)
	workerID := uuid.New()
	runnerID := uuid.New()
	chat = acquireChat(t, f, chat.ID, workerID, runnerID)
	machine := chatstate.NewChatMachine(f.db, f.pubsub, chat.ID)
	require.NoError(t, machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		_, err := tx.CommitStep(chatstate.CommitStepInput{
			Messages: []chatstate.Message{assistantTextMessage(t, "done", f.model.ID)},
		})
		return err
	}))
	chat, err := f.db.GetChatByID(ctx, chat.ID)
	require.NoError(t, err)
	return chat, chatWorkerTaskStartInput{
		TaskID:            uuid.New(),
		ChatID:            chat.ID,
		WorkerID:          workerID,
		RunnerID:          runnerID,
		HistoryVersion:    chat.HistoryVersion,
		GenerationAttempt: chat.GenerationAttempt,
		Status:            chat.Status,
		StopNudges:        &stopNudgeTracker{},
	}
}

func insertActiveGoal(ctx context.Context, t *testing.T, f *workerTestFixture, rootChatID uuid.UUID) database.ChatGoal {
	t.Helper()
	goal, err := f.db.InsertActiveChatGoal(dbauthz.AsSystemRestricted(ctx), database.InsertActiveChatGoalParams{
		RootChatID:      rootChatID,
		Objective:       "finish the work",
		CreatedByUserID: f.user.ID,
	})
	require.NoError(t, err)
	return goal
}

func newGoalTaskStarter(t *testing.T, f *workerTestFixture) *taskStarter {
	t.Helper()
	logger := testutil.Logger(t)
	clock := quartz.NewReal()
	// The finish paths spawn detached finalize goroutines through
	// Server.goInflight, so the server needs a lifecycle context and
	// config cache even in tests.
	server := &Server{
		ctx:         context.Background(),
		db:          f.db,
		pubsub:      f.pubsub,
		logger:      logger,
		clock:       clock,
		configCache: newChatConfigCache(context.Background(), f.db, clock),
		experiments: codersdk.Experiments{codersdk.ExperimentChatGoals},
	}
	return &taskStarter{
		server: server,
		opts: chatWorkerOptions{
			Store:  f.db,
			Pubsub: f.pubsub,
			Logger: logger,
			Clock:  clock,
		},
		routeStateHint: func(context.Context, runnerStateUpdate) {},
	}
}
