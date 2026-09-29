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

const (
	ChatAutomationKindWebhook  ChatAutomationKind = "webhook"
	ChatAutomationKindSchedule ChatAutomationKind = "schedule"
)

// ChatAutomationTargetMode is where a chat automation delivers its prompt.
type ChatAutomationTargetMode string

const (
	ChatAutomationTargetModeExistingChat ChatAutomationTargetMode = "existing_chat"
	ChatAutomationTargetModeNewChat      ChatAutomationTargetMode = "new_chat"
)

// ChatAutomationWebhookUse is how many times a webhook automation can be
// triggered.
type ChatAutomationWebhookUse string

const (
	ChatAutomationWebhookUseSingle ChatAutomationWebhookUse = "single"
	ChatAutomationWebhookUseMulti  ChatAutomationWebhookUse = "multi"
)

// ChatAutomationWhenBusy is what an existing_chat automation does when the
// target chat is busy.
type ChatAutomationWhenBusy string

const (
	ChatAutomationWhenBusyQueue ChatAutomationWhenBusy = "queue"
	ChatAutomationWhenBusySkip  ChatAutomationWhenBusy = "skip"
)

// ChatAutomation is a webhook or scheduled automation that delivers a
// prompt to an agent chat. It never carries the webhook secret or its
// hash.
type ChatAutomation struct {
	ID                   uuid.UUID                 `json:"id" format:"uuid"`
	OrganizationID       uuid.UUID                 `json:"organization_id" format:"uuid"`
	OwnerID              uuid.UUID                 `json:"owner_id" format:"uuid"`
	Name                 string                    `json:"name"`
	CreatedByChatID      *uuid.UUID                `json:"created_by_chat_id,omitempty" format:"uuid"`
	Kind                 ChatAutomationKind        `json:"kind" enums:"webhook,schedule"`
	Enabled              bool                      `json:"enabled"`
	TargetMode           ChatAutomationTargetMode  `json:"target_mode" enums:"existing_chat,new_chat"`
	TargetChatID         *uuid.UUID                `json:"target_chat_id,omitempty" format:"uuid"`
	NewChatModelConfigID *uuid.UUID                `json:"new_chat_model_config_id,omitempty" format:"uuid"`
	ReasoningEffort      *string                   `json:"reasoning_effort,omitempty"`
	WhenBusy             *ChatAutomationWhenBusy   `json:"when_busy,omitempty" enums:"queue,skip"`
	WebhookUse           *ChatAutomationWebhookUse `json:"webhook_use,omitempty" enums:"single,multi"`
	WebhookSecretVersion int64                     `json:"webhook_secret_version"`
	WebhookConsumedAt    *time.Time                `json:"webhook_consumed_at,omitempty" format:"date-time"`
	Prompt               string                    `json:"prompt"`
	ScheduleCron         *string                   `json:"schedule_cron,omitempty"`
	ScheduleTimeZone     *string                   `json:"schedule_time_zone,omitempty"`
	ScheduleNextRunAt    *time.Time                `json:"schedule_next_run_at,omitempty" format:"date-time"`
	// NextRunTimes lists up to five upcoming runs of an enabled schedule.
	// It is empty for webhooks and disabled schedules.
	NextRunTimes []time.Time `json:"next_run_times" format:"date-time"`
	CreatedAt    time.Time   `json:"created_at" format:"date-time"`
	UpdatedAt    time.Time   `json:"updated_at" format:"date-time"`
}

// CreateChatAutomationRequest creates a chat automation owned by the
// caller.
type CreateChatAutomationRequest struct {
	Name                 string                    `json:"name"`
	Kind                 ChatAutomationKind        `json:"kind" enums:"webhook,schedule"`
	TargetMode           ChatAutomationTargetMode  `json:"target_mode" enums:"existing_chat,new_chat"`
	TargetChatID         *uuid.UUID                `json:"target_chat_id,omitempty" format:"uuid"`
	NewChatModelConfigID *uuid.UUID                `json:"new_chat_model_config_id,omitempty" format:"uuid"`
	ReasoningEffort      *string                   `json:"reasoning_effort,omitempty"`
	WhenBusy             *ChatAutomationWhenBusy   `json:"when_busy,omitempty" enums:"queue,skip"`
	WebhookUse           *ChatAutomationWebhookUse `json:"webhook_use,omitempty" enums:"single,multi"`
	Prompt               string                    `json:"prompt"`
	// ScheduleCron is a standard five-field cron expression. As in standard
	// cron, when both day of month and day of week are restricted, a time
	// matches if either field matches.
	ScheduleCron     *string `json:"schedule_cron,omitempty"`
	ScheduleTimeZone *string `json:"schedule_time_zone,omitempty"`
}

// CreateChatAutomationResponse is returned when a chat automation is
// created. WebhookSecret is set only for webhook automations, and this is
// the only response that ever contains it.
type CreateChatAutomationResponse struct {
	Automation    ChatAutomation `json:"automation"`
	WebhookSecret string         `json:"webhook_secret,omitempty"`
}

// UpdateChatAutomationRequest changes the set fields of a chat automation.
// The kind and target mode of an automation cannot change.
type UpdateChatAutomationRequest struct {
	Name             *string `json:"name,omitempty"`
	Prompt           *string `json:"prompt,omitempty"`
	ScheduleCron     *string `json:"schedule_cron,omitempty"`
	ScheduleTimeZone *string `json:"schedule_time_zone,omitempty"`
	// ReasoningEffort sets the effort of new chats. An empty string clears
	// the override, so new chats use the model's default.
	ReasoningEffort      *string                 `json:"reasoning_effort,omitempty"`
	WhenBusy             *ChatAutomationWhenBusy `json:"when_busy,omitempty" enums:"queue,skip"`
	TargetChatID         *uuid.UUID              `json:"target_chat_id,omitempty" format:"uuid"`
	NewChatModelConfigID *uuid.UUID              `json:"new_chat_model_config_id,omitempty" format:"uuid"`
}

func chatAutomationsPath(organizationID uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/chat-automations", organizationID)
}

// ChatAutomations lists the chat automations in an organization that the
// caller can read.
func (c *ExperimentalClient) ChatAutomations(ctx context.Context, organizationID uuid.UUID) ([]ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodGet, chatAutomationsPath(organizationID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var automations []ChatAutomation
	return automations, ReadBodyAsJSON(res, &automations)
}

// CreateChatAutomation creates a chat automation owned by the caller.
func (c *ExperimentalClient) CreateChatAutomation(ctx context.Context, organizationID uuid.UUID, req CreateChatAutomationRequest) (CreateChatAutomationResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, chatAutomationsPath(organizationID), req)
	if err != nil {
		return CreateChatAutomationResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return CreateChatAutomationResponse{}, ReadBodyAsError(res)
	}
	var resp CreateChatAutomationResponse
	return resp, ReadBodyAsJSON(res, &resp)
}

// ChatAutomation returns a chat automation.
func (c *ExperimentalClient) ChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), nil)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomation{}, ReadBodyAsError(res)
	}
	var automation ChatAutomation
	return automation, ReadBodyAsJSON(res, &automation)
}

// UpdateChatAutomation changes a chat automation. Only its owner can
// update it.
func (c *ExperimentalClient) UpdateChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID, req UpdateChatAutomationRequest) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), req)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomation{}, ReadBodyAsError(res)
	}
	var automation ChatAutomation
	return automation, ReadBodyAsJSON(res, &automation)
}

// DeleteChatAutomation deletes a chat automation.
func (c *ExperimentalClient) DeleteChatAutomation(ctx context.Context, organizationID, automationID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("%s/%s", chatAutomationsPath(organizationID), automationID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
