package chatd

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// lastRun reads the automation's stored last run.
func (f *scheduleFixture) lastRun(ctx context.Context, t *testing.T, automationID uuid.UUID) database.ChatAutomation {
	t.Helper()
	row, err := f.db.GetChatAutomationByID(ctx, automationID)
	require.NoError(t, err)
	return row
}

func requireLastRun(t *testing.T, row database.ChatAutomation, outcome database.ChatAutomationRunOutcome, reason codersdk.ChatAutomationRunReason, chatID uuid.NullUUID) {
	t.Helper()
	require.True(t, row.LastRunAt.Valid, "last run recorded")
	require.Equal(t, database.NullChatAutomationRunOutcome{ChatAutomationRunOutcome: outcome, Valid: true}, row.LastRunOutcome)
	if reason == "" {
		require.False(t, row.LastRunReason.Valid)
	} else {
		require.Equal(t, string(reason), row.LastRunReason.String)
	}
	require.Equal(t, chatID, row.LastRunChatID)
}

func TestAutomationLastRun(t *testing.T) {
	t.Parallel()

	t.Run("Accepted", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		require.False(t, automation.LastRunAt.Valid, "no run before the first occurrence")

		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Equal(t, 1, f.inputs(ctx, t))
		row := f.lastRun(ctx, t, automation.ID)
		requireLastRun(t, row, database.ChatAutomationRunOutcomeAccepted, "", uuid.NullUUID{UUID: f.chat.ID, Valid: true})
		require.True(t, row.LastRunAt.Time.Equal(f.clock.Now()))
	})

	t.Run("AcceptedNewChat", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "* * * * *", "UTC")

		result, err := server.RunAutomation(f.asOwner(ctx, t), f.owner.ID, automation.ID, nil)
		require.NoError(t, err)
		row := f.lastRun(ctx, t, automation.ID)
		requireLastRun(t, row, database.ChatAutomationRunOutcomeAccepted, "", uuid.NullUUID{UUID: result.ChatID, Valid: true})
		// Run now leaves the schedule alone, and recording it is not a
		// change to the automation.
		require.True(t, row.UpdatedAt.Equal(automation.UpdatedAt))
	})

	t.Run("SkippedBusy", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusRunning, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusySkip)

		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Zero(t, f.inputs(ctx, t))
		// The skipped run names the chat it was meant for.
		requireLastRun(t, f.lastRun(ctx, t, automation.ID), database.ChatAutomationRunOutcomeSkipped, codersdk.ChatAutomationRunReasonChatBusy, uuid.NullUUID{UUID: f.chat.ID, Valid: true})
	})

	t.Run("RefusedModelUnavailable", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "* * * * *", "UTC")
		_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_model_configs SET enabled = false WHERE id = $1", f.model.ID)
		require.NoError(t, err)

		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Empty(t, f.createdChats(ctx, t))
		requireLastRun(t, f.lastRun(ctx, t, automation.ID), database.ChatAutomationRunOutcomeRefused, codersdk.ChatAutomationRunReasonModelUnavailable, uuid.NullUUID{})
	})

	t.Run("RefusedOwnerInactive", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		ownerCtx := f.asOwner(ctx, t)
		_, err := f.sqlDB.ExecContext(ctx, "UPDATE users SET status = 'suspended' WHERE id = $1", f.owner.ID)
		require.NoError(t, err)

		_, err = server.RunAutomation(ownerCtx, f.owner.ID, automation.ID, nil)
		require.ErrorIs(t, err, ErrAutomationOwnerInactive)
		requireLastRun(t, f.lastRun(ctx, t, automation.ID), database.ChatAutomationRunOutcomeRefused, codersdk.ChatAutomationRunReasonOwnerInactive, uuid.NullUUID{UUID: f.chat.ID, Valid: true})
	})

	t.Run("MissedOccurrence", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		// An hourly schedule has no run within the grace window after the
		// missed one.
		automation := f.existingChat(ctx, t, server, "0 * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time.Add(2*automationScheduleGrace))

		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Zero(t, f.inputs(ctx, t))
		requireLastRun(t, f.lastRun(ctx, t, automation.ID), database.ChatAutomationRunOutcomeSkipped, codersdk.ChatAutomationRunReasonMissed, uuid.NullUUID{UUID: f.chat.ID, Valid: true})
	})

	t.Run("LaterRunWins", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)

		server.recordAutomationRun(ctx, automationRun{
			automationID: automation.ID,
			trigger:      automationRunTriggerSchedule,
			outcome:      database.ChatAutomationRunOutcomeAccepted,
			chatID:       f.chat.ID,
		})
		recorded := f.lastRun(ctx, t, automation.ID).LastRunAt.Time
		// A write that another instance decided earlier does not replace
		// the later one.
		count, err := f.db.UpdateChatAutomationLastRunByID(ctx, database.UpdateChatAutomationLastRunByIDParams{
			ID:             automation.ID,
			LastRunAt:      recorded.Add(-time.Second),
			LastRunOutcome: database.ChatAutomationRunOutcomeRefused,
		})
		require.NoError(t, err)
		require.Zero(t, count)
		row := f.lastRun(ctx, t, automation.ID)
		require.True(t, row.LastRunAt.Time.Equal(recorded))
		require.Equal(t, database.ChatAutomationRunOutcomeAccepted, row.LastRunOutcome.ChatAutomationRunOutcome)
	})
}

func TestClassifyAutomationRun(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		err     error
		outcome database.ChatAutomationRunOutcome
		reason  codersdk.ChatAutomationRunReason
		record  bool
	}{
		{nil, database.ChatAutomationRunOutcomeAccepted, "", true},
		{ErrAutomationChatBusy, database.ChatAutomationRunOutcomeSkipped, codersdk.ChatAutomationRunReasonChatBusy, true},
		{&AutomationQueueShareFullError{Max: 1}, database.ChatAutomationRunOutcomeSkipped, codersdk.ChatAutomationRunReasonQueueFull, true},
		{chatstate.ErrMessageQueueFull, database.ChatAutomationRunOutcomeSkipped, codersdk.ChatAutomationRunReasonQueueFull, true},
		{ErrAutomationOccurrenceExpired, database.ChatAutomationRunOutcomeSkipped, codersdk.ChatAutomationRunReasonMissed, true},
		{&AutomationPreviousRunActiveError{ChatID: uuid.New()}, database.ChatAutomationRunOutcomeSkipped, codersdk.ChatAutomationRunReasonPreviousRunActive, true},
		{&chathooks.UserPromptDeniedError{UserMessage: "no"}, database.ChatAutomationRunOutcomeRefused, codersdk.ChatAutomationRunReasonHookDenied, true},
		{ErrAutomationWebhookConsumed, database.ChatAutomationRunOutcomeRefused, codersdk.ChatAutomationRunReasonSingleUseConsumed, true},
		{&AutomationEventTooLargeError{Size: 2, Max: 1}, database.ChatAutomationRunOutcomeRefused, codersdk.ChatAutomationRunReasonBodyTooLarge, true},
		{xerrors.New("boom"), database.ChatAutomationRunOutcomeRefused, codersdk.ChatAutomationRunReasonError, true},
		{ErrAutomationScheduleStale, "", "", false},
		{ErrAutomationNotFound, "", "", false},
		{ErrAutomationSecretChanged, "", "", false},
		{context.Canceled, "", "", false},
	} {
		outcome, reason, record := classifyAutomationRun(tc.err)
		require.Equal(t, tc.record, record, "%v", tc.err)
		require.Equal(t, tc.outcome, outcome, "%v", tc.err)
		require.Equal(t, tc.reason, reason, "%v", tc.err)
	}
}
