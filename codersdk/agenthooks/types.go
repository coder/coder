// Package agenthooks defines the experimental wire protocol for Coder agent
// lifecycle hooks. It requires the agent-lifecycle-hooks experiment and has no
// backward-compatibility guarantee, including for SchemaVersion 1.
package agenthooks

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// SchemaVersion is the current lifecycle hook request schema version.
const SchemaVersion = 1

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

type Meta struct {
	DispatchID    uuid.UUID  `json:"dispatch_id"`
	SchemaVersion int        `json:"schema_version"`
	ChatID        uuid.UUID  `json:"chat_id"`
	OwnerID       uuid.UUID  `json:"owner_id"`
	WorkspaceID   *uuid.UUID `json:"workspace_id,omitempty"`
	TurnID        *uuid.UUID `json:"turn_id,omitempty"`
	ParentChatID  *uuid.UUID `json:"parent_chat_id,omitempty"`
	// RootChatID groups a subagent subtree with its user-facing conversation.
	// Unset for top-level chats.
	RootChatID *uuid.UUID `json:"root_chat_id,omitempty"`
}

type SessionStartData struct {
	Source string `json:"source"`
}

// UserPromptSubmitData includes concatenated text and persisted parts.
// Inspect Parts when structure matters.
type UserPromptSubmitData struct {
	Prompt string          `json:"prompt"`
	Parts  json.RawMessage `json:"parts,omitempty"`
}

type PreToolUseData struct {
	ToolUseID string          `json:"tool_use_id"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

type PostToolUseData struct {
	ToolUseID    string          `json:"tool_use_id"`
	ToolName     string          `json:"tool_name"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
	ToolError    string          `json:"tool_error,omitempty"`
}

type PreCompactData struct{}

type PostCompactData struct{}

type StopData struct{}

type Response struct {
	Permission   *Permission `json:"permission,omitempty"`
	ModelContext string      `json:"model_context,omitempty"`
	UserMessage  string      `json:"user_message,omitempty"`
	// AllowedTools distinguishes unchanged (nil), no tools (empty), and named tools.
	AllowedTools *[]string `json:"allowed_tools,omitempty"`
	EndChat      bool      `json:"end_chat,omitempty"`
}

// Permission controls whether mutable hook input may proceed.
type Permission struct {
	Decision      PermissionDecision `json:"decision"`
	Reason        string             `json:"reason,omitempty"`
	InputOverride json.RawMessage    `json:"input_override,omitempty"`
}

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
