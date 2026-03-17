package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ChatAutomationTriggerType represents how an automation is triggered.
type ChatAutomationTriggerType string

const (
	ChatAutomationTriggerWebhook ChatAutomationTriggerType = "webhook"
	ChatAutomationTriggerCron    ChatAutomationTriggerType = "cron"
)

// ChatAutomation represents a stored automation that creates chats
// when triggered.
type ChatAutomation struct {
	ID                uuid.UUID                 `json:"id" format:"uuid"`
	OwnerID           uuid.UUID                 `json:"owner_id" format:"uuid"`
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	Icon              string                    `json:"icon"`
	TriggerType       ChatAutomationTriggerType `json:"trigger_type"`
	WebhookSecret     *string                   `json:"webhook_secret,omitempty"`
	WebhookURL        string                    `json:"webhook_url,omitempty"`
	CronSchedule      *string                   `json:"cron_schedule,omitempty"`
	ModelConfigID     uuid.UUID                 `json:"model_config_id" format:"uuid"`
	SystemPrompt      string                    `json:"system_prompt"`
	PromptTemplate    string                    `json:"prompt_template"`
	Enabled           bool                      `json:"enabled"`
	MaxConcurrentRuns int32                     `json:"max_concurrent_runs"`
	CreatedAt         time.Time                 `json:"created_at" format:"date-time"`
	UpdatedAt         time.Time                 `json:"updated_at" format:"date-time"`
}

// CreateChatAutomationRequest is the payload for creating a new
// automation.
type CreateChatAutomationRequest struct {
	Name              string                    `json:"name"`
	Description       string                    `json:"description"`
	Icon              string                    `json:"icon"`
	TriggerType       ChatAutomationTriggerType `json:"trigger_type"`
	CronSchedule      *string                   `json:"cron_schedule,omitempty"`
	ModelConfigID     uuid.UUID                 `json:"model_config_id" format:"uuid"`
	SystemPrompt      string                    `json:"system_prompt"`
	PromptTemplate    string                    `json:"prompt_template"`
	MaxConcurrentRuns int32                     `json:"max_concurrent_runs"`
}

// UpdateChatAutomationRequest is the payload for updating an
// automation.
type UpdateChatAutomationRequest struct {
	Name              string    `json:"name"`
	Description       string    `json:"description"`
	Icon              string    `json:"icon"`
	CronSchedule      *string   `json:"cron_schedule,omitempty"`
	ModelConfigID     uuid.UUID `json:"model_config_id" format:"uuid"`
	SystemPrompt      string    `json:"system_prompt"`
	PromptTemplate    string    `json:"prompt_template"`
	Enabled           bool      `json:"enabled"`
	MaxConcurrentRuns int32     `json:"max_concurrent_runs"`
}

// ChatAutomationRun represents a single execution of an automation.
type ChatAutomationRun struct {
	ID             uuid.UUID       `json:"id" format:"uuid"`
	AutomationID   uuid.UUID       `json:"automation_id" format:"uuid"`
	ChatID         *uuid.UUID      `json:"chat_id,omitempty" format:"uuid"`
	TriggerPayload json.RawMessage `json:"trigger_payload"`
	RenderedPrompt string          `json:"rendered_prompt"`
	Status         string          `json:"status"`
	Error          *string         `json:"error,omitempty"`
	StartedAt      *time.Time      `json:"started_at,omitempty" format:"date-time"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty" format:"date-time"`
	CreatedAt      time.Time       `json:"created_at" format:"date-time"`
}

// ListChatAutomations returns all chat automations for the
// authenticated user.
func (c *Client) ListChatAutomations(ctx context.Context) ([]ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/chats/automations", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var automations []ChatAutomation
	return automations, json.NewDecoder(res.Body).Decode(&automations)
}

// CreateChatAutomation creates a new chat automation.
func (c *Client) CreateChatAutomation(ctx context.Context, req CreateChatAutomationRequest) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/chats/automations", req)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return ChatAutomation{}, ReadBodyAsError(res)
	}

	var automation ChatAutomation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

// ChatAutomation returns a single chat automation by ID.
func (c *Client) ChatAutomation(ctx context.Context, id uuid.UUID) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/automations/%s", id), nil)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomation{}, ReadBodyAsError(res)
	}

	var automation ChatAutomation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

// UpdateChatAutomation updates an existing chat automation.
func (c *Client) UpdateChatAutomation(ctx context.Context, id uuid.UUID, req UpdateChatAutomationRequest) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/v2/chats/automations/%s", id), req)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomation{}, ReadBodyAsError(res)
	}

	var automation ChatAutomation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

// DeleteChatAutomation deletes a chat automation.
func (c *Client) DeleteChatAutomation(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/chats/automations/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// TriggerChatAutomation manually triggers an automation run.
func (c *Client) TriggerChatAutomation(ctx context.Context, id uuid.UUID) (ChatAutomationRun, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/automations/%s/trigger", id), nil)
	if err != nil {
		return ChatAutomationRun{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return ChatAutomationRun{}, ReadBodyAsError(res)
	}

	var run ChatAutomationRun
	return run, json.NewDecoder(res.Body).Decode(&run)
}

// RotateChatAutomationSecret rotates the webhook secret for an
// automation.
func (c *Client) RotateChatAutomationSecret(ctx context.Context, id uuid.UUID) (ChatAutomation, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/chats/automations/%s/rotate-secret", id), nil)
	if err != nil {
		return ChatAutomation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ChatAutomation{}, ReadBodyAsError(res)
	}

	var automation ChatAutomation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

// ListChatAutomationRuns returns the runs for a chat automation.
func (c *Client) ListChatAutomationRuns(ctx context.Context, id uuid.UUID) ([]ChatAutomationRun, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/chats/automations/%s/runs", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var runs []ChatAutomationRun
	return runs, json.NewDecoder(res.Body).Decode(&runs)
}
