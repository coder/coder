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
	CredentialKind        string
	CredentialHint        string
}

type InterceptionRecordEnded struct {
	ID      string
	EndedAt time.Time
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
	InterceptionID  string
	MsgID           string
	Tool            string
	ToolCallID      string
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
