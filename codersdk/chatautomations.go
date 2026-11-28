package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ChatAutomationKind is what starts a chat automation.
type ChatAutomationKind string

const ChatAutomationKindSchedule ChatAutomationKind = "schedule"

// ChatAutomationTargetMode is where a chat automation sends its prompt.
type ChatAutomationTargetMode string

const (
	// ChatAutomationTargetModeExistingChat sends every message to one chat.
	ChatAutomationTargetModeExistingChat ChatAutomationTargetMode = "existing_chat"
	// ChatAutomationTargetModeNewChat starts a new chat for each accepted run.
	ChatAutomationTargetModeNewChat ChatAutomationTargetMode = "new_chat"
)

// ChatAutomationRunOutcome is how the latest run of an automation ended.
type ChatAutomationRunOutcome string

const (
	ChatAutomationRunOutcomeAccepted ChatAutomationRunOutcome = "accepted"
	ChatAutomationRunOutcomeSkipped  ChatAutomationRunOutcome = "skipped"
	ChatAutomationRunOutcomeRefused  ChatAutomationRunOutcome = "refused"
)

// ChatAutomationLastRun is the latest run of an automation. It is one
// record, not a history.
type ChatAutomationLastRun struct {
	At      time.Time                `json:"at" format:"date-time"`
	Outcome ChatAutomationRunOutcome `json:"outcome" enums:"accepted,skipped,refused"`
	// Reason says why a run was skipped or refused. It is empty otherwise.
	Reason string     `json:"reason"`
	ChatID *uuid.UUID `json:"chat_id" format:"uuid"`
}

// ChatAutomationChat identifies a chat that an automation refers to. The API
// sets it only when the caller can read the chat.
type ChatAutomationChat struct {
	ID       uuid.UUID `json:"id" format:"uuid"`
	Title    string    `json:"title"`
	Archived bool      `json:"archived"`
}

// ChatAutomation is a schedule that sends a saved prompt to a chat as its
// owner.
type ChatAutomation struct {
	ID              uuid.UUID                `json:"id" format:"uuid"`
	OrganizationID  uuid.UUID                `json:"organization_id" format:"uuid"`
	OwnerID         uuid.UUID                `json:"owner_id" format:"uuid"`
	Owner           MinimalUser              `json:"owner"`
	Name            string                   `json:"name"`
	Kind            ChatAutomationKind       `json:"kind" enums:"schedule"`
	Enabled         bool                     `json:"enabled"`
	TargetMode      ChatAutomationTargetMode `json:"target_mode" enums:"existing_chat,new_chat"`
	TargetChatID    *uuid.UUID               `json:"target_chat_id" format:"uuid"`
	TargetChat      *ChatAutomationChat      `json:"target_chat"`
	CreatedByChatID *uuid.UUID               `json:"created_by_chat_id" format:"uuid"`
	CreatedByChat   *ChatAutomationChat      `json:"created_by_chat"`
	// NewChatModelConfigID and ProjectID apply to new_chat automations.
	NewChatModelConfigID *uuid.UUID `json:"new_chat_model_config_id" format:"uuid"`
	ProjectID            *uuid.UUID `json:"project_id" format:"uuid"`
	Prompt               string     `json:"prompt"`
	ScheduleCron         *string    `json:"schedule_cron"`
	ScheduleTimeZone     *string    `json:"schedule_time_zone"`
	// ScheduleNextRunAt is the occurrence that the scheduler runs next.
	ScheduleNextRunAt *time.Time `json:"schedule_next_run_at" format:"date-time"`
	// NextRunTimes lists up to five upcoming runs of an enabled schedule.
	NextRunTimes []time.Time `json:"next_run_times" format:"date-time" typescript:",notnull"`
	// LastRun is null until the first run.
	LastRun   *ChatAutomationLastRun `json:"last_run"`
	CreatedAt time.Time              `json:"created_at" format:"date-time"`
	UpdatedAt time.Time              `json:"updated_at" format:"date-time"`
}

func chatAutomationsPath(organizationID uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/chats/automations", organizationID)
}

// ChatAutomations lists the chat automations that the caller can read in an
// organization. owner is "me", a user ID or a username, or empty for every
// owner.
func (c *ExperimentalClient) ChatAutomations(ctx context.Context, organizationID uuid.UUID, owner string) ([]ChatAutomation, error) {
	return requestJSON[[]ChatAutomation](ctx, c.Client, http.MethodGet, chatAutomationsPath(organizationID), http.StatusOK, nil, WithQueryParam("owner", owner))
}

// ChatAutomation gets a chat automation.
func (c *ExperimentalClient) ChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID) (ChatAutomation, error) {
	return requestJSON[ChatAutomation](ctx, c.Client, http.MethodGet, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), http.StatusOK, nil)
}

// DeleteChatAutomation deletes a chat automation.
func (c *ExperimentalClient) DeleteChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID) error {
	return requestNoContent(ctx, c.Client, http.MethodDelete, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), nil)
}
