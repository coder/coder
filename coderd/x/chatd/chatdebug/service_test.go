package chatdebug_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbmock"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	dbpubsub "github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/x/chatd/chatdebug"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

type testFixture struct {
	ctx      context.Context
	db       database.Store
	sqlDB    *sql.DB
	svc      *chatdebug.Service
	org      database.Organization
	owner    database.User
	chat     database.Chat
	model    database.ChatModelConfig
	provider string
}

func TestService_IsEnabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, _ := dbtestutil.NewDBWithSQLDB(t)
	_, owner, chat, model := seedChat(t, db)
	require.NotEqual(t, uuid.Nil, model.ID)

	svc := chatdebug.NewService(db, testutil.Logger(t), nil)

	// Default is off until an admin allows user opt-in.
	require.False(t, svc.IsEnabled(ctx, chat.ID, owner.ID))

	err := db.UpsertChatDebugLoggingAllowUsers(ctx, true)
	require.NoError(t, err)
	// Allowing user opt-in is not enough on its own; the user must opt in.
	require.False(t, svc.IsEnabled(ctx, chat.ID, owner.ID))
	require.False(t, svc.IsEnabled(ctx, chat.ID, uuid.Nil))

	err = db.UpsertUserChatDebugLoggingEnabled(ctx,
		database.UpsertUserChatDebugLoggingEnabledParams{
			UserID:              owner.ID,
			DebugLoggingEnabled: true,
		},
	)
	require.NoError(t, err)
	require.True(t, svc.IsEnabled(ctx, chat.ID, owner.ID))

	err = db.UpsertUserChatDebugLoggingEnabled(ctx,
		database.UpsertUserChatDebugLoggingEnabledParams{
			UserID:              owner.ID,
			DebugLoggingEnabled: false,
		},
	)
	require.NoError(t, err)
	require.False(t, svc.IsEnabled(ctx, chat.ID, owner.ID))
}

func TestService_IsEnabled_AlwaysEnable(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, _ := dbtestutil.NewDBWithSQLDB(t)
	_, owner, chat, model := seedChat(t, db)
	require.NotEqual(t, uuid.Nil, model.ID)

	svc := chatdebug.NewService(db, testutil.Logger(t), nil, chatdebug.WithAlwaysEnable(true))
	require.True(t, svc.IsEnabled(ctx, chat.ID, owner.ID))
	require.True(t, svc.IsEnabled(ctx, chat.ID, uuid.Nil))
}

func TestService_IsEnabled_ZeroValueService(t *testing.T) {
	t.Parallel()

	var svc *chatdebug.Service
	require.False(t, svc.IsEnabled(context.Background(), uuid.Nil, uuid.Nil))

	require.False(t, (&chatdebug.Service{}).IsEnabled(context.Background(), uuid.Nil, uuid.Nil))
}

func TestService_CreateRun(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	rootChat := insertChat(t, fixture.db, fixture.org.ID, fixture.owner.ID, fixture.model.ID)
	parentChat := insertChat(t, fixture.db, fixture.org.ID, fixture.owner.ID, fixture.model.ID)
	triggerMsg := insertMessage(t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleUser, "trigger")
	historyTipMsg := insertMessage(t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleAssistant,
		"history-tip")

	run, err := fixture.svc.CreateRun(fixture.ctx, chatdebug.CreateRunParams{
		ChatID:              fixture.chat.ID,
		RootChatID:          rootChat.ID,
		ParentChatID:        parentChat.ID,
		ModelConfigID:       fixture.model.ID,
		TriggerMessageID:    triggerMsg.ID,
		HistoryTipMessageID: historyTipMsg.ID,
		Kind:                chatdebug.KindChatTurn,
		Status:              chatdebug.StatusInProgress,
		Provider:            fixture.provider,
		Model:               fixture.model.Model,
		Summary: map[string]any{
			"phase": "create",
			"count": 1,
		},
	})
	require.NoError(t, err)
	assertRunMatches(t, run, fixture.chat.ID, rootChat.ID, parentChat.ID,
		fixture.model.ID, triggerMsg.ID, historyTipMsg.ID,
		chatdebug.KindChatTurn, chatdebug.StatusInProgress,
		fixture.provider, fixture.model.Model,
		`{"count":1,"phase":"create"}`)

	stored, err := fixture.db.GetChatDebugRunByID(fixture.ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, run.ID, stored.ID)
	require.JSONEq(t, string(run.Summary), string(stored.Summary))
}

func TestService_CreateRun_TypedNilSummaryUsesDefaultObject(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	var summary map[string]any

	run, err := fixture.svc.CreateRun(fixture.ctx, chatdebug.CreateRunParams{
		ChatID:  fixture.chat.ID,
		Kind:    chatdebug.KindChatTurn,
		Status:  chatdebug.StatusInProgress,
		Summary: summary,
	})
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(run.Summary))
}

func TestService_UpdateRun(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run, err := fixture.svc.CreateRun(fixture.ctx, chatdebug.CreateRunParams{
		ChatID: fixture.chat.ID,
		Kind:   chatdebug.KindChatTurn,
		Status: chatdebug.StatusInProgress,
		Summary: map[string]any{
			"before": true,
		},
	})
	require.NoError(t, err)

	finishedAt := time.Now().UTC().Round(time.Microsecond)
	updated, err := fixture.svc.UpdateRun(fixture.ctx, chatdebug.UpdateRunParams{
		ID:         run.ID,
		ChatID:     fixture.chat.ID,
		Status:     chatdebug.StatusCompleted,
		Summary:    map[string]any{"after": "done"},
		FinishedAt: finishedAt,
	})
	require.NoError(t, err)
	require.Equal(t, string(chatdebug.StatusCompleted), updated.Status)
	require.True(t, updated.FinishedAt.Valid)
	require.WithinDuration(t, finishedAt, updated.FinishedAt.Time, time.Second)
	require.JSONEq(t, `{"after":"done"}`, string(updated.Summary))

	stored, err := fixture.db.GetChatDebugRunByID(fixture.ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, string(chatdebug.StatusCompleted), stored.Status)
	require.JSONEq(t, `{"after":"done"}`, string(stored.Summary))
	require.True(t, stored.FinishedAt.Valid)
}

func TestService_UpdateRun_AutoFillsFinishedAtOnTerminalStatus(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run, err := fixture.svc.CreateRun(fixture.ctx, chatdebug.CreateRunParams{
		ChatID: fixture.chat.ID,
		Kind:   chatdebug.KindChatTurn,
		Status: chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	// Pass a terminal status without FinishedAt. The service must
	// auto-fill it so the run is immediately visible to the
	// InsertChatDebugStep atomic guard (finished_at IS NULL).
	// Truncate to microsecond precision to match Postgres timestamptz
	// resolution; without this, nanosecond-precise Go timestamps can
	// appear strictly after a round-tripped value in the same
	// microsecond.
	before := time.Now().Truncate(time.Microsecond)
	updated, err := fixture.svc.UpdateRun(fixture.ctx, chatdebug.UpdateRunParams{
		ID:     run.ID,
		ChatID: fixture.chat.ID,
		Status: chatdebug.StatusCompleted,
	})
	require.NoError(t, err)
	require.Equal(t, string(chatdebug.StatusCompleted), updated.Status)
	require.True(t, updated.FinishedAt.Valid,
		"FinishedAt must be auto-filled for terminal status")
	require.False(t, updated.FinishedAt.Time.Before(before),
		"auto-filled FinishedAt should not be earlier than test start")
}

func TestService_UpdateRun_FinishedAtIsWriteOnce(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run, err := fixture.svc.CreateRun(fixture.ctx, chatdebug.CreateRunParams{
		ChatID: fixture.chat.ID,
		Kind:   chatdebug.KindChatTurn,
		Status: chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	// First finalization stamps finished_at with an explicit value so
	// the test is independent of wall-clock timing.
	originalFinishedAt := time.Now().UTC().
		Truncate(time.Microsecond).Add(-time.Hour)
	first, err := fixture.svc.UpdateRun(fixture.ctx, chatdebug.UpdateRunParams{
		ID:         run.ID,
		ChatID:     fixture.chat.ID,
		Status:     chatdebug.StatusCompleted,
		FinishedAt: originalFinishedAt,
	})
	require.NoError(t, err)
	require.True(t, first.FinishedAt.Valid)
	require.True(t, first.FinishedAt.Time.Equal(originalFinishedAt))

	// A later summary refresh on the already-finalized run must not
	// overwrite the original completion timestamp, even though the
	// service auto-fills FinishedAt with clock.Now() whenever a
	// terminal status is passed. Without the SQL write-once guard,
	// this second call would clobber finished_at with the current
	// time and corrupt duration/ordering calculations.
	second, err := fixture.svc.UpdateRun(fixture.ctx, chatdebug.UpdateRunParams{
		ID:      run.ID,
		ChatID:  fixture.chat.ID,
		Status:  chatdebug.StatusCompleted,
		Summary: map[string]any{"refreshed": true},
	})
	require.NoError(t, err)
	require.True(t, second.FinishedAt.Valid)
	require.True(t, second.FinishedAt.Time.Equal(originalFinishedAt),
		"FinishedAt must be preserved across repeated terminal-status updates")

	// Even a caller that explicitly passes a new FinishedAt cannot
	// overwrite the original.
	override := originalFinishedAt.Add(time.Hour)
	third, err := fixture.svc.UpdateRun(fixture.ctx, chatdebug.UpdateRunParams{
		ID:         run.ID,
		ChatID:     fixture.chat.ID,
		Status:     chatdebug.StatusCompleted,
		FinishedAt: override,
	})
	require.NoError(t, err)
	require.True(t, third.FinishedAt.Time.Equal(originalFinishedAt),
		"explicit FinishedAt must not overwrite an already-set value")
}

func TestService_CreateStep(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)
	historyTipMsg := insertMessage(t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleAssistant,
		"history-tip")

	step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:               run.ID,
		ChatID:              fixture.chat.ID,
		Operation:           chatdebug.OperationStream,
		Status:              chatdebug.StatusInProgress,
		HistoryTipMessageID: historyTipMsg.ID,
		NormalizedRequest: map[string]any{
			"messages": []string{"hello"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, fixture.chat.ID, step.ChatID)
	require.Equal(t, run.ID, step.RunID)
	require.EqualValues(t, 1, step.StepNumber)
	require.Equal(t, string(chatdebug.OperationStream), step.Operation)
	require.Equal(t, string(chatdebug.StatusInProgress), step.Status)
	require.True(t, step.HistoryTipMessageID.Valid)
	require.Equal(t, historyTipMsg.ID, step.HistoryTipMessageID.Int64)
	require.JSONEq(t, `{"messages":["hello"]}`, string(step.NormalizedRequest))

	steps, err := fixture.db.GetChatDebugStepsByRunID(fixture.ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	require.Equal(t, step.ID, steps[0].ID)
}

func TestService_CreateStep_NumbersStepsPerRun(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	first := createRun(t, fixture)
	second := createRun(t, fixture)

	for _, tc := range []struct {
		runID uuid.UUID
		want  int32
	}{{first.ID, 1}, {second.ID, 1}, {first.ID, 2}} {
		step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
			RunID:     tc.runID,
			ChatID:    fixture.chat.ID,
			Operation: chatdebug.OperationStream,
			Status:    chatdebug.StatusInProgress,
		})
		require.NoError(t, err)
		require.Equal(t, tc.want, step.StepNumber)
	}
}

// Creates that wait on a locked run all start before any step exists.
// Each must still get its own number, however many of them there are.
func TestService_CreateStep_NumbersConcurrentSteps(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)

	tx, err := fixture.sqlDB.BeginTx(fixture.ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(fixture.ctx, "SELECT 1 FROM chat_debug_runs WHERE id = $1 FOR UPDATE", run.ID)
	require.NoError(t, err)

	const creates = 20
	errs := make(chan error, creates)
	for range creates {
		go func() {
			_, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
				RunID:     run.ID,
				ChatID:    fixture.chat.ID,
				Operation: chatdebug.OperationStream,
				Status:    chatdebug.StatusInProgress,
			})
			errs <- err
		}()
	}
	// Release the run only after every create waits on a lock.
	require.Eventually(t, func() bool {
		if len(errs) > 0 {
			return true
		}
		var waiting int
		err := fixture.sqlDB.QueryRowContext(fixture.ctx, `SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'`).Scan(&waiting)
		return err == nil && waiting == creates
	}, testutil.WaitShort, testutil.IntervalFast)
	require.NoError(t, tx.Commit())

	for range creates {
		require.NoError(t, testutil.RequireReceive(fixture.ctx, t, errs))
	}
	steps, err := fixture.db.GetChatDebugStepsByRunID(fixture.ctx, run.ID)
	require.NoError(t, err)
	got := make([]int32, 0, len(steps))
	for _, step := range steps {
		got = append(got, step.StepNumber)
	}
	want := make([]int32, 0, creates)
	for n := range int32(creates) {
		want = append(want, n+1)
	}
	require.Equal(t, want, got)
}

func TestService_CreateStep_RejectsFinalizedRun(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)

	// Finalize the run so it has a terminal state.
	_, err := fixture.svc.UpdateRun(fixture.ctx, chatdebug.UpdateRunParams{
		ID:         run.ID,
		ChatID:     fixture.chat.ID,
		Status:     chatdebug.StatusInterrupted,
		FinishedAt: time.Now(),
	})
	require.NoError(t, err)

	// Creating a step on the finalized run must fail.
	_, err = fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     run.ID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationStream,
		Status:    chatdebug.StatusInProgress,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "already finalized")
}

func TestService_CreateStep_MissingRunReportsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)

	// Use a random run ID that was never inserted. The insert CTE
	// returns zero rows, which must be classified as "not found"
	// instead of being conflated with the already-finalized case.
	_, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     uuid.New(),
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationStream,
		Status:    chatdebug.StatusInProgress,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "not found",
		"missing parent runs must surface as not-found, not already-finalized")
	require.NotContains(t, err.Error(), "already finalized")
}

func TestService_CreateStep_ChatIDMismatchReportsNotFound(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)

	// Create a second chat under the same owner/model and try to
	// attach a step to the existing run using the wrong chat_id.
	// The insert's locked_run WHERE fails on chat_id, producing
	// sql.ErrNoRows; classifyMissingRun must report not-found.
	otherChat := insertChat(t, fixture.db, fixture.org.ID,
		fixture.owner.ID, fixture.model.ID)

	_, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     run.ID,
		ChatID:    otherChat.ID,
		Operation: chatdebug.OperationStream,
		Status:    chatdebug.StatusInProgress,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "not found",
		"chat_id mismatch must surface as not-found, not already-finalized")
	require.NotContains(t, err.Error(), "already finalized")
}

func TestService_UpdateStep(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)
	step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     run.ID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationStream,
		Status:    chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	assistantMsg := insertMessage(t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleAssistant,
		"assistant")
	finishedAt := time.Now().UTC().Round(time.Microsecond)
	updated, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:                 step.ID,
		ChatID:             fixture.chat.ID,
		Status:             chatdebug.StatusCompleted,
		AssistantMessageID: assistantMsg.ID,
		NormalizedResponse: map[string]any{"text": "done"},
		Usage:              map[string]any{"input_tokens": 10, "output_tokens": 5},
		Attempts: []chatdebug.Attempt{{
			Number:         1,
			ResponseStatus: 200,
			DurationMs:     25,
		}},
		Metadata:   map[string]any{"provider": fixture.provider},
		FinishedAt: finishedAt,
	})
	require.NoError(t, err)
	require.Equal(t, string(chatdebug.StatusCompleted), updated.Status)
	require.True(t, updated.AssistantMessageID.Valid)
	require.Equal(t, assistantMsg.ID, updated.AssistantMessageID.Int64)
	require.True(t, updated.NormalizedResponse.Valid)
	require.JSONEq(t, `{"text":"done"}`,
		string(updated.NormalizedResponse.RawMessage))
	require.True(t, updated.Usage.Valid)
	require.JSONEq(t, `{"input_tokens":10,"output_tokens":5}`,
		string(updated.Usage.RawMessage))
	require.JSONEq(t,
		`[{"number":1,"response_status":200,"duration_ms":25}]`,
		string(updated.Attempts),
	)
	require.JSONEq(t, `{"provider":"`+fixture.provider+`"}`,
		string(updated.Metadata))
	require.True(t, updated.FinishedAt.Valid)
	storedSteps, err := fixture.db.GetChatDebugStepsByRunID(fixture.ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, storedSteps, 1)
	require.Equal(t, updated.ID, storedSteps[0].ID)
}

func TestService_UpdateStep_AutoFillsFinishedAtOnTerminalStatus(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)
	step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     run.ID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationStream,
		Status:    chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	// Pass a terminal status without FinishedAt. The service must
	// auto-fill it so terminal rows do not keep finished_at = NULL.
	// Truncate to microsecond precision to match Postgres timestamptz
	// resolution.
	before := time.Now().Truncate(time.Microsecond)
	updated, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:     step.ID,
		ChatID: fixture.chat.ID,
		Status: chatdebug.StatusError,
	})
	require.NoError(t, err)
	require.Equal(t, string(chatdebug.StatusError), updated.Status)
	require.True(t, updated.FinishedAt.Valid,
		"FinishedAt must be auto-filled for terminal status")
	require.False(t, updated.FinishedAt.Time.Before(before),
		"auto-filled FinishedAt should not be earlier than test start")
}

func TestService_UpdateStep_TypedNilAttemptsPreserveExistingValue(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)
	step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     run.ID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationStream,
		Status:    chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	_, err = fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:     step.ID,
		ChatID: fixture.chat.ID,
		Status: chatdebug.StatusCompleted,
		Attempts: []chatdebug.Attempt{{
			Number: 1,
		}},
	})
	require.NoError(t, err)

	var typedNilAttempts []chatdebug.Attempt
	updated, err := fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:       step.ID,
		ChatID:   fixture.chat.ID,
		Attempts: typedNilAttempts,
	})
	require.NoError(t, err)

	var attempts []map[string]any
	require.NoError(t, json.Unmarshal(updated.Attempts, &attempts))
	require.Len(t, attempts, 1)
	require.EqualValues(t, 1, attempts[0]["number"])
}

func TestService_DeleteByChatID(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)
	_, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     run.ID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationGenerate,
		Status:    chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	deleted, err := fixture.svc.DeleteByChatID(fixture.ctx, fixture.chat.ID,
		time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	runs, err := fixture.db.GetChatDebugRunsByChatID(fixture.ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID:   fixture.chat.ID,
		LimitVal: 100,
	})
	require.NoError(t, err)
	require.Empty(t, runs)
}

func TestService_DeleteAfterMessageID(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	low := insertMessage(t, fixture.db, fixture.chat.ID, fixture.owner.ID,
		fixture.model.ID, database.ChatMessageRoleAssistant, "low")
	threshold := insertMessage(t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleAssistant,
		"threshold")
	high := insertMessage(t, fixture.db, fixture.chat.ID, fixture.owner.ID,
		fixture.model.ID, database.ChatMessageRoleAssistant, "high")
	require.Less(t, low.ID, threshold.ID)
	require.Less(t, threshold.ID, high.ID)

	runKeep := createRun(t, fixture)
	stepKeep, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     runKeep.ID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationGenerate,
		Status:    chatdebug.StatusInProgress,
	})
	require.NoError(t, err)
	_, err = fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:                 stepKeep.ID,
		ChatID:             fixture.chat.ID,
		AssistantMessageID: low.ID,
	})
	require.NoError(t, err)

	runDelete := createRun(t, fixture)
	stepDelete, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:     runDelete.ID,
		ChatID:    fixture.chat.ID,
		Operation: chatdebug.OperationGenerate,
		Status:    chatdebug.StatusInProgress,
	})
	require.NoError(t, err)
	_, err = fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:                 stepDelete.ID,
		ChatID:             fixture.chat.ID,
		AssistantMessageID: high.ID,
	})
	require.NoError(t, err)

	deleted, err := fixture.svc.DeleteAfterMessageID(fixture.ctx, fixture.chat.ID,
		threshold.ID, time.Now().Add(time.Minute))
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)

	runs, err := fixture.db.GetChatDebugRunsByChatID(fixture.ctx, database.GetChatDebugRunsByChatIDParams{
		ChatID:   fixture.chat.ID,
		LimitVal: 100,
	})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, runKeep.ID, runs[0].ID)

	steps, err := fixture.db.GetChatDebugStepsByRunID(fixture.ctx, runKeep.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	require.Equal(t, stepKeep.ID, steps[0].ID)
}

func TestClassifyError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want chatdebug.Status
	}{
		{"nil", nil, chatdebug.StatusCompleted},
		{"context.Canceled", context.Canceled, chatdebug.StatusInterrupted},
		// Wrapped context.Canceled must still classify as interrupted so
		// callers that decorate cancellation errors do not flip to
		// StatusError.
		{
			"wrapped context.Canceled",
			xerrors.Errorf("canceled mid-stream: %w", context.Canceled),
			chatdebug.StatusInterrupted,
		},
		{"generic error", xerrors.New("boom"), chatdebug.StatusError},
		// context.DeadlineExceeded is not context.Canceled and is not
		// special-cased by ClassifyError, so it must fall through to
		// StatusError. This pins the priority ordering in the switch.
		{
			"context.DeadlineExceeded",
			context.DeadlineExceeded, chatdebug.StatusError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatdebug.ClassifyError(tt.err))
		})
	}
}

func TestService_FinalizeRun_FallsBackToSeedSummary(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)

	runID := uuid.New()
	chatID := uuid.New()
	seed := map[string]any{"first_message": "hello"}

	// Force AggregateRunSummary to fail by returning an error from the
	// step fetch it depends on. FinalizeRun must log the warning and
	// continue with the caller-supplied SeedSummary.
	db.EXPECT().
		GetChatDebugStepsByRunID(gomock.Any(), runID).
		Return(nil, xerrors.New("boom"))

	db.EXPECT().
		UpdateChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, arg database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			require.Equal(t, runID, arg.ID)
			require.Equal(t, chatID, arg.ChatID)
			require.True(t, arg.Summary.Valid)
			var got map[string]any
			require.NoError(t, json.Unmarshal(arg.Summary.RawMessage, &got))
			require.Equal(t, "hello", got["first_message"])
			return database.ChatDebugRun{
				ID:     runID,
				ChatID: chatID,
			}, nil
		})

	err := svc.FinalizeRun(context.Background(), chatdebug.FinalizeRunParams{
		RunID:       runID,
		ChatID:      chatID,
		Status:      chatdebug.StatusCompleted,
		SeedSummary: seed,
	})
	require.NoError(t, err)
}

func TestService_FinalizeRun_ReturnsWrappedUpdateError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)

	runID := uuid.New()
	chatID := uuid.New()

	db.EXPECT().
		GetChatDebugStepsByRunID(gomock.Any(), runID).
		Return(nil, nil)
	db.EXPECT().
		UpdateChatDebugRun(gomock.Any(), gomock.Any()).
		Return(database.ChatDebugRun{}, xerrors.New("update failed"))

	err := svc.FinalizeRun(context.Background(), chatdebug.FinalizeRunParams{
		RunID:  runID,
		ChatID: chatID,
		Status: chatdebug.StatusCompleted,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "update debug run")
	require.Contains(t, err.Error(), "update failed")
}

func TestService_FinalizeRun_CustomTimeoutAppliesToDBCalls(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)

	runID := uuid.New()
	chatID := uuid.New()
	customTimeout := 123 * time.Millisecond
	// Allow for scheduling jitter but ensure the custom timeout is
	// honored rather than the 5s default. Both DB calls receive the
	// same timeout-bounded context.
	maxRemaining := customTimeout + 50*time.Millisecond

	db.EXPECT().
		GetChatDebugStepsByRunID(gomock.Any(), runID).
		DoAndReturn(func(ctx context.Context, _ uuid.UUID) ([]database.ChatDebugStep, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "FinalizeRun must apply its Timeout to aggregation context")
			require.LessOrEqual(t, time.Until(deadline), maxRemaining)
			return nil, nil
		})
	db.EXPECT().
		UpdateChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "FinalizeRun must apply its Timeout to update context")
			require.LessOrEqual(t, time.Until(deadline), maxRemaining)
			return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
		})

	err := svc.FinalizeRun(context.Background(), chatdebug.FinalizeRunParams{
		RunID:   runID,
		ChatID:  chatID,
		Status:  chatdebug.StatusCompleted,
		Timeout: customTimeout,
	})
	require.NoError(t, err)
}

func TestService_FinalizeRun_DetachesFromParentCancellation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)

	runID := uuid.New()
	chatID := uuid.New()

	// FinalizeRun uses context.WithoutCancel so a canceled parent must
	// not propagate to the DB calls. Verify both calls see a live
	// context with the FinalizeRun-owned deadline.
	parentCtx, cancel := context.WithCancel(context.Background())
	cancel()

	db.EXPECT().
		GetChatDebugStepsByRunID(gomock.Any(), runID).
		DoAndReturn(func(ctx context.Context, _ uuid.UUID) ([]database.ChatDebugStep, error) {
			require.NoError(t, ctx.Err(),
				"aggregation context must not inherit parent cancellation")
			_, ok := ctx.Deadline()
			require.True(t, ok)
			return nil, nil
		})
	db.EXPECT().
		UpdateChatDebugRun(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
			require.NoError(t, ctx.Err(),
				"update context must not inherit parent cancellation")
			return database.ChatDebugRun{ID: runID, ChatID: chatID}, nil
		})

	err := svc.FinalizeRun(parentCtx, chatdebug.FinalizeRunParams{
		RunID:  runID,
		ChatID: chatID,
		Status: chatdebug.StatusCompleted,
	})
	require.NoError(t, err)
}

func TestService_StartRun(t *testing.T) {
	t.Parallel()

	params := chatdebug.CreateRunParams{
		ChatID:              uuid.New(),
		RootChatID:          uuid.New(),
		ParentChatID:        uuid.New(),
		ModelConfigID:       uuid.New(),
		TriggerMessageID:    3,
		HistoryTipMessageID: 5,
		Kind:                chatdebug.KindQuickgen,
		Status:              chatdebug.StatusInProgress,
		Provider:            "openai",
		Model:               "gpt-test",
	}
	tests := []struct {
		name        string
		createErr   error
		finalizeErr error
		wantWarning string
	}{
		{name: "Finalizes"},
		{name: "CreateFails", createErr: xerrors.New("insert failed"), wantWarning: "failed to create debug run"},
		{name: "FinalizeFails", finalizeErr: xerrors.New("update failed"), wantWarning: "failed to finalize debug run"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			db := dbmock.NewMockStore(gomock.NewController(t))
			sink := testutil.NewFakeSink(t)
			svc := chatdebug.NewService(db, sink.Logger(slog.LevelWarn), nil)
			callerCtx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitLong))
			runID := uuid.New()
			db.EXPECT().InsertChatDebugRun(gomock.Any(), gomock.Any()).DoAndReturn(
				func(ctx context.Context, got database.InsertChatDebugRunParams) (database.ChatDebugRun, error) {
					cancel()
					require.NoError(t, ctx.Err())
					_, ok := ctx.Deadline()
					require.True(t, ok)
					require.JSONEq(t, `{"first_message":"hello"}`, string(got.Summary.RawMessage))
					return database.ChatDebugRun{ID: runID, ChatID: got.ChatID}, tc.createErr
				})
			if tc.createErr == nil {
				db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, nil)
				db.EXPECT().UpdateChatDebugRun(gomock.Any(), gomock.Any()).DoAndReturn(
					func(ctx context.Context, got database.UpdateChatDebugRunParams) (database.ChatDebugRun, error) {
						deadline, ok := ctx.Deadline()
						require.True(t, ok)
						require.LessOrEqual(t, time.Until(deadline), time.Second)
						require.Equal(t, runID, got.ID)
						require.Equal(t, params.ChatID, got.ChatID)
						require.Equal(t, sql.NullString{String: string(chatdebug.StatusError), Valid: true}, got.Status)
						require.Contains(t, string(got.Summary.RawMessage), `"first_message":"hello"`)
						return database.ChatDebugRun{}, tc.finalizeErr
					})
			}

			ctx, finish := svc.StartRun(callerCtx, params, map[string]any{"first_message": "hello"}, time.Second)
			run, ok := chatdebug.RunFromContext(ctx)
			require.Equal(t, tc.createErr == nil, ok)
			if ok {
				require.Equal(t, chatdebug.RunContext{
					RunID:               runID,
					ChatID:              params.ChatID,
					RootChatID:          params.RootChatID,
					ParentChatID:        params.ParentChatID,
					ModelConfigID:       params.ModelConfigID,
					TriggerMessageID:    params.TriggerMessageID,
					HistoryTipMessageID: params.HistoryTipMessageID,
					Provider:            params.Provider,
					Model:               params.Model,
				}, *run)
			}
			finish(xerrors.New("model failed"))

			var warnings []string
			for _, entry := range sink.Entries() {
				warnings = append(warnings, entry.Message)
			}
			if tc.wantWarning == "" {
				require.Empty(t, warnings)
			} else {
				require.Equal(t, []string{tc.wantWarning}, warnings)
			}
		})
	}
}

func TestService_PublishesEvents(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	_, owner, chat, model := seedChat(t, db)
	require.NotEqual(t, uuid.Nil, owner.ID)

	memoryPubsub := dbpubsub.NewInMemory()
	svc := chatdebug.NewService(db, testutil.Logger(t), memoryPubsub)
	type eventResult struct {
		event chatdebug.DebugEvent
		err   error
	}
	events := make(chan eventResult, 1)
	cancel, err := memoryPubsub.Subscribe(chatdebug.PubsubChannel(chat.ID),
		func(_ context.Context, message []byte) {
			var event chatdebug.DebugEvent
			unmarshalErr := json.Unmarshal(message, &event)
			events <- eventResult{event: event, err: unmarshalErr}
		},
	)
	require.NoError(t, err)
	defer cancel()

	run, err := svc.CreateRun(ctx, chatdebug.CreateRunParams{
		ChatID:        chat.ID,
		ModelConfigID: model.ID,
		Kind:          chatdebug.KindChatTurn,
		Status:        chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	select {
	case received := <-events:
		require.NoError(t, received.err)
		require.Equal(t, chatdebug.EventKindRunUpdate, received.event.Kind)
		require.Equal(t, chat.ID, received.event.ChatID)
		require.Equal(t, run.ID, received.event.RunID)
		require.Equal(t, uuid.Nil, received.event.StepID)
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for debug event")
	}

	select {
	case received := <-events:
		t.Fatalf("unexpected extra event: %+v", received.event)
	default:
	}
}

func newFixture(t *testing.T) testFixture {
	t.Helper()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, sqlDB := dbtestutil.NewDBWithSQLDB(t)
	org, owner, chat, model := seedChat(t, db)
	prov, err := db.GetAIProviderByID(ctx, model.AIProviderID.UUID)
	require.NoError(t, err)
	return testFixture{
		ctx:      ctx,
		db:       db,
		sqlDB:    sqlDB,
		svc:      chatdebug.NewService(db, testutil.Logger(t), nil),
		org:      org,
		owner:    owner,
		chat:     chat,
		model:    model,
		provider: string(prov.Type),
	}
}

func seedChat(
	t *testing.T,
	db database.Store,
) (database.Organization, database.User, database.Chat, database.ChatModelConfig) {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	providerName := "openai"
	dbgen.ChatProvider(t, db, database.ChatProvider{
		Provider:    providerName,
		DisplayName: "OpenAI",
	})

	model := dbgen.ChatModelConfig(t, db, database.ChatModelConfig{
		Model:     "model-" + uuid.NewString(),
		IsDefault: true,
	})

	chat := insertChat(t, db, org.ID, owner.ID, model.ID)
	return org, owner, chat, model
}

func insertChat(
	t *testing.T,
	db database.Store,
	orgID uuid.UUID,
	ownerID uuid.UUID,
	modelID uuid.UUID,
) database.Chat {
	t.Helper()

	chat := dbgen.Chat(t, db, database.Chat{
		OrganizationID:    orgID,
		OwnerID:           ownerID,
		LastModelConfigID: modelID,
		Title:             "chat-" + uuid.NewString(),
	})
	return chat
}

func insertMessage(
	t *testing.T,
	db database.Store,
	chatID uuid.UUID,
	createdBy uuid.UUID,
	modelID uuid.UUID,
	role database.ChatMessageRole,
	text string,
) database.ChatMessage {
	t.Helper()

	parts, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{
		codersdk.ChatMessageText(text),
	})
	require.NoError(t, err)

	msg := dbgen.ChatMessage(t, db, database.ChatMessage{
		ChatID:         chatID,
		CreatedBy:      uuid.NullUUID{UUID: createdBy, Valid: true},
		ModelConfigID:  uuid.NullUUID{UUID: modelID, Valid: true},
		Role:           role,
		Content:        parts,
		ContentVersion: chatprompt.CurrentContentVersion,
	})
	return msg
}

func createRun(t *testing.T, fixture testFixture) database.ChatDebugRun {
	t.Helper()

	run, err := fixture.svc.CreateRun(fixture.ctx, chatdebug.CreateRunParams{
		ChatID:        fixture.chat.ID,
		ModelConfigID: fixture.model.ID,
		Kind:          chatdebug.KindChatTurn,
		Status:        chatdebug.StatusInProgress,
		Provider:      fixture.provider,
		Model:         fixture.model.Model,
	})
	require.NoError(t, err)
	return run
}

func assertRunMatches(
	t *testing.T,
	run database.ChatDebugRun,
	chatID uuid.UUID,
	rootChatID uuid.UUID,
	parentChatID uuid.UUID,
	modelID uuid.UUID,
	triggerMessageID int64,
	historyTipMessageID int64,
	kind chatdebug.RunKind,
	status chatdebug.Status,
	provider string,
	model string,
	summary string,
) {
	t.Helper()

	require.Equal(t, chatID, run.ChatID)
	require.True(t, run.RootChatID.Valid)
	require.Equal(t, rootChatID, run.RootChatID.UUID)
	require.True(t, run.ParentChatID.Valid)
	require.Equal(t, parentChatID, run.ParentChatID.UUID)
	require.True(t, run.ModelConfigID.Valid)
	require.Equal(t, modelID, run.ModelConfigID.UUID)
	require.True(t, run.TriggerMessageID.Valid)
	require.Equal(t, triggerMessageID, run.TriggerMessageID.Int64)
	require.True(t, run.HistoryTipMessageID.Valid)
	require.Equal(t, historyTipMessageID, run.HistoryTipMessageID.Int64)
	require.Equal(t, string(kind), run.Kind)
	require.Equal(t, string(status), run.Status)
	require.True(t, run.Provider.Valid)
	require.Equal(t, provider, run.Provider.String)
	require.True(t, run.Model.Valid)
	require.Equal(t, model, run.Model.String)
	require.JSONEq(t, summary, string(run.Summary))
	require.False(t, run.StartedAt.IsZero())
	require.False(t, run.UpdatedAt.IsZero())
	require.False(t, run.FinishedAt.Valid)
}
