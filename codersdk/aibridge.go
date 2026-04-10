package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type AIBridgeInterception struct {
	ID           uuid.UUID            `json:"id" format:"uuid"`
	APIKeyID     *string              `json:"api_key_id"`
	Initiator    MinimalUser          `json:"initiator"`
	Provider     string               `json:"provider"`
	ProviderName string               `json:"provider_name"`
	Model        string               `json:"model"`
	Client       *string              `json:"client"`
	Metadata     map[string]any       `json:"metadata"`
	StartedAt    time.Time            `json:"started_at" format:"date-time"`
	EndedAt      *time.Time           `json:"ended_at" format:"date-time"`
	TokenUsages  []AIBridgeTokenUsage `json:"token_usages"`
	UserPrompts  []AIBridgeUserPrompt `json:"user_prompts"`
	ToolUsages   []AIBridgeToolUsage  `json:"tool_usages"`
}

type AIBridgeTokenUsage struct {
	ID                    uuid.UUID      `json:"id" format:"uuid"`
	InterceptionID        uuid.UUID      `json:"interception_id" format:"uuid"`
	ProviderResponseID    string         `json:"provider_response_id"`
	InputTokens           int64          `json:"input_tokens"`
	OutputTokens          int64          `json:"output_tokens"`
	CacheReadInputTokens  int64          `json:"cache_read_input_tokens"`
	CacheWriteInputTokens int64          `json:"cache_write_input_tokens"`
	Metadata              map[string]any `json:"metadata"`
	CreatedAt             time.Time      `json:"created_at" format:"date-time"`
}

type AIBridgeUserPrompt struct {
	ID                 uuid.UUID      `json:"id" format:"uuid"`
	InterceptionID     uuid.UUID      `json:"interception_id" format:"uuid"`
	ProviderResponseID string         `json:"provider_response_id"`
	Prompt             string         `json:"prompt"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          time.Time      `json:"created_at" format:"date-time"`
}

type AIBridgeToolUsage struct {
	ID                 uuid.UUID      `json:"id" format:"uuid"`
	InterceptionID     uuid.UUID      `json:"interception_id" format:"uuid"`
	ProviderResponseID string         `json:"provider_response_id"`
	ServerURL          string         `json:"server_url"`
	Tool               string         `json:"tool"`
	Input              string         `json:"input"`
	Injected           bool           `json:"injected"`
	InvocationError    string         `json:"invocation_error"`
	Metadata           map[string]any `json:"metadata"`
	CreatedAt          time.Time      `json:"created_at" format:"date-time"`
}

type AIBridgeListInterceptionsResponse struct {
	Count   int64                  `json:"count"`
	Results []AIBridgeInterception `json:"results"`
}

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
// /api/v2/aibridge/sessions/{session_id} which returns a single
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
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	Client        string    `json:"client,omitempty"`
	SessionID     string    `json:"session_id,omitempty"`

	// AfterSessionID is a cursor for pagination. It is the session ID of the
	// last session in the previous page.
	AfterSessionID string `json:"after_session_id,omitempty"`

	FilterQuery string `json:"q,omitempty"`
}

// @typescript-ignore AIBridgeListInterceptionsFilter
type AIBridgeListInterceptionsFilter struct {
	// Limit defaults to 100, max is 1000.
	// Offset based pagination is not supported for AI Bridge interceptions. Use
	// cursor pagination instead with after_id.
	Pagination Pagination `json:"pagination,omitempty"`

	// Initiator is a user ID, username, or "me".
	Initiator     string    `json:"initiator,omitempty"`
	StartedBefore time.Time `json:"started_before,omitempty" format:"date-time"`
	StartedAfter  time.Time `json:"started_after,omitempty" format:"date-time"`
	Provider      string    `json:"provider,omitempty"`
	Model         string    `json:"model,omitempty"`
	Client        string    `json:"client,omitempty"`

	FilterQuery string `json:"q,omitempty"`
}

// asRequestOption returns a function that can be used in (*Client).Request.
// It modifies the request query parameters.
func (f AIBridgeListInterceptionsFilter) asRequestOption() RequestOption {
	return func(r *http.Request) {
		var params []string
		// Make sure all user input is quoted to ensure it's parsed as a single
		// string.
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
		if f.Model != "" {
			params = append(params, fmt.Sprintf("model:%q", f.Model))
		}
		if f.Client != "" {
			params = append(params, fmt.Sprintf("client:%q", f.Client))
		}
		if f.FilterQuery != "" {
			// If custom stuff is added, just add it on here.
			params = append(params, f.FilterQuery)
		}

		q := r.URL.Query()
		q.Set("q", strings.Join(params, " "))
		r.URL.RawQuery = q.Encode()
	}
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

// AIBridgeListInterceptions returns AI Bridge interceptions with the given
// filter.
func (c *Client) AIBridgeListInterceptions(ctx context.Context, filter AIBridgeListInterceptionsFilter) (AIBridgeListInterceptionsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/interceptions", nil, filter.asRequestOption(), filter.Pagination.asRequestOption(), filter.Pagination.asRequestOption())
	if err != nil {
		return AIBridgeListInterceptionsResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AIBridgeListInterceptionsResponse{}, ReadBodyAsError(res)
	}
	var resp AIBridgeListInterceptionsResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}

// AIBridgeListSessions returns AI Bridge sessions with the given filter.
func (c *Client) AIBridgeListSessions(ctx context.Context, filter AIBridgeListSessionsFilter) (AIBridgeListSessionsResponse, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/sessions", nil, filter.asRequestOption(), filter.Pagination.asRequestOption())
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
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/aibridge/sessions/%s", sessionID), nil, func(r *http.Request) {
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
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/aibridge/clients", nil)
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
