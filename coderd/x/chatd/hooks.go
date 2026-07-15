package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
)

// UserPromptDeniedError reports a consumer denial without persisting the prompt.
type UserPromptDeniedError struct {
	UserMessage string
}

func (*UserPromptDeniedError) Error() string {
	return "user prompt denied by lifecycle hook"
}

func promptText(parts []codersdk.ChatMessagePart) string {
	var prompt strings.Builder
	for _, part := range parts {
		if part.Type == codersdk.ChatMessagePartTypeText {
			_, _ = prompt.WriteString(part.Text)
		}
	}
	return prompt.String()
}

func (p *Server) dispatchUserPromptSubmit(
	ctx context.Context,
	chat database.Chat,
	turnID uuid.UUID,
	parts []codersdk.ChatMessagePart,
) (agenthooks.Response, error) {
	if p.hookDispatcher == nil || !p.hookDispatcher.Enabled() {
		return agenthooks.Response{}, nil
	}

	var workspaceID *uuid.UUID
	if chat.WorkspaceID.Valid {
		workspaceID = &chat.WorkspaceID.UUID
	}
	response, err := p.hookDispatcher.Dispatch(ctx, chathooks.Event{
		Type:        agenthooks.EventUserPromptSubmit,
		ChatID:      chat.ID,
		OwnerID:     chat.OwnerID,
		WorkspaceID: workspaceID,
		TurnID:      &turnID,
		ToolUseID:   nil,
		Data: agenthooks.UserPromptSubmitData{
			Prompt: promptText(parts),
		},
	})
	if err != nil {
		return agenthooks.Response{}, err
	}
	if response.Permission != nil && response.Permission.Decision == agenthooks.PermissionDeny {
		return agenthooks.Response{}, &UserPromptDeniedError{UserMessage: response.UserMessage}
	}
	return response, nil
}

func userPromptOverride(response agenthooks.Response) (string, bool, error) {
	if response.Permission == nil || response.Permission.Decision != agenthooks.PermissionAllow {
		return "", false, nil
	}
	var override struct {
		Prompt *string `json:"prompt"`
	}
	decoder := json.NewDecoder(bytes.NewReader(response.Permission.InputOverride))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&override); err != nil {
		return "", false, xerrors.Errorf("decode user prompt input override: %w", err)
	}
	if override.Prompt == nil {
		return "", false, xerrors.New("decode user prompt input override: prompt is required")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", false, xerrors.New("decode user prompt input override: trailing JSON value")
	}
	return *override.Prompt, true, nil
}
