package db2sdk

import (
	"time"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
)

// chatAutomationNextRunCount is how many upcoming runs of a schedule the
// response lists.
const chatAutomationNextRunCount = 5

// ChatAutomationRefs holds what a chat automation response adds to the row:
// the owners and the chats that the rows name.
type ChatAutomationRefs struct {
	Owners map[uuid.UUID]database.User
	// Chats holds only the chats that the caller can read.
	Chats map[uuid.UUID]database.Chat
}

// ChatAutomationFromListRow copies the columns of a list row. The row has no
// webhook secret, so the result has none either.
func ChatAutomationFromListRow(r database.GetChatAutomationsByOrganizationIDRow) database.ChatAutomation {
	return database.ChatAutomation{
		ID:                     r.ID,
		OrganizationID:         r.OrganizationID,
		OwnerID:                r.OwnerID,
		Name:                   r.Name,
		CreatedByChatID:        r.CreatedByChatID,
		Kind:                   r.Kind,
		Enabled:                r.Enabled,
		Prompt:                 r.Prompt,
		TargetMode:             r.TargetMode,
		TargetChatID:           r.TargetChatID,
		WhenBusy:               r.WhenBusy,
		NewChatModelConfigID:   r.NewChatModelConfigID,
		ReasoningEffort:        r.ReasoningEffort,
		ProjectID:              r.ProjectID,
		WebhookAuth:            r.WebhookAuth,
		WebhookSecretVersion:   r.WebhookSecretVersion,
		ScheduleCron:           r.ScheduleCron,
		ScheduleTimeZone:       r.ScheduleTimeZone,
		ScheduleNextRunAt:      r.ScheduleNextRunAt,
		ScheduleRevision:       r.ScheduleRevision,
		LastRunAt:              r.LastRunAt,
		LastRunOutcome:         r.LastRunOutcome,
		LastRunReason:          r.LastRunReason,
		LastRunChatID:          r.LastRunChatID,
		LastRunUnacceptedCount: r.LastRunUnacceptedCount,
		CreatedAt:              r.CreatedAt,
		UpdatedAt:              r.UpdatedAt,
	}
}

// ChatAutomation builds the response from a row, field by field. It
// never reads WebhookSecret or WebhookSecretKeyID.
func ChatAutomation(a database.ChatAutomation, refs ChatAutomationRefs, now time.Time) codersdk.ChatAutomation {
	out := codersdk.ChatAutomation{
		ID:                   a.ID,
		OrganizationID:       a.OrganizationID,
		OwnerID:              a.OwnerID,
		Name:                 a.Name,
		Kind:                 codersdk.ChatAutomationKind(a.Kind),
		Enabled:              a.Enabled,
		TargetMode:           codersdk.ChatAutomationTargetMode(a.TargetMode),
		TargetChatID:         nullUUIDPtr(a.TargetChatID),
		TargetChat:           chatAutomationChat(refs, a.TargetChatID),
		CreatedByChatID:      nullUUIDPtr(a.CreatedByChatID),
		CreatedByChat:        chatAutomationChat(refs, a.CreatedByChatID),
		NewChatModelConfigID: nullUUIDPtr(a.NewChatModelConfigID),
		ProjectID:            nullUUIDPtr(a.ProjectID),
		WebhookSecretVersion: a.WebhookSecretVersion,
		Prompt:               a.Prompt,
		ScheduleNextRunAt:    nullTimePtr(a.ScheduleNextRunAt),
		NextRunTimes:         chatAutomationNextRunTimes(a, now),
		// Empty until the run path reports why an automation cannot run.
		PausedReasons: []codersdk.ChatAutomationPausedReason{},
		CreatedAt:     a.CreatedAt,
		UpdatedAt:     a.UpdatedAt,
	}
	if owner, ok := refs.Owners[a.OwnerID]; ok {
		minimal := MinimalUser(owner)
		out.Owner = &minimal
	}
	if a.WhenBusy.Valid {
		out.WhenBusy = ptr.Ref(codersdk.ChatAutomationWhenBusy(a.WhenBusy.ChatAutomationWhenBusy))
	}
	if a.ReasoningEffort.Valid {
		out.ReasoningEffort = ptr.Ref(string(a.ReasoningEffort.ChatReasoningEffort))
	}
	if a.WebhookAuth.Valid {
		out.WebhookAuth = ptr.Ref(codersdk.ChatAutomationWebhookAuth(a.WebhookAuth.ChatAutomationWebhookAuth))
	}
	if a.ScheduleCron.Valid {
		out.ScheduleCron = ptr.Ref(a.ScheduleCron.String)
	}
	if a.ScheduleTimeZone.Valid {
		out.ScheduleTimeZone = ptr.Ref(a.ScheduleTimeZone.String)
	}
	if a.LastRunAt.Valid && a.LastRunOutcome.Valid {
		out.LastRun = &codersdk.ChatAutomationLastRun{
			At:      a.LastRunAt.Time,
			Outcome: codersdk.ChatAutomationRunOutcome(a.LastRunOutcome.ChatAutomationRunOutcome),
			ChatID:  nullUUIDPtr(a.LastRunChatID),
		}
		if a.LastRunReason.Valid {
			out.LastRun.Reason = ptr.Ref(a.LastRunReason.String)
		}
	}
	return out
}

func chatAutomationChat(refs ChatAutomationRefs, id uuid.NullUUID) *codersdk.ChatAutomationChat {
	if !id.Valid {
		return nil
	}
	chat, ok := refs.Chats[id.UUID]
	if !ok {
		return nil
	}
	return &codersdk.ChatAutomationChat{ID: chat.ID, Title: chat.Title, Archived: chat.Archived}
}

// chatAutomationNextRunTimes lists the next runs of an enabled schedule. It
// is empty for anything else, and for a stored spec that this host cannot
// parse.
func chatAutomationNextRunTimes(a database.ChatAutomation, now time.Time) []time.Time {
	times := []time.Time{}
	if a.Kind != database.ChatAutomationKindSchedule || !a.Enabled || !a.ScheduleCron.Valid || !a.ScheduleTimeZone.Valid {
		return times
	}
	sched, err := cron.Standard(a.ScheduleCron.String, a.ScheduleTimeZone.String)
	if err != nil {
		return times
	}
	for next := now; len(times) < chatAutomationNextRunCount; {
		next = sched.Next(next)
		if next.IsZero() {
			break
		}
		times = append(times, next)
	}
	return times
}
