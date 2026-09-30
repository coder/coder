// Package extract defines read-only extractors that record facts about AI
// provider traffic without taking part in the HTTP request lifecycle.
//
// Extractors never touch, block, or alter the traffic they observe. They
// write recorder records (prompt, token, tool, and model thought usage) as
// they observe the traffic, and keep only the small outcome a caller needs
// to end the interception. Parse failures reduce recorded detail and are
// logged as parse notes, never reported as traffic errors.
//
// Provider-specific extractors live in subpackages (for example
// extract/responses); this package holds the shared interfaces and the
// transport adapters for SSE streams and complete bodies.
//
// Bodies are passed as []byte: callers fill one reusable buffer once and
// share it between forwarding and extraction.
package extract

const (
	// MaxEventBytes bounds a single streamed event. Larger events are
	// skipped with a parse note.
	MaxEventBytes = 4 << 20
	// MaxBodyBytes bounds a complete (non-streaming) response body after
	// decoding. Larger bodies are skipped with a parse note.
	MaxBodyBytes = 8 << 20
)

// RequestFacts are the facts derived from a request body. Model, Streaming,
// and CorrelatingToolCallID are known before any response exists, for the
// interception start record. Prompt is passed on to the response
// extraction, which records it once the provider response ID is known.
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

// ResponseExtraction observes one upstream response and records what it
// finds. SSE, complete-body, and WebSocket transports all feed it the same
// provider JSON.
//
// A ResponseExtraction is not safe for concurrent use: callers must call
// its methods sequentially, feeding events in stream order. It never
// blocks beyond its recorder calls, never retains or mutates the passed
// slices after returning, and tolerates any input.
type ResponseExtraction interface {
	// OnEvent observes one streamed event. eventType is the transport-level
	// event name (the SSE "event" field) and may be empty, for example for
	// WebSocket frames; raw is the event JSON.
	OnEvent(eventType string, raw []byte)
	// ProcessBlocking observes a complete, decoded response body and its
	// HTTP status code: either a non-streaming response or an error
	// response to any request. DecodeBody produces raw from the wire body.
	ProcessBlocking(statusCode int, raw []byte)
	// Outcome reports how the response ended so far. It is meant to be
	// read after the terminal event, or when the stream ends.
	Outcome() Outcome
}

// Outcome is what a caller needs to end an interception.
type Outcome struct {
	// ResponseID is the provider's response or message ID, empty if none
	// was seen.
	ResponseID string
	Terminal   Terminal
	// Err is the provider-shaped terminal error, if the provider reported
	// one. It is the same error type the provider's interceptors return, so
	// interceptionerror.Categorize applies unchanged.
	Err error
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
