package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

type AIBridgeSession struct {
	ID                string                           `json:"id"`
	Initiator         MinimalUser                      `json:"initiator"`
	Providers         []string                         `json:"providers"`
	Models            []string                         `json:"models"`
	Client            *string                          `json:"client"`
	Metadata          map[string]any                   `json:"metadata"`
	StartedAt         time.Time                        `json:"started_at" format:"date-time"`
	EndedAt           *time.Time                       `json:"ended_at,omitempty" format:"date-time"`
	Threads           int64                            `json:"threads"`
	TokenUsageSummary AIBridgeSessionTokenUsageSummary `json:"token_usage_summary"`
	LastPrompt        *string                          `json:"last_prompt,omitempty"`
	LastActiveAt      time.Time                        `json:"last_active_at" format:"date-time"`
}

type AIBridgeSessionTokenUsageSummary struct {
	InputTokens           int64 `json:"input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	CacheReadInputTokens  int64 `json:"cache_read_input_tokens"`
	CacheWriteInputTokens int64 `json:"cache_write_input_tokens"`
}

type AIBridgeListSessionsResponse struct {
	Count    int64             `json:"count"`
	Sessions []AIBridgeSession `json:"sessions"`
}

// AIBridgeSessionThreadsResponse is the response for GET
// /api/v2/ai-gateway/sessions/{session_id} which returns a single
// session with fully expanded threads.
type AIBridgeSessionThreadsResponse struct {
	ID                string                           `json:"id"`
	Initiator         MinimalUser                      `json:"initiator"`
	Providers         []string                         `json:"providers"`
	Models            []string                         `json:"models"`
	Client            *string                          `json:"client,omitempty"`
	Metadata          map[string]any                   `json:"metadata"`
	PageStartedAt     *time.Time                       `json:"page_started_at,omitempty" format:"date-time"`
	PageEndedAt       *time.Time                       `json:"page_ended_at,omitempty" format:"date-time"`
	StartedAt         time.Time                        `json:"started_at" format:"date-time"`
	EndedAt           *time.Time                       `json:"ended_at,omitempty" format:"date-time"`
	TokenUsageSummary AIBridgeSessionThreadsTokenUsage `json:"token_usage_summary"`
	Threads           []AIBridgeThread                 `json:"threads"`
}

// AIBridgeSessionThreadsTokenUsage represents aggregated token usage
// with metadata containing provider-specific fields.
type AIBridgeSessionThreadsTokenUsage struct {
	InputTokens           int64          `json:"input_tokens"`
	OutputTokens          int64          `json:"output_tokens"`
	CacheReadInputTokens  int64          `json:"cache_read_input_tokens"`
	CacheWriteInputTokens int64          `json:"cache_write_input_tokens"`
	Metadata              map[string]any `json:"metadata"`
}

// AIBridgeThread represents a single thread within a session.
// A thread groups interceptions by their thread_root_id.
type AIBridgeThread struct {
	ID             uuid.UUID                        `json:"id" format:"uuid"`
	Prompt         *string                          `json:"prompt,omitempty"`
	Model          string                           `json:"model"`
	Provider       string                           `json:"provider"`
	CredentialKind string                           `json:"credential_kind"`
	CredentialHint string                           `json:"credential_hint"`
	StartedAt      time.Time                        `json:"started_at" format:"date-time"`
	EndedAt        *time.Time                       `json:"ended_at,omitempty" format:"date-time"`
	TokenUsage     AIBridgeSessionThreadsTokenUsage `json:"token_usage"`
	AgenticActions []AIBridgeAgenticAction          `json:"agentic_actions"`
	// AgentFirewallSessionID links this thread to an agent firewall
	// confinement session. Nil when the request did not pass through
	// the agent firewall.
	AgentFirewallSessionID *uuid.UUID `json:"agent_firewall_session_id,omitempty" format:"uuid"`
	// AgentFirewallSequenceNumber is the firewall sequence number from
	// the root interception. Used to determine the position of this
	// LLM request in the firewall event stream. Nil when the request
	// did not pass through the agent firewall.
	AgentFirewallSequenceNumber *int32 `json:"agent_firewall_sequence_number,omitempty"`
}

// AIBridgeAgenticAction represents a tool call with associated
// thinking blocks and token usage from one or more interceptions.
type AIBridgeAgenticAction struct {
	Model      string                           `json:"model"`
	TokenUsage AIBridgeSessionThreadsTokenUsage `json:"token_usage"`
	Thinking   []AIBridgeModelThought           `json:"thinking"`
	ToolCalls  []AIBridgeToolCall               `json:"tool_calls"`
}

// AIBridgeModelThought represents a single thinking block from
// the model.
type AIBridgeModelThought struct {
	Text string `json:"text"`
}

// AIBridgeToolCall represents a tool call recorded during an
// interception.
type AIBridgeToolCall struct {
	ID                 uuid.UUID      `json:"id" format:"uuid"`
	InterceptionID     uuid.UUID      `json:"interception_id" format:"uuid"`
	ProviderResponseID string         `json:"provider_response_id"`
	ServerURL          string         `json:"server_url"`
	Tool               string         `json:"tool"`
	Injected           bool           `json:"injected"`
	Input              string         `json:"input"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          time.Time      `json:"created_at" format:"date-time"`
}

// @typescript-ignore AIBridgeListSessionsFilter
type AIBridgeListSessionsFilter struct {
	// Limit defaults to 100, max is 1000.
	Pagination Pagination `json:"pagination,omitempty"`

	// Initiator is a user ID, username, or "me".
	Initiator     string    `json:"initiator,omitempty"`
	StartedBefore time.Time `json:"started_before,omitempty" format:"date-time"`
	StartedAfter  time.Time `json:"started_after,omitempty" format:"date-time"`
	// Provider matches the runtime provider type column (openai,
	// anthropic, copilot). The runtime type collapses the configured
	// ai_provider_type: azure, google, openai-compat, openrouter, and
	// vercel route through openai; bedrock routes through anthropic.
	// Retained for backward compatibility; new clients should prefer
	// ProviderName, which scopes to a specific configured row.
	Provider     string `json:"provider,omitempty"`
	ProviderName string `json:"provider_name,omitempty"`
	Model        string `json:"model,omitempty"`
	Client       string `json:"client,omitempty"`
	SessionID    string `json:"session_id,omitempty"`

	// AfterSessionID is a cursor for pagination. It is the session ID of the
	// last session in the previous page.
	AfterSessionID string `json:"after_session_id,omitempty"`

	FilterQuery string `json:"q,omitempty"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
func (f AIBridgeListSessionsFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		var params []string
		if f.Initiator != "" {
			params = append(params, fmt.Sprintf("initiator:%q", f.Initiator))
		}
		if !f.StartedBefore.IsZero() {
			params = append(params, fmt.Sprintf("started_before:%q", f.StartedBefore.Format(time.RFC3339Nano)))
		}
		if !f.StartedAfter.IsZero() {
			params = append(params, fmt.Sprintf("started_after:%q", f.StartedAfter.Format(time.RFC3339Nano)))
		}
		if f.Provider != "" {
			params = append(params, fmt.Sprintf("provider:%q", f.Provider))
		}
		if f.ProviderName != "" {
			params = append(params, fmt.Sprintf("provider_name:%q", f.ProviderName))
		}
		if f.Model != "" {
			params = append(params, fmt.Sprintf("model:%q", f.Model))
		}
		if f.Client != "" {
			params = append(params, fmt.Sprintf("client:%q", f.Client))
		}
		if f.SessionID != "" {
			params = append(params, fmt.Sprintf("session_id:%q", f.SessionID))
		}
		if f.FilterQuery != "" {
			params = append(params, f.FilterQuery)
		}

		q := r.URL.Query()
		q.Set("q", strings.Join(params, " "))
		if f.AfterSessionID != "" {
			q.Set("after_session_id", f.AfterSessionID)
		}
		r.URL.RawQuery = q.Encode()
	}
}

// AIBridgeListSessions returns AI Bridge sessions with the given filter.
func (c *Client) AIBridgeListSessions(ctx context.Context, filter AIBridgeListSessionsFilter) (AIBridgeListSessionsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/ai-gateway/sessions", nil, filter.asRequestOption(), filter.Pagination.asRequestOption())
	if err != nil {
		return AIBridgeListSessionsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIBridgeListSessionsResponse{}, ReadBodyAsError(res)
	}
	var resp AIBridgeListSessionsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AIBridgeGetSessionThreads returns a single session with expanded
// thread details including agentic actions and thinking blocks.
func (c *Client) AIBridgeGetSessionThreads(ctx context.Context, sessionID string, afterID, beforeID uuid.UUID, limit int32) (AIBridgeSessionThreadsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/ai-gateway/sessions/%s", sessionID), nil, func(r *http.Request) {
		q := r.URL.Query()
		if afterID != uuid.Nil {
			q.Set("after_id", afterID.String())
		}
		if beforeID != uuid.Nil {
			q.Set("before_id", beforeID.String())
		}
		if limit > 0 {
			q.Set("limit", fmt.Sprintf("%d", limit))
		}
		r.URL.RawQuery = q.Encode()
	})
	if err != nil {
		return AIBridgeSessionThreadsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIBridgeSessionThreadsResponse{}, ReadBodyAsError(res)
	}
	var resp AIBridgeSessionThreadsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AIBridgeListClients returns the distinct AI clients visible to the caller.
func (c *Client) AIBridgeListClients(ctx context.Context) ([]string, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/ai-gateway/clients", nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var clients []string
	return clients, json.NewDecoder(res.Body).Decode(&clients)
}

type GroupAIBudget struct {
	GroupID          uuid.UUID `json:"group_id" format:"uuid"`
	SpendLimitMicros int64     `json:"spend_limit_micros"`
	CreatedAt        time.Time `json:"created_at" format:"date-time"`
	UpdatedAt        time.Time `json:"updated_at" format:"date-time"`
}

type UpsertGroupAIBudgetRequest struct {
	SpendLimitMicros int64 `json:"spend_limit_micros" validate:"gte=0"`
}

// GroupAIBudget returns the AI spend budget configured for the given group.
func (c *Client) GroupAIBudget(ctx context.Context, group uuid.UUID) (GroupAIBudget, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/groups/%s/ai/budget", group.String()),
		nil,
	)
	if err != nil {
		return GroupAIBudget{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupAIBudget{}, ReadBodyAsError(res)
	}
	var resp GroupAIBudget
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpsertGroupAIBudget creates or updates the AI spend budget for the given group.
func (c *Client) UpsertGroupAIBudget(ctx context.Context, group uuid.UUID, req UpsertGroupAIBudgetRequest) (GroupAIBudget, error) {
	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/groups/%s/ai/budget", group.String()),
		req,
	)
	if err != nil {
		return GroupAIBudget{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return GroupAIBudget{}, ReadBodyAsError(res)
	}
	var resp GroupAIBudget
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteGroupAIBudget removes the AI spend budget for the given group.
func (c *Client) DeleteGroupAIBudget(ctx context.Context, group uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/groups/%s/ai/budget", group.String()),
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

type UserAIBudgetOverride struct {
	UserID           uuid.UUID `json:"user_id" format:"uuid"`
	GroupID          uuid.UUID `json:"group_id" format:"uuid"`
	SpendLimitMicros int64     `json:"spend_limit_micros"`
	CreatedAt        time.Time `json:"created_at" format:"date-time"`
	UpdatedAt        time.Time `json:"updated_at" format:"date-time"`
}

type UpsertUserAIBudgetOverrideRequest struct {
	// GroupID is the group the user's spend is attributed to. The user must
	// be a member of this group.
	GroupID          uuid.UUID `json:"group_id" format:"uuid" validate:"required"`
	SpendLimitMicros int64     `json:"spend_limit_micros" validate:"gte=0"`
}

// UserAIBudgetOverride returns the AI spend budget override configured for the given user.
func (c *Client) UserAIBudgetOverride(ctx context.Context, user uuid.UUID) (UserAIBudgetOverride, error) {
	res, err := c.Request(ctx, http.MethodGet,
		fmt.Sprintf("/api/v2/users/%s/ai/budget", user.String()),
		nil,
	)
	if err != nil {
		return UserAIBudgetOverride{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return UserAIBudgetOverride{}, ReadBodyAsError(res)
	}
	var resp UserAIBudgetOverride
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// UpsertUserAIBudgetOverride creates or updates the AI spend budget override for the given user.
func (c *Client) UpsertUserAIBudgetOverride(ctx context.Context, user uuid.UUID, req UpsertUserAIBudgetOverrideRequest) (UserAIBudgetOverride, error) {
	res, err := c.Request(ctx, http.MethodPut,
		fmt.Sprintf("/api/v2/users/%s/ai/budget", user.String()),
		req,
	)
	if err != nil {
		return UserAIBudgetOverride{}, xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return UserAIBudgetOverride{}, ReadBodyAsError(res)
	}
	var resp UserAIBudgetOverride
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// DeleteUserAIBudgetOverride removes the AI spend budget override for the given user.
func (c *Client) DeleteUserAIBudgetOverride(ctx context.Context, user uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v2/users/%s/ai/budget", user.String()),
		nil,
	)
	if err != nil {
		return xerrors.Errorf("make request: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
