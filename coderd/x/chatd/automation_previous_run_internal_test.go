package chatd

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// setChatStatus sets the status of a chat, as a worker would.
func (f *scheduleFixture) setChatStatus(ctx context.Context, t *testing.T, chatID uuid.UUID, status database.ChatStatus) {
	t.Helper()
	_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET status = $1 WHERE id = $2", status, chatID)
	require.NoError(t, err)
}

// finishCreatedChats ends the turns of the chats automations created, as a
// worker would, so the next new_chat occurrence is not skipped for an
// active earlier run. No worker runs in these tests.
func (f *scheduleFixture) finishCreatedChats(ctx context.Context, t *testing.T) {
	t.Helper()
	_, err := f.sqlDB.ExecContext(ctx, "UPDATE chats SET status = 'waiting' WHERE owner_id = $1 AND automation_id IS NOT NULL", f.owner.ID)
	require.NoError(t, err)
}

// TestAutomationPreviousRunActive checks that a scheduled occurrence of a
// new_chat automation is skipped while a chat of an earlier run is still
// running, and that webhook deliveries and Run now are not.
func TestAutomationPreviousRunActive(t *testing.T) {
	t.Parallel()

	t.Run("ScheduleSkipsUntilPreviousRunEnds", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "* * * * *", "UTC")

		// The first occurrence creates a chat. No worker runs in this
		// test, so the chat stays running, as with a stalled model.
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		chats := f.createdChats(ctx, t)
		require.Len(t, chats, 1)
		first := chats[0]
		f.setChatStatus(ctx, t, first.ID, database.ChatStatusRunning)

		// The next occurrence is skipped and names the active chat. The
		// cursor moves on, so the skip does not retry every scan.
		next := f.cursor(ctx, t, automation.ID)
		f.advanceTo(ctx, t, next)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Len(t, f.createdChats(ctx, t), 1)
		requireLastRun(t, f.lastRun(ctx, t, automation.ID), database.ChatAutomationRunOutcomeSkipped,
			codersdk.ChatAutomationRunReasonPreviousRunActive, uuid.NullUUID{UUID: first.ID, Valid: true})
		require.True(t, f.cursor(ctx, t, automation.ID).After(next))

		// Retries count as active: the chat is still running while it
		// waits to retry. An interrupt that has not finished does too.
		f.setChatStatus(ctx, t, first.ID, database.ChatStatusInterrupting)
		f.advanceTo(ctx, t, f.cursor(ctx, t, automation.ID))
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		require.Len(t, f.createdChats(ctx, t), 1)

		// Once the earlier chat ends, in error or waiting for the next
		// message, the next occurrence runs.
		for i, status := range []database.ChatStatus{database.ChatStatusError, database.ChatStatusWaiting} {
			for _, chat := range f.createdChats(ctx, t) {
				f.setChatStatus(ctx, t, chat.ID, status)
			}
			f.advanceTo(ctx, t, f.cursor(ctx, t, automation.ID))
			server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
			require.Len(t, f.createdChats(ctx, t), 2+i, "after the earlier chat became %s", status)
			require.Equal(t, database.ChatAutomationRunOutcomeAccepted, f.lastRun(ctx, t, automation.ID).LastRunOutcome.ChatAutomationRunOutcome)
		}
	})

	t.Run("RunNowNotLimited", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "* * * * *", "UTC")
		ownerCtx := f.asOwner(ctx, t)

		first, err := server.RunAutomation(ownerCtx, f.owner.ID, automation.ID, nil)
		require.NoError(t, err)
		f.setChatStatus(ctx, t, first.ChatID, database.ChatStatusRunning)
		// A person asked for this run, so it starts although the first
		// one is still running.
		second, err := server.RunAutomation(ownerCtx, f.owner.ID, automation.ID, nil)
		require.NoError(t, err)
		require.NotEqual(t, first.ChatID, second.ChatID)
	})

	t.Run("WebhookNotLimited", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation, _, err := server.CreateAutomation(f.asOwner(ctx, t), CreateAutomationParams{
			OrganizationID: f.org.ID,
			OwnerID:        f.owner.ID,
			Request: codersdk.CreateChatAutomationRequest{
				Name:                 "Per event",
				Kind:                 codersdk.ChatAutomationKindWebhook,
				TargetMode:           codersdk.ChatAutomationTargetModeNewChat,
				NewChatModelConfigID: &f.model.ID,
				Prompt:               "Review the event.",
			},
		})
		require.NoError(t, err)

		// Concurrent webhook runs, such as one chat per event, are
		// intended, so a running earlier chat does not hold back the next.
		var chatIDs []uuid.UUID
		for range 2 {
			result, err := server.PublishAutomationWebhook(ctx, PublishAutomationWebhookParams{
				AutomationID:  automation.ID,
				SecretVersion: automation.WebhookSecretVersion,
				Body:          []byte(`{"event":"opened"}`),
			})
			require.NoError(t, err)
			f.setChatStatus(ctx, t, result.ChatID, database.ChatStatusRunning)
			chatIDs = append(chatIDs, result.ChatID)
		}
		require.NotEqual(t, chatIDs[0], chatIDs[1])
	})
}
