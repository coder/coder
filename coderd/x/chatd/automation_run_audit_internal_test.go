package chatd

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// runAudits returns the start entries of automationID, oldest first, with
// their additional fields decoded.
func (f *scheduleFixture) runAudits(t *testing.T, automationID uuid.UUID) ([]database.AuditLog, []map[string]string) {
	t.Helper()
	var (
		logs   []database.AuditLog
		fields []map[string]string
	)
	for _, entry := range f.auditor.AuditLogs() {
		if entry.ResourceType != database.ResourceTypeChatAutomation || entry.ResourceID != automationID || entry.Action != database.AuditActionStart {
			continue
		}
		var additional map[string]string
		require.NoError(t, json.Unmarshal(entry.AdditionalFields, &additional))
		logs = append(logs, entry)
		fields = append(fields, additional)
	}
	return logs, fields
}

// TestAutomationRunAudit checks that every run of an automation, accepted,
// skipped, missed, or refused, leaves one start entry for the automation,
// attributed to its owner.
func TestAutomationRunAudit(t *testing.T) {
	t.Parallel()

	t.Run("ScheduleAccepted", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)

		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		logs, fields := f.runAudits(t, automation.ID)
		require.Len(t, logs, 1)
		require.Equal(t, f.owner.ID, logs[0].UserID)
		require.Equal(t, f.org.ID, logs[0].OrganizationID)
		require.EqualValues(t, http.StatusAccepted, logs[0].StatusCode)
		require.Equal(t, "schedule", fields[0]["trigger"])
		require.Equal(t, "accepted", fields[0]["outcome"])
		require.Empty(t, fields[0]["reason"])
		require.Equal(t, f.chat.ID.String(), fields[0]["chat_id"])
		require.NotEmpty(t, fields[0]["input_id"])
	})

	t.Run("ScheduleSkippedBusy", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusRunning, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "* * * * *", "UTC", codersdk.ChatAutomationWhenBusySkip)

		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time)
		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		logs, fields := f.runAudits(t, automation.ID)
		require.Len(t, logs, 1)
		require.EqualValues(t, http.StatusConflict, logs[0].StatusCode)
		require.Equal(t, "skipped", fields[0]["outcome"])
		require.Equal(t, "chat_busy", fields[0]["reason"])
	})

	t.Run("ScheduleMissed", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.existingChat(ctx, t, server, "0 * * * *", "UTC", codersdk.ChatAutomationWhenBusyQueue)
		f.advanceTo(ctx, t, automation.ScheduleNextRunAt.Time.Add(2*automationScheduleGrace))

		server.scanAutomationSchedules(ctx, defaultAutomationScheduleBatchSize)
		logs, fields := f.runAudits(t, automation.ID)
		require.Len(t, logs, 1)
		require.Equal(t, "schedule", fields[0]["trigger"])
		require.Equal(t, "skipped", fields[0]["outcome"])
		require.Equal(t, "missed", fields[0]["reason"])
	})

	t.Run("RunNowRefused", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation := f.newChat(ctx, t, server, "* * * * *", "UTC")
		_, err := f.sqlDB.ExecContext(ctx, "UPDATE chat_model_configs SET enabled = false WHERE id = $1", f.model.ID)
		require.NoError(t, err)

		_, err = server.RunAutomation(f.asOwner(ctx, t), f.owner.ID, automation.ID, nil)
		require.ErrorIs(t, err, ErrAutomationModelUnavailable)
		logs, fields := f.runAudits(t, automation.ID)
		require.Len(t, logs, 1)
		require.EqualValues(t, http.StatusConflict, logs[0].StatusCode)
		require.Equal(t, "run_now", fields[0]["trigger"])
		require.Equal(t, "refused", fields[0]["outcome"])
		require.Equal(t, "model_unavailable", fields[0]["reason"])
	})

	t.Run("WebhookCarriesRequest", func(t *testing.T) {
		t.Parallel()
		f := newScheduleFixture(t, database.ChatStatusWaiting, scheduleStart)
		ctx := testutil.Context(t, testutil.WaitLong)
		server := f.newServer(t, Limits{})
		automation, _, err := server.CreateAutomation(f.asOwner(ctx, t), CreateAutomationParams{
			OrganizationID: f.org.ID,
			OwnerID:        f.owner.ID,
			Request: codersdk.CreateChatAutomationRequest{
				Name:         "Hook",
				Kind:         codersdk.ChatAutomationKindWebhook,
				TargetMode:   codersdk.ChatAutomationTargetModeExistingChat,
				TargetChatID: &f.chat.ID,
				Prompt:       "Look at the event.",
			},
		})
		require.NoError(t, err)

		request := AutomationRequest{IP: "203.0.113.7", UserAgent: "ci-sender/1.0", RequestID: uuid.New()}
		_, err = server.PublishAutomationWebhook(ctx, PublishAutomationWebhookParams{
			AutomationID:  automation.ID,
			SecretVersion: automation.WebhookSecretVersion,
			Body:          []byte(`{}`),
			Request:       request,
		})
		require.NoError(t, err)
		logs, fields := f.runAudits(t, automation.ID)
		require.Len(t, logs, 1)
		require.Equal(t, "webhook", fields[0]["trigger"])
		require.Equal(t, "accepted", fields[0]["outcome"])
		require.Equal(t, "203.0.113.7", logs[0].Ip.IPNet.IP.String())
		require.Equal(t, "ci-sender/1.0", logs[0].UserAgent.String)
		require.Equal(t, request.RequestID, logs[0].RequestID)
	})
}
