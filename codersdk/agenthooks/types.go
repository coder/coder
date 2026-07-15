// Package agenthooks defines the wire protocol for Coder agent lifecycle hooks.
package agenthooks

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// SchemaVersion is the current lifecycle hook request schema version.
const SchemaVersion = 1

// EventType identifies an agent lifecycle event.
type EventType string

const (
	EventSessionStart     EventType = "session_start"
	EventUserPromptSubmit EventType = "user_prompt_submit"
	EventPreToolUse       EventType = "pre_tool_use"
	EventPostToolUse      EventType = "post_tool_use"
	EventPreCompact       EventType = "pre_compact"
	EventPostCompact      EventType = "post_compact"
	EventStop             EventType = "stop"
)

// Request is the body coderd posts to the configured lifecycle hook URL.
type Request struct {
	Type EventType       `json:"type"`
	Meta Meta            `json:"meta"`
	Data json.RawMessage `json:"data"`
}

// Decode returns the typed event data for the request event type.
func (r Request) Decode() (any, error) {
	var data any
	switch r.Type {
	case EventSessionStart:
		data = &SessionStartData{}
	case EventUserPromptSubmit:
		data = &UserPromptSubmitData{}
	case EventPreToolUse:
		data = &PreToolUseData{}
	case EventPostToolUse:
		data = &PostToolUseData{}
	case EventPreCompact:
		data = &PreCompactData{}
	case EventPostCompact:
		data = &PostCompactData{}
	case EventStop:
		data = &StopData{}
	default:
		return nil, xerrors.Errorf("unknown event type %q", r.Type)
	}

	if err := json.Unmarshal(r.Data, data); err != nil {
		return nil, xerrors.Errorf("decode %q event data: %w", r.Type, err)
	}
	return data, nil
}

// Meta contains fields common to every lifecycle hook event.
type Meta struct {
	DispatchID    uuid.UUID  `json:"dispatch_id"`
	SchemaVersion int        `json:"schema_version"`
	ChatID        uuid.UUID  `json:"chat_id"`
	OwnerID       uuid.UUID  `json:"owner_id"`
	WorkspaceID   *uuid.UUID `json:"workspace_id,omitempty"`
	TurnID        *uuid.UUID `json:"turn_id,omitempty"`
}

// SessionStartData describes the start or resumption of a chat session.
type SessionStartData struct {
	Source string `json:"source"`
}

// UserPromptSubmitData contains the submitted user prompt.
type UserPromptSubmitData struct {
	Prompt string `json:"prompt"`
}

// PreToolUseData describes a tool call before execution.
type PreToolUseData struct {
	ToolUseID uuid.UUID       `json:"tool_use_id"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// PostToolUseData describes a completed or failed tool call.
type PostToolUseData struct {
	ToolUseID    uuid.UUID       `json:"tool_use_id"`
	ToolName     string          `json:"tool_name"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
	ToolError    string          `json:"tool_error,omitempty"`
}

// PreCompactData is emitted before chat compaction.
type PreCompactData struct{}

// PostCompactData is emitted after chat compaction.
type PostCompactData struct{}

// StopData is emitted when the model stops a turn.
type StopData struct{}

// Response is returned by a lifecycle hook consumer.
type Response struct {
	Permission   *Permission `json:"permission,omitempty"`
	ModelContext string      `json:"model_context,omitempty"`
	UserMessage  string      `json:"user_message,omitempty"`
	AllowedTools []string    `json:"allowed_tools,omitempty"`
	EndChat      bool        `json:"end_chat,omitempty"`
}

// Permission controls whether mutable hook input may proceed.
type Permission struct {
	Decision      PermissionDecision `json:"decision"`
	Reason        string             `json:"reason,omitempty"`
	InputOverride json.RawMessage    `json:"input_override,omitempty"`
}

// PermissionDecision is a hook consumer's decision for mutable input.
type PermissionDecision string

const (
	PermissionAllow PermissionDecision = "allow"
	PermissionDeny  PermissionDecision = "deny"
	PermissionAsk   PermissionDecision = "ask"
)

// Claims describes the JWT minted by coderd for a lifecycle hook dispatch.
type Claims struct {
	Issuer     string    `json:"iss"`
	Subject    string    `json:"sub"`
	Audience   string    `json:"aud"`
	IssuedAt   int64     `json:"iat"`
	NotBefore  int64     `json:"nbf"`
	Expires    int64     `json:"exp"`
	JTI        uuid.UUID `json:"jti"`
	Type       EventType `json:"type"`
	BodySHA256 string    `json:"body_sha256"`
}

// ChatID parses the chat UUID from the Subject claim.
func (c Claims) ChatID() (uuid.UUID, error) {
	value, ok := strings.CutPrefix(c.Subject, "coder:chat:")
	if !ok {
		return uuid.Nil, xerrors.Errorf("invalid subject %q", c.Subject)
	}
	chatID, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, xerrors.Errorf("parse chat ID: %w", err)
	}
	return chatID, nil
}
