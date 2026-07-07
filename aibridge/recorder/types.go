package recorder

import (
	"context"
	"time"
)

// Recorder describes all the possible usage information we need to capture during interactions with AI providers.
// Additionally, it introduces the concept of an "Interception", which includes information about which provider/model was
// used and by whom. All usage records should reference this Interception by ID.
type Recorder interface {
	// RecordInterception records metadata about an interception with an upstream AI provider.
	RecordInterception(ctx context.Context, req *InterceptionRecord) error
	// RecordInterceptionEnded records that given interception has completed.
	RecordInterceptionEnded(ctx context.Context, req *InterceptionRecordEnded) error
	// RecordTokenUsage records the tokens used in an interception with an upstream AI provider.
	RecordTokenUsage(ctx context.Context, req *TokenUsageRecord) error
	// RecordPromptUsage records the prompts used in an interception with an upstream AI provider.
	RecordPromptUsage(ctx context.Context, req *PromptUsageRecord) error
	// RecordToolUsage records the tools used in an interception with an upstream AI provider.
	RecordToolUsage(ctx context.Context, req *ToolUsageRecord) error
	// RecordModelThought records model thoughts produced in an interception with an upstream AI provider.
	RecordModelThought(ctx context.Context, req *ModelThoughtRecord) error
}

type ToolArgs any

type Metadata map[string]any

type InterceptionRecord struct {
	ID                    string
	InitiatorID           string
	Metadata              Metadata
	Model                 string
	Provider              string
	ProviderName          string
	StartedAt             time.Time
	ClientSessionID       *string
	Client                string
	UserAgent             string
	CorrelatingToolCallID *string
	// AgentFirewallSessionID is the UUID of the Agent Firewall session
	// that produced this request. Nil when the request did not pass
	// through Agent Firewall.
	AgentFirewallSessionID *string
	// AgentFirewallSequenceNumber is the monotonically increasing
	// sequence number assigned by Agent Firewall. Nil when the request
	// did not pass through Agent Firewall.
	AgentFirewallSequenceNumber *int32
	// CredentialKind is always set: either BYOK or centralized.
	CredentialKind string
	// CredentialHint is only set for BYOK, where the key is known
	// from the request. Centralized uses key failover, so the hint
	// can only be determined at end-of-interception.
	CredentialHint string
}

type InterceptionRecordEnded struct {
	ID      string
	EndedAt time.Time
	// CredentialHint is the hint observed at end-of-interception.
	// Only applied to the DB row for centralized; ignored for BYOK.
	CredentialHint string
}

type TokenUsageRecord struct {
	InterceptionID        string
	MsgID                 string
	Input                 int64
	Output                int64
	CacheReadInputTokens  int64
	CacheWriteInputTokens int64
	// ExtraTokenTypes holds token types which *may* exist over and above input/output.
	// These should ultimately get merged into [Metadata], but it's useful to keep these
	// with their actual type (int64) since [Metadata] is a map[string]any.
	ExtraTokenTypes map[string]int64
	Metadata        Metadata
	CreatedAt       time.Time
}

type PromptUsageRecord struct {
	InterceptionID string
	MsgID          string
	Prompt         string
	Metadata       Metadata
	CreatedAt      time.Time
}

type ToolUsageRecord struct {
	InterceptionID string
	MsgID          string
	Tool           string
	// ToolCallID is the correlation ID used to match a tool call to its
	// result (call_id in the Responses API, id in chat completions and
	// Anthropic messages). It is empty for hosted Responses tools (e.g.
	// web_search_call) which the provider executes internally.
	ToolCallID string
	// ItemID is the provider's unique ID for the output item that carried
	// the tool call. It is specific to the OpenAI Responses API, where an
	// output item has both an id and a call_id. It is empty for the chat
	// completions and Anthropic messages APIs, which have no separate item
	// ID concept.
	ItemID          string
	ServerURL       *string
	Args            ToolArgs
	Injected        bool
	InvocationError error
	Metadata        Metadata
	CreatedAt       time.Time
}

// Model thought source constants.
const (
	ThoughtSourceThinking         = "thinking"
	ThoughtSourceReasoningSummary = "reasoning_summary"
	ThoughtSourceCommentary       = "commentary"
)

type ModelThoughtRecord struct {
	InterceptionID string
	Content        string
	Metadata       Metadata
	CreatedAt      time.Time
}
