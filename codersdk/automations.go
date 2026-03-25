package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// AutomationStatus represents the state of an automation.
type AutomationStatus string

const (
	AutomationStatusDisabled AutomationStatus = "disabled"
	AutomationStatusPreview  AutomationStatus = "preview"
	AutomationStatusActive   AutomationStatus = "active"
)

// Automation represents an automation that bridges external webhooks to
// Coder chats.
type Automation struct {
	ID                    uuid.UUID         `json:"id" format:"uuid"`
	OwnerID               uuid.UUID         `json:"owner_id" format:"uuid"`
	OrganizationID        uuid.UUID         `json:"organization_id" format:"uuid"`
	Name                  string            `json:"name"`
	Description           string            `json:"description"`
	WebhookURL            string            `json:"webhook_url"`
	Filter                json.RawMessage   `json:"filter"`
	SessionLabels         json.RawMessage   `json:"session_labels"`
	SystemPrompt          string            `json:"system_prompt"`
	ModelConfigID         *uuid.UUID        `json:"model_config_id,omitempty" format:"uuid"`
	WorkspaceID           *uuid.UUID        `json:"workspace_id,omitempty" format:"uuid"`
	MCPServerIDs          []uuid.UUID       `json:"mcp_server_ids"`
	AllowedTools          []string          `json:"allowed_tools"`
	Status                AutomationStatus  `json:"status"`
	MaxChatCreatesPerHour int32             `json:"max_chat_creates_per_hour"`
	MaxMessagesPerHour    int32             `json:"max_messages_per_hour"`
	CreatedAt             time.Time         `json:"created_at" format:"date-time"`
	UpdatedAt             time.Time         `json:"updated_at" format:"date-time"`
}

// CreateAutomationRequest is the request body for creating an automation.
type CreateAutomationRequest struct {
	Name                  string           `json:"name"`
	Description           string           `json:"description,omitempty"`
	Filter                json.RawMessage  `json:"filter,omitempty"`
	SessionLabels         json.RawMessage  `json:"session_labels,omitempty"`
	SystemPrompt          string           `json:"system_prompt,omitempty"`
	ModelConfigID         *uuid.UUID       `json:"model_config_id,omitempty" format:"uuid"`
	WorkspaceID           *uuid.UUID       `json:"workspace_id,omitempty" format:"uuid"`
	MCPServerIDs          []uuid.UUID      `json:"mcp_server_ids,omitempty"`
	AllowedTools          []string         `json:"allowed_tools,omitempty"`
	MaxChatCreatesPerHour *int32           `json:"max_chat_creates_per_hour,omitempty"`
	MaxMessagesPerHour    *int32           `json:"max_messages_per_hour,omitempty"`
}

// UpdateAutomationRequest is the request body for updating an automation.
type UpdateAutomationRequest struct {
	Name                  *string          `json:"name,omitempty"`
	Description           *string          `json:"description,omitempty"`
	Filter                json.RawMessage  `json:"filter,omitempty"`
	SessionLabels         json.RawMessage  `json:"session_labels,omitempty"`
	SystemPrompt          *string          `json:"system_prompt,omitempty"`
	ModelConfigID         *uuid.UUID       `json:"model_config_id,omitempty" format:"uuid"`
	WorkspaceID           *uuid.UUID       `json:"workspace_id,omitempty" format:"uuid"`
	MCPServerIDs          *[]uuid.UUID     `json:"mcp_server_ids,omitempty"`
	AllowedTools          *[]string        `json:"allowed_tools,omitempty"`
	Status                *AutomationStatus `json:"status,omitempty"`
	MaxChatCreatesPerHour *int32           `json:"max_chat_creates_per_hour,omitempty"`
	MaxMessagesPerHour    *int32           `json:"max_messages_per_hour,omitempty"`
}

// AutomationWebhookEventStatus represents the outcome of a webhook event.
type AutomationWebhookEventStatus string

const (
	WebhookEventStatusFiltered    AutomationWebhookEventStatus = "filtered"
	WebhookEventStatusPreview     AutomationWebhookEventStatus = "preview"
	WebhookEventStatusCreated     AutomationWebhookEventStatus = "created"
	WebhookEventStatusContinued   AutomationWebhookEventStatus = "continued"
	WebhookEventStatusRateLimited AutomationWebhookEventStatus = "rate_limited"
	WebhookEventStatusError       AutomationWebhookEventStatus = "error"
)

// AutomationWebhookEvent records the outcome of a single webhook
// delivery.
type AutomationWebhookEvent struct {
	ID             uuid.UUID                    `json:"id" format:"uuid"`
	AutomationID   uuid.UUID                    `json:"automation_id" format:"uuid"`
	ReceivedAt     time.Time                    `json:"received_at" format:"date-time"`
	Payload        json.RawMessage              `json:"payload"`
	FilterMatched  bool                         `json:"filter_matched"`
	ResolvedLabels json.RawMessage              `json:"resolved_labels"`
	MatchedChatID  *uuid.UUID                   `json:"matched_chat_id,omitempty" format:"uuid"`
	CreatedChatID  *uuid.UUID                   `json:"created_chat_id,omitempty" format:"uuid"`
	Status         AutomationWebhookEventStatus `json:"status"`
	Error          *string                      `json:"error,omitempty"`
}

// AutomationTestResult is the result of a dry-run test against an
// automation's filter and session resolution logic.
type AutomationTestResult struct {
	FilterMatched   bool            `json:"filter_matched"`
	ResolvedLabels  json.RawMessage `json:"resolved_labels"`
	ExistingChatID  *uuid.UUID      `json:"existing_chat_id,omitempty" format:"uuid"`
	WouldCreateChat bool            `json:"would_create_new_chat"`
}

func (c *ExperimentalClient) CreateAutomation(ctx context.Context, req CreateAutomationRequest) (Automation, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/experimental/automations", req)
	if err != nil {
		return Automation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Automation{}, ReadBodyAsError(res)
	}
	var automation Automation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

func (c *ExperimentalClient) GetAutomation(ctx context.Context, id uuid.UUID) (Automation, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/automations/%s", id), nil)
	if err != nil {
		return Automation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Automation{}, ReadBodyAsError(res)
	}
	var automation Automation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

func (c *ExperimentalClient) ListAutomations(ctx context.Context) ([]Automation, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/experimental/automations", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var automations []Automation
	return automations, json.NewDecoder(res.Body).Decode(&automations)
}

func (c *ExperimentalClient) UpdateAutomation(ctx context.Context, id uuid.UUID, req UpdateAutomationRequest) (Automation, error) {
	res, err := c.Request(ctx, http.MethodPatch, fmt.Sprintf("/api/experimental/automations/%s", id), req)
	if err != nil {
		return Automation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Automation{}, ReadBodyAsError(res)
	}
	var automation Automation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

func (c *ExperimentalClient) DeleteAutomation(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/automations/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

func (c *ExperimentalClient) RegenerateAutomationWebhookSecret(ctx context.Context, id uuid.UUID) (Automation, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/automations/%s/regenerate-secret", id), nil)
	if err != nil {
		return Automation{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Automation{}, ReadBodyAsError(res)
	}
	var automation Automation
	return automation, json.NewDecoder(res.Body).Decode(&automation)
}

func (c *ExperimentalClient) ListAutomationWebhookEvents(ctx context.Context, id uuid.UUID) ([]AutomationWebhookEvent, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/automations/%s/events", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var events []AutomationWebhookEvent
	return events, json.NewDecoder(res.Body).Decode(&events)
}

func (c *ExperimentalClient) TestAutomation(ctx context.Context, id uuid.UUID, payload json.RawMessage) (AutomationTestResult, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/automations/%s/test", id), payload)
	if err != nil {
		return AutomationTestResult{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AutomationTestResult{}, ReadBodyAsError(res)
	}
	var result AutomationTestResult
	return result, json.NewDecoder(res.Body).Decode(&result)
}
