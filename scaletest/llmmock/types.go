package llmmock

import (
	"time"

	"github.com/google/uuid"
)

// Provider represents the LLM provider type.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
)

// RequestSummary contains metadata about an intercepted LLM API request.
type RequestSummary struct {
	ID        uuid.UUID `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Provider  Provider  `json:"provider"`
	Model     string    `json:"model"`
	UserID    string    `json:"user_id,omitempty"`
	Stream    bool      `json:"stream"`
	// Request body as JSON string for reference
	RequestBody string `json:"request_body,omitempty"`
}

// ResponseSummary contains metadata about an LLM API response.
type ResponseSummary struct {
	RequestID    uuid.UUID `json:"request_id"`
	Timestamp    time.Time `json:"timestamp"`
	Status       int       `json:"status"`
	Stream       bool      `json:"stream"`
	FinishReason string    `json:"finish_reason,omitempty"` // OpenAI: finish_reason, Anthropic: stop_reason
	PromptTokens int       `json:"prompt_tokens,omitempty"`
	OutputTokens int       `json:"output_tokens,omitempty"` // OpenAI: completion_tokens, Anthropic: output_tokens
	TotalTokens  int       `json:"total_tokens,omitempty"`
	// Response body as JSON string for reference (non-streaming) or first chunk (streaming)
	ResponseBody string `json:"response_body,omitempty"`
}

// RequestRecord combines request and response information.
type RequestRecord struct {
	Request  RequestSummary   `json:"request"`
	Response *ResponseSummary `json:"response,omitempty"`
}
