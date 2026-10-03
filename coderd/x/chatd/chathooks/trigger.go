// Package chathooks integrates chat lifecycle hooks into chatd: it
// builds event envelopes, dispatches them, and converts consumer
// responses into transcript effects and permission decisions.
package chathooks

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

const (
	SessionStartSourceStartup = "startup"
	SessionStartSourceResume  = "resume"
	SessionStartSourceClear   = "clear"
)

func SessionStartSource(messages []database.ChatMessage) string {
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleAssistant {
			return SessionStartSourceResume
		}
	}
	return SessionStartSourceStartup
}

// Trigger is the only component that talks to the hook dispatcher.
// Every lifecycle event flows through trigger, which builds the wire
// envelope, dispatches, and normalizes the outcome.
type Trigger struct {
	dispatcher *dispatch.Dispatcher
}

func NewTrigger(dispatcher *dispatch.Dispatcher) *Trigger {
	return &Trigger{dispatcher: dispatcher}
}

func (t *Trigger) Enabled() bool {
	return t != nil && t.dispatcher.Enabled()
}

// Chat identifies the chat and turn an event belongs to. Admission
// events for chats that do not exist yet (create, subagent spawn) fill
// the fields directly instead of loading a row.
type Chat struct {
	ID           uuid.UUID
	OwnerID      uuid.UUID
	WorkspaceID  uuid.NullUUID
	ParentChatID uuid.NullUUID
	RootChatID   uuid.NullUUID
	TurnID       *uuid.UUID
}

func ChatFor(chat database.Chat, turnID *uuid.UUID) Chat {
	return Chat{
		ID:           chat.ID,
		OwnerID:      chat.OwnerID,
		WorkspaceID:  chat.WorkspaceID,
		ParentChatID: chat.ParentChatID,
		RootChatID:   chat.RootChatID,
		TurnID:       turnID,
	}
}

func (c Chat) ref() agenthooks.ChatRef {
	ref := agenthooks.ChatRef{
		ChatID:  c.ID,
		OwnerID: c.OwnerID,
		TurnID:  c.TurnID,
	}
	if c.WorkspaceID.Valid {
		ref.WorkspaceID = &c.WorkspaceID.UUID
	}
	if c.ParentChatID.Valid {
		ref.ParentChatID = &c.ParentChatID.UUID
	}
	if c.RootChatID.Valid {
		ref.RootChatID = &c.RootChatID.UUID
	}
	return ref
}

type Message struct {
	Source       string
	Prompt       string
	Parts        json.RawMessage
	ToolUseID    string
	ToolName     string
	ToolInput    json.RawMessage
	ToolResponse json.RawMessage
	ToolError    string
}

func UserPromptMessage(parts []codersdk.ChatMessagePart) (Message, error) {
	encoded, err := chatprompt.MarshalParts(parts)
	if err != nil {
		return Message{}, xerrors.Errorf("marshal prompt parts for hook: %w", err)
	}
	return Message{
		Prompt: textFromParts(parts),
		Parts:  encoded.RawMessage,
	}, nil
}

// Result is a consumer response normalized for callers: a non-empty
// InputOverride means the permission decision was allow with a
// replacement input (the wire contract rejects allow without one).
// Denials surface as *deniedError instead.
type Result struct {
	InputOverride json.RawMessage
	ModelContext  string
	UserMessage   string
}

var emptyResult = &Result{}

func (r *Result) GetModelContext() string {
	if r == nil {
		return ""
	}
	return r.ModelContext
}

func (r *Result) GetUserMessage() string {
	if r == nil {
		return ""
	}
	return r.UserMessage
}

func (t *Trigger) Trigger(
	ctx context.Context,
	chat Chat,
	msg Message,
	event agenthooks.EventType,
	capacity dispatch.CapacityClass,
) (*Result, error) {
	if !t.Enabled() {
		return emptyResult, nil
	}
	var data any
	switch event {
	case agenthooks.EventSessionStart:
		data = agenthooks.SessionStartData{Source: msg.Source}
	case agenthooks.EventUserPromptSubmit:
		data = agenthooks.UserPromptSubmitData{Prompt: msg.Prompt, Parts: msg.Parts}
	case agenthooks.EventPreToolUse:
		data = agenthooks.PreToolUseData{ToolUseID: msg.ToolUseID, ToolName: msg.ToolName, ToolInput: msg.ToolInput}
	case agenthooks.EventPostToolUse:
		data = agenthooks.PostToolUseData{ToolUseID: msg.ToolUseID, ToolName: msg.ToolName, ToolResponse: msg.ToolResponse, ToolError: msg.ToolError}
	case agenthooks.EventPreCompact:
		data = agenthooks.PreCompactData{}
	case agenthooks.EventPostCompact:
		data = agenthooks.PostCompactData{}
	case agenthooks.EventStop:
		data = agenthooks.StopData{}
	default:
		return nil, xerrors.Errorf("unsupported hook event %q", event)
	}
	response, _, err := t.dispatcher.Dispatch(ctx, dispatch.Event{
		Type:     event,
		ChatRef:  chat.ref(),
		Data:     data,
		Capacity: capacity,
	})
	if err != nil {
		return nil, err
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		return nil, &deniedError{
			Event:        event,
			Reason:       response.Permission.Reason,
			ModelContext: response.ModelContext,
			UserMessage:  response.UserMessage,
		}
	}
	result := &Result{
		ModelContext: response.ModelContext,
		UserMessage:  response.UserMessage,
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionAllow {
		result.InputOverride = response.Permission.InputOverride
	}
	return result, nil
}

func textFromParts(parts []codersdk.ChatMessagePart) string {
	var builder strings.Builder
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			_, _ = builder.WriteString(part.Text)
		}
	}
	return builder.String()
}
