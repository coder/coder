package chatd

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chathooks"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
)

const (
	sessionStartSourceStartup = "startup"
	sessionStartSourceResume  = "resume"
	sessionStartSourceClear   = "clear"
)

func sessionStartSource(messages []database.ChatMessage) string {
	for _, message := range messages {
		if message.Role == database.ChatMessageRoleAssistant {
			return sessionStartSourceResume
		}
	}
	return sessionStartSourceStartup
}

// hookTrigger is the only component that talks to the hook dispatcher.
// Every lifecycle event flows through trigger, which builds the wire
// envelope, dispatches, and normalizes the outcome.
type hookTrigger struct {
	dispatcher *chathooks.Dispatcher
}

func newHookTrigger(dispatcher *chathooks.Dispatcher) *hookTrigger {
	return &hookTrigger{dispatcher: dispatcher}
}

func (t *hookTrigger) enabled() bool {
	return t != nil && t.dispatcher.Enabled()
}

// hookChat identifies the chat and turn an event belongs to. Admission
// events for chats that do not exist yet (create, subagent spawn) fill
// the fields directly instead of loading a row.
type hookChat struct {
	ID           uuid.UUID
	OwnerID      uuid.UUID
	WorkspaceID  uuid.NullUUID
	ParentChatID uuid.NullUUID
	RootChatID   uuid.NullUUID
	TurnID       *uuid.UUID
}

func hookChatFor(chat database.Chat, turnID *uuid.UUID) hookChat {
	return hookChat{
		ID:           chat.ID,
		OwnerID:      chat.OwnerID,
		WorkspaceID:  chat.WorkspaceID,
		ParentChatID: chat.ParentChatID,
		RootChatID:   chat.RootChatID,
		TurnID:       turnID,
	}
}

func (c hookChat) ref() agenthooks.ChatRef {
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

type hookMessage struct {
	Source       string
	Prompt       string
	Parts        json.RawMessage
	ToolUseID    string
	ToolName     string
	ToolInput    json.RawMessage
	ToolResponse json.RawMessage
	ToolError    string
}

func userPromptHookMessage(parts []codersdk.ChatMessagePart) (hookMessage, error) {
	encoded, err := chatprompt.MarshalParts(parts)
	if err != nil {
		return hookMessage{}, xerrors.Errorf("marshal prompt parts for hook: %w", err)
	}
	return hookMessage{
		Prompt: textFromParts(parts),
		Parts:  encoded.RawMessage,
	}, nil
}

// hookResult is a consumer response normalized for callers: a non-empty
// InputOverride means the permission decision was allow with a
// replacement input (the wire contract rejects allow without one).
// Denials surface as *hookDeniedError instead.
type hookResult struct {
	InputOverride json.RawMessage
	ModelContext  string
	UserMessage   string
}

var emptyHookResult = &hookResult{}

func (r *hookResult) modelContext() string {
	if r == nil {
		return ""
	}
	return r.ModelContext
}

func (r *hookResult) userMessage() string {
	if r == nil {
		return ""
	}
	return r.UserMessage
}

func (t *hookTrigger) trigger(
	ctx context.Context,
	chat hookChat,
	msg hookMessage,
	event agenthooks.EventType,
) (*hookResult, error) {
	if !t.enabled() {
		return emptyHookResult, nil
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
	response, _, err := t.dispatcher.Dispatch(ctx, chathooks.Event{
		Type:    event,
		ChatRef: chat.ref(),
		Data:    data,
	})
	if err != nil {
		return nil, err
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		return nil, &hookDeniedError{
			Event:        event,
			Reason:       response.Permission.Reason,
			ModelContext: response.ModelContext,
			UserMessage:  response.UserMessage,
		}
	}
	result := &hookResult{
		ModelContext: response.ModelContext,
		UserMessage:  response.UserMessage,
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionAllow {
		result.InputOverride = response.Permission.InputOverride
	}
	return result, nil
}
