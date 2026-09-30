// Package extract defines read-only extractors that derive recording facts
// from AI provider traffic without taking part in the HTTP request lifecycle.
//
// Extractors never touch, block, or alter the traffic they observe. Parse
// failures reduce recorded detail and are reported as notes, never as
// traffic errors. Provider-specific extractors live in subpackages (for
// example extract/responses); this package holds the shared interfaces, the
// transport adapters (SSE and complete bodies), and the conversion into
// recorder records.
package extract

import "github.com/coder/coder/v2/aibridge/recorder"

const (
	// MaxEventBytes bounds a single streamed event. Larger events are
	// skipped with a note.
	MaxEventBytes = 4 << 20
	// MaxBodyBytes bounds a complete (non-streaming) response body after
	// decoding. Larger bodies are skipped with a note.
	MaxBodyBytes = 8 << 20
)

// RequestFacts are the recording facts derived from a request body.
type RequestFacts struct {
	Model     string
	Streaming bool
	// Prompt is the last user prompt, empty if none.
	Prompt string
	// CorrelatingToolCallID is the tool call ID the request answers, for
	// example the call_id of a trailing function_call_output item. Empty if
	// none.
	CorrelatingToolCallID string
}

// RequestExtractor derives facts from a raw request body. A returned error
// is a parse note: the returned facts hold whatever could be read.
type RequestExtractor interface {
	ExtractRequest(body []byte) (RequestFacts, error)
}

// ResponseExtraction accumulates facts for one upstream response. SSE,
// complete-body, and WebSocket transports all feed it the same provider
// event JSON. Implementations must never block, never retain or mutate the
// passed slices after returning, and must tolerate any input.
type ResponseExtraction interface {
	// OnEvent observes one streamed event. eventType is the transport-level
	// event name (the SSE "event" field) and may be empty, for example for
	// WebSocket frames; raw is the event JSON.
	OnEvent(eventType string, raw []byte)
	// OnBody observes a complete response body and its HTTP status code:
	// either a non-streaming response or an error response to any request.
	OnBody(statusCode int, raw []byte)
	// Result returns the facts gathered so far. It may be called at any
	// time, including on a truncated stream.
	Result() ResponseFacts
}

// ResponseFacts are the recording facts derived from one upstream response.
type ResponseFacts struct {
	// ResponseID is the provider's response or message ID.
	ResponseID string
	// Model is the model reported by the provider.
	Model string
	// ServiceTier is the provider's service tier for the response, if any.
	ServiceTier string
	// Usage is nil when the response reported no usage.
	Usage     *TokenUsage
	ToolCalls []ToolCall
	Thoughts  []Thought
	Terminal  Terminal
	// Err is the provider-shaped terminal error, if the provider reported
	// one. It is the same error type the provider's interceptors return, so
	// provider error categorization applies unchanged.
	Err error
	// ParseNotes describe input that was skipped because it could not be
	// parsed. They never indicate a traffic error.
	ParseNotes []string
}

// TokenUsage is normalized token usage, matching the fields of
// recorder.TokenUsageRecord. Input excludes cache reads and writes.
type TokenUsage struct {
	Input           int64
	Output          int64
	CacheReadInput  int64
	CacheWriteInput int64
	// Extra holds provider-specific token types, for example reasoning
	// output tokens.
	Extra map[string]int64
}

// ToolCall is a tool call the provider asked for or executed.
type ToolCall struct {
	// ItemID is the provider's output item ID, if the API has one.
	ItemID string
	// CallID correlates the call with its result. Empty for hosted tools
	// the provider executes itself.
	CallID string
	Name   string
	Args   recorder.ToolArgs
}

// Thought is model reasoning or commentary surfaced by the provider.
type Thought struct {
	Content string
	// Source is one of the recorder.ThoughtSource* constants.
	Source string
}

// TerminalStatus is how a response ended.
type TerminalStatus string

const (
	// TerminalNone means no terminal event was observed, for example on a
	// truncated stream.
	TerminalNone       TerminalStatus = ""
	TerminalCompleted  TerminalStatus = "completed"
	TerminalIncomplete TerminalStatus = "incomplete"
	TerminalFailed     TerminalStatus = "failed"
)

// Terminal describes how a response ended.
type Terminal struct {
	Status TerminalStatus
	// Reason is the provider's incomplete reason or error code, if any.
	Reason string
}
