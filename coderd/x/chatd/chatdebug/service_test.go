package chatdebug_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/xerrors"

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
	ctx   context.Context
	db    database.Store
	svc   *chatdebug.Service
	org   database.Organization
	owner database.User
	chat  database.Chat
	model database.ChatModelConfig
}

func TestService_IsEnabled(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _, _ := dbtestutil.NewDBWithSQLDB(t)
	_, owner, chat, model := seedChat(ctx, t, db)
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
	_, owner, chat, model := seedChat(ctx, t, db)
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
	rootChat := insertChat(fixture.ctx, t, fixture.db, fixture.org.ID, fixture.owner.ID, fixture.model.ID)
	parentChat := insertChat(fixture.ctx, t, fixture.db, fixture.org.ID, fixture.owner.ID, fixture.model.ID)
	triggerMsg := insertMessage(fixture.ctx, t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleUser, "trigger")
	historyTipMsg := insertMessage(fixture.ctx, t, fixture.db, fixture.chat.ID,
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
		Provider:            fixture.model.Provider,
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
		fixture.model.Provider, fixture.model.Model,
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
	historyTipMsg := insertMessage(fixture.ctx, t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleAssistant,
		"history-tip")

	step, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:               run.ID,
		ChatID:              fixture.chat.ID,
		StepNumber:          1,
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

func TestService_CreateStep_RetriesDuplicateStepNumbers(t *testing.T) {
	t.Parallel()

	fixture := newFixture(t)
	run := createRun(t, fixture)

	first, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:      run.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	second, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:      run.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationGenerate,
		Status:     chatdebug.StatusInProgress,
	})
	require.NoError(t, err)
	require.EqualValues(t, 1, first.StepNumber)
	require.EqualValues(t, 2, second.StepNumber)
}

func TestService_CreateStep_ListRetryErrorWins(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	runID := uuid.New()
	chatID := uuid.New()
	listErr := xerrors.New("list chat debug steps")

	db.EXPECT().InsertChatDebugStep(
		gomock.Any(),
		gomock.AssignableToTypeOf(database.InsertChatDebugStepParams{}),
	).Return(database.ChatDebugStep{}, &pq.Error{
		Code:       pq.ErrorCode("23505"),
		Constraint: string(database.UniqueIndexChatDebugStepsRunStep),
	})
	db.EXPECT().GetChatDebugStepsByRunID(gomock.Any(), runID).Return(nil, listErr)

	_, err := svc.CreateStep(context.Background(), chatdebug.CreateStepParams{
		RunID:      runID,
		ChatID:     chatID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
	})
	require.ErrorIs(t, err, listErr)
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
		RunID:      run.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
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
		RunID:      uuid.New(),
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
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
	otherChat := insertChat(fixture.ctx, t, fixture.db, fixture.org.ID,
		fixture.owner.ID, fixture.model.ID)

	_, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:      run.ID,
		ChatID:     otherChat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
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
		RunID:      run.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	assistantMsg := insertMessage(fixture.ctx, t, fixture.db, fixture.chat.ID,
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
		Metadata:   map[string]any{"provider": fixture.model.Provider},
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
	require.JSONEq(t, `{"provider":"`+fixture.model.Provider+`"}`,
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
		RunID:      run.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	// Pass a terminal status without FinishedAt. The service must
	// auto-fill it so the stale sweep does not leave terminal rows
	// with finished_at = NULL.
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
		RunID:      run.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationStream,
		Status:     chatdebug.StatusInProgress,
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
		RunID:      run.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationGenerate,
		Status:     chatdebug.StatusInProgress,
	})
	require.NoError(t, err)

	deleted, err := fixture.svc.DeleteByChatID(fixture.ctx, fixture.chat.ID)
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
	low := insertMessage(fixture.ctx, t, fixture.db, fixture.chat.ID, fixture.owner.ID,
		fixture.model.ID, database.ChatMessageRoleAssistant, "low")
	threshold := insertMessage(fixture.ctx, t, fixture.db, fixture.chat.ID,
		fixture.owner.ID, fixture.model.ID, database.ChatMessageRoleAssistant,
		"threshold")
	high := insertMessage(fixture.ctx, t, fixture.db, fixture.chat.ID, fixture.owner.ID,
		fixture.model.ID, database.ChatMessageRoleAssistant, "high")
	require.Less(t, low.ID, threshold.ID)
	require.Less(t, threshold.ID, high.ID)

	runKeep := createRun(t, fixture)
	stepKeep, err := fixture.svc.CreateStep(fixture.ctx, chatdebug.CreateStepParams{
		RunID:      runKeep.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationGenerate,
		Status:     chatdebug.StatusInProgress,
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
		RunID:      runDelete.ID,
		ChatID:     fixture.chat.ID,
		StepNumber: 1,
		Operation:  chatdebug.OperationGenerate,
		Status:     chatdebug.StatusInProgress,
	})
	require.NoError(t, err)
	_, err = fixture.svc.UpdateStep(fixture.ctx, chatdebug.UpdateStepParams{
		ID:                 stepDelete.ID,
		ChatID:             fixture.chat.ID,
		AssistantMessageID: high.ID,
	})
	require.NoError(t, err)

	deleted, err := fixture.svc.DeleteAfterMessageID(fixture.ctx, fixture.chat.ID,
		threshold.ID)
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

func TestService_FinalizeStale_UsesConfiguredThreshold(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	db := dbmock.NewMockStore(ctrl)
	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	svc.SetStaleAfter(42 * time.Second)

	db.EXPECT().FinalizeStaleChatDebugRows(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, params database.FinalizeStaleChatDebugRowsParams) (database.FinalizeStaleChatDebugRowsRow, error) {
			require.WithinDuration(t, time.Now().Add(-42*time.Second), params.UpdatedBefore, 2*time.Second)
			return database.FinalizeStaleChatDebugRowsRow{}, nil
		},
	)

	result, err := svc.FinalizeStale(context.Background())
	require.NoError(t, err)
	require.Zero(t, result.RunsFinalized)
	require.Zero(t, result.StepsFinalized)
}

func TestService_FinalizeStale(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	_, owner, chat, model := seedChat(ctx, t, db)
	require.NotEqual(t, uuid.Nil, owner.ID)

	staleTime := time.Now().Add(-10 * time.Minute).UTC().Round(time.Microsecond)
	run, err := db.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Kind:          string(chatdebug.KindChatTurn),
		Status:        string(chatdebug.StatusInProgress),
		StartedAt:     sql.NullTime{Time: staleTime, Valid: true},
		UpdatedAt:     sql.NullTime{Time: staleTime, Valid: true},
	})
	require.NoError(t, err)
	step, err := db.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      run.ID,
		StepNumber: 1,
		Operation:  string(chatdebug.OperationStream),
		Status:     string(chatdebug.StatusInProgress),
		StartedAt:  sql.NullTime{Time: staleTime, Valid: true},
		UpdatedAt:  sql.NullTime{Time: staleTime, Valid: true},
		ChatID:     chat.ID,
	})
	require.NoError(t, err)

	svc := chatdebug.NewService(db, testutil.Logger(t), nil)
	result, err := svc.FinalizeStale(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.RunsFinalized)
	require.EqualValues(t, 1, result.StepsFinalized)

	storedRun, err := db.GetChatDebugRunByID(ctx, run.ID)
	require.NoError(t, err)
	require.Equal(t, string(chatdebug.StatusInterrupted), storedRun.Status)
	require.True(t, storedRun.FinishedAt.Valid)

	storedSteps, err := db.GetChatDebugStepsByRunID(ctx, run.ID)
	require.NoError(t, err)
	require.Len(t, storedSteps, 1)
	require.Equal(t, step.ID, storedSteps[0].ID)
	require.Equal(t, string(chatdebug.StatusInterrupted), storedSteps[0].Status)
	require.True(t, storedSteps[0].FinishedAt.Valid)
}

func TestService_FinalizeStale_BroadcastsFinalizeEvent(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	_, owner, chat, model := seedChat(ctx, t, db)
	require.NotEqual(t, uuid.Nil, owner.ID)

	staleTime := time.Now().Add(-10 * time.Minute).UTC().Round(time.Microsecond)
	run, err := db.InsertChatDebugRun(ctx, database.InsertChatDebugRunParams{
		ChatID:        chat.ID,
		ModelConfigID: uuid.NullUUID{UUID: model.ID, Valid: true},
		Kind:          string(chatdebug.KindChatTurn),
		Status:        string(chatdebug.StatusInProgress),
		StartedAt:     sql.NullTime{Time: staleTime, Valid: true},
		UpdatedAt:     sql.NullTime{Time: staleTime, Valid: true},
	})
	require.NoError(t, err)
	_, err = db.InsertChatDebugStep(ctx, database.InsertChatDebugStepParams{
		RunID:      run.ID,
		StepNumber: 1,
		Operation:  string(chatdebug.OperationStream),
		Status:     string(chatdebug.StatusInProgress),
		StartedAt:  sql.NullTime{Time: staleTime, Valid: true},
		UpdatedAt:  sql.NullTime{Time: staleTime, Valid: true},
		ChatID:     chat.ID,
	})
	require.NoError(t, err)

	memoryPubsub := dbpubsub.NewInMemory()
	svc := chatdebug.NewService(db, testutil.Logger(t), memoryPubsub)
	type eventResult struct {
		event chatdebug.DebugEvent
		err   error
	}
	events := make(chan eventResult, 1)
	cancel, err := memoryPubsub.Subscribe(chatdebug.PubsubChannel(uuid.Nil),
		func(_ context.Context, message []byte) {
			var event chatdebug.DebugEvent
			unmarshalErr := json.Unmarshal(message, &event)
			events <- eventResult{event: event, err: unmarshalErr}
		},
	)
	require.NoError(t, err)
	defer cancel()

	result, err := svc.FinalizeStale(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, result.RunsFinalized)
	require.EqualValues(t, 1, result.StepsFinalized)

	select {
	case received := <-events:
		require.NoError(t, received.err)
		require.Equal(t, chatdebug.EventKindFinalize, received.event.Kind)
		require.Equal(t, uuid.Nil, received.event.ChatID)
		require.Equal(t, uuid.Nil, received.event.RunID)
		require.Equal(t, uuid.Nil, received.event.StepID)
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out waiting for finalize event")
	}
}

func TestService_FinalizeStale_NoChangesDoesNotBroadcast(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	_, owner, chat, _ := seedChat(ctx, t, db)
	require.NotEqual(t, uuid.Nil, owner.ID)

	memoryPubsub := dbpubsub.NewInMemory()
	svc := chatdebug.NewService(db, testutil.Logger(t), memoryPubsub)
	events := make(chan chatdebug.DebugEvent, 1)
	cancel, err := memoryPubsub.Subscribe(chatdebug.PubsubChannel(uuid.Nil),
		func(_ context.Context, message []byte) {
			var event chatdebug.DebugEvent
			if err := json.Unmarshal(message, &event); err == nil {
				events <- event
			}
		},
	)
	require.NoError(t, err)
	defer cancel()

	result, err := svc.FinalizeStale(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, result.RunsFinalized)
	require.EqualValues(t, 0, result.StepsFinalized)

	select {
	case event := <-events:
		t.Fatalf("unexpected finalize event: %+v", event)
	default:
	}

	_ = chat // keep seeded chat usage explicit for test readability.
}

func TestService_PublishesEvents(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)
	db, _ := dbtestutil.NewDB(t)
	_, owner, chat, model := seedChat(ctx, t, db)
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
	db, _ := dbtestutil.NewDB(t)
	org, owner, chat, model := seedChat(ctx, t, db)
	return testFixture{
		ctx:   ctx,
		db:    db,
		svc:   chatdebug.NewService(db, testutil.Logger(t), nil),
		org:   org,
		owner: owner,
		chat:  chat,
		model: model,
	}
}

func seedChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
) (database.Organization, database.User, database.Chat, database.ChatModelConfig) {
	t.Helper()

	org := dbgen.Organization(t, db, database.Organization{})
	owner := dbgen.User(t, db, database.User{})
	providerName := "openai"
	_, err := db.InsertChatProvider(ctx, database.InsertChatProviderParams{
		Provider:             providerName,
		DisplayName:          "OpenAI",
		APIKey:               "test-key",
		CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
		Enabled:              true,
		CentralApiKeyEnabled: true,
	})
	require.NoError(t, err)

	model, err := db.InsertChatModelConfig(ctx,
		database.InsertChatModelConfigParams{
			Provider:             providerName,
			Model:                "model-" + uuid.NewString(),
			DisplayName:          "Test Model",
			CreatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
			UpdatedBy:            uuid.NullUUID{UUID: owner.ID, Valid: true},
			Enabled:              true,
			IsDefault:            true,
			ContextLimit:         128000,
			CompressionThreshold: 70,
			Options:              json.RawMessage(`{}`),
		},
	)
	require.NoError(t, err)

	chat := insertChat(ctx, t, db, org.ID, owner.ID, model.ID)
	return org, owner, chat, model
}

func insertChat(
	ctx context.Context,
	t *testing.T,
	db database.Store,
	orgID uuid.UUID,
	ownerID uuid.UUID,
	modelID uuid.UUID,
) database.Chat {
	t.Helper()

	chat, err := db.InsertChat(ctx, database.InsertChatParams{
		OrganizationID:    orgID,
		Status:            database.ChatStatusWaiting,
		ClientType:        database.ChatClientTypeUi,
		OwnerID:           ownerID,
		LastModelConfigID: modelID,
		Title:             "chat-" + uuid.NewString(),
	})
	require.NoError(t, err)
	return chat
}

func insertMessage(
	ctx context.Context,
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

	messages, err := db.InsertChatMessages(ctx, database.InsertChatMessagesParams{
		ChatID:              chatID,
		CreatedBy:           []uuid.UUID{createdBy},
		ModelConfigID:       []uuid.UUID{modelID},
		Role:                []database.ChatMessageRole{role},
		Content:             []string{string(parts.RawMessage)},
		ContentVersion:      []int16{chatprompt.CurrentContentVersion},
		Visibility:          []database.ChatMessageVisibility{database.ChatMessageVisibilityBoth},
		InputTokens:         []int64{0},
		OutputTokens:        []int64{0},
		TotalTokens:         []int64{0},
		ReasoningTokens:     []int64{0},
		CacheCreationTokens: []int64{0},
		CacheReadTokens:     []int64{0},
		ContextLimit:        []int64{0},
		Compressed:          []bool{false},
		TotalCostMicros:     []int64{0},
		RuntimeMs:           []int64{0},
		ProviderResponseID:  []string{""},
	})
	require.NoError(t, err)
	require.Len(t, messages, 1)
	return messages[0]
}

func createRun(t *testing.T, fixture testFixture) database.ChatDebugRun {
	t.Helper()

	run, err := fixture.svc.CreateRun(fixture.ctx, chatdebug.CreateRunParams{
		ChatID:        fixture.chat.ID,
		ModelConfigID: fixture.model.ID,
		Kind:          chatdebug.KindChatTurn,
		Status:        chatdebug.StatusInProgress,
		Provider:      fixture.model.Provider,
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
