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

// Automation represents an automation that bridges external triggers to
// Coder chats.
type Automation struct {
	ID                    uuid.UUID        `json:"id" format:"uuid"`
	OwnerID               uuid.UUID        `json:"owner_id" format:"uuid"`
	OrganizationID        uuid.UUID        `json:"organization_id" format:"uuid"`
	Name                  string           `json:"name"`
	Description           string           `json:"description"`
	Instructions          string           `json:"instructions"`
	ModelConfigID         *uuid.UUID       `json:"model_config_id,omitempty" format:"uuid"`
	MCPServerIDs          []uuid.UUID      `json:"mcp_server_ids"`
	AllowedTools          []string         `json:"allowed_tools"`
	Status                AutomationStatus `json:"status"`
	MaxChatCreatesPerHour int32            `json:"max_chat_creates_per_hour"`
	MaxMessagesPerHour    int32            `json:"max_messages_per_hour"`
	CreatedAt             time.Time        `json:"created_at" format:"date-time"`
	UpdatedAt             time.Time        `json:"updated_at" format:"date-time"`
}

// CreateAutomationRequest is the request body for creating an automation.
type CreateAutomationRequest struct {
	Name                  string      `json:"name"`
	Description           string      `json:"description,omitempty"`
	Instructions          string      `json:"instructions,omitempty"`
	ModelConfigID         *uuid.UUID  `json:"model_config_id,omitempty" format:"uuid"`
	MCPServerIDs          []uuid.UUID `json:"mcp_server_ids,omitempty"`
	AllowedTools          []string    `json:"allowed_tools,omitempty"`
	MaxChatCreatesPerHour *int32      `json:"max_chat_creates_per_hour,omitempty"`
	MaxMessagesPerHour    *int32      `json:"max_messages_per_hour,omitempty"`
}

// UpdateAutomationRequest is the request body for updating an automation.
type UpdateAutomationRequest struct {
	Name                  *string           `json:"name,omitempty"`
	Description           *string           `json:"description,omitempty"`
	Instructions          *string           `json:"instructions,omitempty"`
	ModelConfigID         *uuid.UUID        `json:"model_config_id,omitempty" format:"uuid"`
	MCPServerIDs          *[]uuid.UUID      `json:"mcp_server_ids,omitempty"`
	AllowedTools          *[]string         `json:"allowed_tools,omitempty"`
	Status                *AutomationStatus `json:"status,omitempty"`
	MaxChatCreatesPerHour *int32            `json:"max_chat_creates_per_hour,omitempty"`
	MaxMessagesPerHour    *int32            `json:"max_messages_per_hour,omitempty"`
}

// AutomationTriggerType represents the type of trigger.
type AutomationTriggerType string

const (
	AutomationTriggerTypeWebhook AutomationTriggerType = "webhook"
	AutomationTriggerTypeCron    AutomationTriggerType = "cron"
)

// AutomationTrigger represents a trigger attached to an automation.
type AutomationTrigger struct {
	ID           uuid.UUID             `json:"id" format:"uuid"`
	AutomationID uuid.UUID             `json:"automation_id" format:"uuid"`
	Type         AutomationTriggerType `json:"type"`
	WebhookURL   string                `json:"webhook_url,omitempty"`
	CronSchedule *string               `json:"cron_schedule,omitempty"`
	Filter       json.RawMessage       `json:"filter"`
	LabelPaths   json.RawMessage       `json:"label_paths"`
	CreatedAt    time.Time             `json:"created_at" format:"date-time"`
	UpdatedAt    time.Time             `json:"updated_at" format:"date-time"`
}

// CreateAutomationTriggerRequest is the request body for creating a
// trigger.
type CreateAutomationTriggerRequest struct {
	Type         AutomationTriggerType `json:"type"`
	CronSchedule *string               `json:"cron_schedule,omitempty"`
	Filter       json.RawMessage       `json:"filter,omitempty"`
	LabelPaths   json.RawMessage       `json:"label_paths,omitempty"`
}

// UpdateAutomationTriggerRequest is the request body for updating a
// trigger.
type UpdateAutomationTriggerRequest struct {
	CronSchedule *string         `json:"cron_schedule,omitempty"`
	Filter       json.RawMessage `json:"filter,omitempty"`
	LabelPaths   json.RawMessage `json:"label_paths,omitempty"`
}

// AutomationEventStatus represents the outcome of an automation event.
type AutomationEventStatus string

const (
	AutomationEventStatusFiltered    AutomationEventStatus = "filtered"
	AutomationEventStatusPreview     AutomationEventStatus = "preview"
	AutomationEventStatusCreated     AutomationEventStatus = "created"
	AutomationEventStatusContinued   AutomationEventStatus = "continued"
	AutomationEventStatusRateLimited AutomationEventStatus = "rate_limited"
	AutomationEventStatusError       AutomationEventStatus = "error"
)

// AutomationEvent records the outcome of a single automation event.
type AutomationEvent struct {
	ID             uuid.UUID             `json:"id" format:"uuid"`
	AutomationID   uuid.UUID             `json:"automation_id" format:"uuid"`
	TriggerID      *uuid.UUID            `json:"trigger_id,omitempty" format:"uuid"`
	ReceivedAt     time.Time             `json:"received_at" format:"date-time"`
	Payload        json.RawMessage       `json:"payload"`
	FilterMatched  bool                  `json:"filter_matched"`
	ResolvedLabels json.RawMessage       `json:"resolved_labels"`
	MatchedChatID  *uuid.UUID            `json:"matched_chat_id,omitempty" format:"uuid"`
	CreatedChatID  *uuid.UUID            `json:"created_chat_id,omitempty" format:"uuid"`
	Status         AutomationEventStatus `json:"status"`
	Error          *string               `json:"error,omitempty"`
}

// AutomationTestResult is the result of a dry-run test against an
// automation trigger's filter and session resolution logic.
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

func (c *ExperimentalClient) ListAutomationEvents(ctx context.Context, id uuid.UUID) ([]AutomationEvent, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/automations/%s/events", id), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var events []AutomationEvent
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

// CreateAutomationTrigger creates a new trigger for an automation.
func (c *ExperimentalClient) CreateAutomationTrigger(ctx context.Context, automationID uuid.UUID, req CreateAutomationTriggerRequest) (AutomationTrigger, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/automations/%s/triggers", automationID), req)
	if err != nil {
		return AutomationTrigger{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return AutomationTrigger{}, ReadBodyAsError(res)
	}
	var trigger AutomationTrigger
	return trigger, json.NewDecoder(res.Body).Decode(&trigger)
}

// ListAutomationTriggers lists all triggers for an automation.
func (c *ExperimentalClient) ListAutomationTriggers(ctx context.Context, automationID uuid.UUID) ([]AutomationTrigger, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/experimental/automations/%s/triggers", automationID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var triggers []AutomationTrigger
	return triggers, json.NewDecoder(res.Body).Decode(&triggers)
}

// DeleteAutomationTrigger deletes a trigger from an automation.
func (c *ExperimentalClient) DeleteAutomationTrigger(ctx context.Context, automationID uuid.UUID, triggerID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/experimental/automations/%s/triggers/%s", automationID, triggerID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// RegenerateAutomationTriggerSecret regenerates the webhook secret for
// a trigger.
func (c *ExperimentalClient) RegenerateAutomationTriggerSecret(ctx context.Context, automationID uuid.UUID, triggerID uuid.UUID) (AutomationTrigger, error) {
	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/experimental/automations/%s/triggers/%s/regenerate-secret", automationID, triggerID), nil)
	if err != nil {
		return AutomationTrigger{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AutomationTrigger{}, ReadBodyAsError(res)
	}
	var trigger AutomationTrigger
	return trigger, json.NewDecoder(res.Body).Decode(&trigger)
}
