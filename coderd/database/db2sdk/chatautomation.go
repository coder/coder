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

// ChatAutomationRefs holds the owners and chats that automation rows name.
type ChatAutomationRefs struct {
	Owners map[uuid.UUID]database.User
	// Chats holds only the chats that the caller can read.
	Chats map[uuid.UUID]database.Chat
}

// ChatAutomation converts a row to its API shape. now anchors NextRunTimes.
func ChatAutomation(a database.ChatAutomation, refs ChatAutomationRefs, now time.Time) codersdk.ChatAutomation {
	out := codersdk.ChatAutomation{
		ID:                   a.ID,
		OrganizationID:       a.OrganizationID,
		OwnerID:              a.OwnerID,
		Owner:                MinimalUser(refs.Owners[a.OwnerID]),
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
		Prompt:               a.Prompt,
		ScheduleNextRunAt:    nullTimePtr(a.ScheduleNextRunAt),
		NextRunTimes:         chatAutomationNextRunTimes(a, now),
		CreatedAt:            a.CreatedAt,
		UpdatedAt:            a.UpdatedAt,
	}
	if a.ScheduleCron.Valid {
		out.ScheduleCron = ptr.Ref(a.ScheduleCron.String)
	}
	if a.ScheduleTimeZone.Valid {
		out.ScheduleTimeZone = ptr.Ref(a.ScheduleTimeZone.String)
	}
	if a.LastRunAt.Valid && a.LastRunOutcome.Valid {
		out.LastRun = &codersdk.ChatAutomationLastRun{
			At:              a.LastRunAt.Time,
			Outcome:         codersdk.ChatAutomationRunOutcome(a.LastRunOutcome.ChatAutomationRunOutcome),
			Reason:          a.LastRunReason.String,
			ChatID:          nullUUIDPtr(a.LastRunChatID),
			UnacceptedCount: a.LastRunUnacceptedCount,
		}
	}
	return out
}

func chatAutomationChat(refs ChatAutomationRefs, id uuid.NullUUID) *codersdk.ChatAutomationChat {
	chat, ok := refs.Chats[id.UUID]
	if !id.Valid || !ok {
		return nil
	}
	return &codersdk.ChatAutomationChat{ID: chat.ID, Title: chat.Title, Archived: chat.Archived}
}

// chatAutomationNextRunTimes lists the next runs of an enabled schedule. It
// is empty for anything else, and for a stored spec that this host cannot
// parse.
func chatAutomationNextRunTimes(a database.ChatAutomation, now time.Time) []time.Time {
	times := []time.Time{}
	if !a.Enabled || !a.ScheduleCron.Valid || !a.ScheduleTimeZone.Valid {
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
