// Package agenthooks defines the experimental wire protocol for Coder agent
// lifecycle hooks. The protocol, including SchemaVersion 1, has no
// backward-compatibility guarantee.
//
// Coder persists no hook dispatch state, so delivery is best-effort and may
// duplicate. A failed dispatch is never queued for redelivery; hooks are fail
// closed, so the operation that raised the event fails instead. Consumers must
// therefore tolerate duplicates without assuming every event arrives.
//
// A retried HTTP attempt reuses its Meta.DispatchID, so consumers deduplicate
// transport retries by that ID. A repeated logical event gets a new ID, so
// keep side effects keyed on the event's own identifiers, such as tool_use_id,
// or make them safe to repeat.
package agenthooks

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

// SchemaVersion is the current lifecycle hook request schema version.
const SchemaVersion = 1

// EventType names a lifecycle event carried by a hook request.
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

// Meta identifies a hook dispatch and its chat.
type Meta struct {
	DispatchID    uuid.UUID `json:"dispatch_id"`
	SchemaVersion int       `json:"schema_version"`
	ChatRef
}

// ChatRef identifies the chat a lifecycle hook event refers to.
type ChatRef struct {
	ChatID       uuid.UUID  `json:"chat_id"`
	OwnerID      uuid.UUID  `json:"owner_id"`
	WorkspaceID  *uuid.UUID `json:"workspace_id,omitempty"`
	TurnID       *uuid.UUID `json:"turn_id,omitempty"`
	ParentChatID *uuid.UUID `json:"parent_chat_id,omitempty"`
	// RootChatID identifies the user-facing root of the chat tree.
	RootChatID *uuid.UUID `json:"root_chat_id,omitempty"`
}

// SessionStartData reports why a chat session started. Source is
// "startup", "resume", or "clear".
type SessionStartData struct {
	Source string `json:"source"`
}

// UserPromptSubmitData includes concatenated text and persisted parts.
// Inspect Parts when structure matters. GoalObjective carries the chat
// goal objective admitted with the prompt when the submission also sets
// a goal; it feeds every subsequent generation's instructions, so
// prompt policy must observe it even when it differs from the message.
type UserPromptSubmitData struct {
	Prompt        string          `json:"prompt"`
	Parts         json.RawMessage `json:"parts,omitempty"`
	GoalObjective string          `json:"goal_objective,omitempty"`
}

// PreToolUseData describes a tool call before execution.
type PreToolUseData struct {
	ToolUseID string          `json:"tool_use_id"`
	ToolName  string          `json:"tool_name"`
	ToolInput json.RawMessage `json:"tool_input"`
}

// PostToolUseData describes a completed tool call, carrying either
// ToolResponse or ToolError.
type PostToolUseData struct {
	ToolUseID    string          `json:"tool_use_id"`
	ToolName     string          `json:"tool_name"`
	ToolResponse json.RawMessage `json:"tool_response,omitempty"`
	ToolError    string          `json:"tool_error,omitempty"`
}

// PreCompactData is empty; Meta identifies the chat being compacted.
type PreCompactData struct{}

// PostCompactData is empty; Meta identifies the compacted chat.
type PostCompactData struct{}

// StopData is empty; Meta identifies the chat that stopped.
type StopData struct{}

// Response carries a consumer's decision and optional injected content.
// Permission is honored for user_prompt_submit and pre_tool_use only.
// user_prompt_submit folds injected content into the submitted message.
// A denied pre_tool_use yields a synthetic tool result carrying only the
// policy text and any Reason; ModelContext persists separately as
// model-only transcript content that never reaches clients.
type Response struct {
	Permission   *Permission `json:"permission,omitempty"`
	ModelContext string      `json:"model_context,omitempty"`
	UserMessage  string      `json:"user_message,omitempty"`
}

// Permission controls whether mutable hook input may proceed.
type Permission struct {
	Decision      PermissionDecision `json:"decision"`
	Reason        string             `json:"reason,omitempty"`
	InputOverride json.RawMessage    `json:"input_override,omitempty"`
}

// PermissionDecision is a consumer's verdict on mutable hook input.
type PermissionDecision string

const (
	PermissionAllow PermissionDecision = "allow"
	PermissionDeny  PermissionDecision = "deny"
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

// ChatID returns the chat ID encoded in the "coder:chat:<id>" subject.
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
