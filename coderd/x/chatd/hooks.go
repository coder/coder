package chatd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprompt"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
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

func (p *Server) handleUserPromptDispatchError(ctx context.Context, chatID uuid.UUID, dispatchErr error) error {
	var structured *chathooks.DispatchError
	if !errors.As(dispatchErr, &structured) {
		return dispatchErr
	}
	lastError := fmt.Sprintf(
		"hook dispatch failed: %s: %s (dispatch %s)",
		agenthooks.EventUserPromptSubmit,
		structured.Class,
		structured.DispatchID,
	)
	var failedChat database.Chat
	machine := p.newChatMachine(chatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		if _, err := tx.FailIdle(chatstate.FailIdleInput{LastError: lastError}); err != nil {
			return err
		}
		chat, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("reload chat after hook failure: %w", err)
		}
		failedChat = chat
		return nil
	})
	if errors.Is(err, chatstate.ErrTransitionNotAllowed) {
		return dispatchErr
	}
	if err != nil {
		return errors.Join(dispatchErr, xerrors.Errorf("fail idle chat after hook dispatch: %w", err))
	}
	p.publishChatPubsubEvent(failedChat, codersdk.ChatWatchEventKindStatusChange, nil)
	return dispatchErr
}

func hookPrefixMessages(response agenthooks.Response, modelConfigID, turnID uuid.UUID) ([]chatstate.Message, error) {
	messages := make([]chatstate.Message, 0, 2)
	if response.ModelContext != "" {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(response.ModelContext)})
		if err != nil {
			return nil, xerrors.Errorf("marshal hook model context: %w", err)
		}
		messages = append(messages, chatstate.Message{
			Role:           database.ChatMessageRoleUser,
			Content:        content,
			Visibility:     database.ChatMessageVisibilityModel,
			TurnID:         uuid.NullUUID{UUID: turnID, Valid: true},
			ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
			ContentVersion: chatprompt.CurrentContentVersion,
		})
	}
	if response.UserMessage != "" {
		content, err := chatprompt.MarshalParts([]codersdk.ChatMessagePart{codersdk.ChatMessageText(response.UserMessage)})
		if err != nil {
			return nil, xerrors.Errorf("marshal hook user message: %w", err)
		}
		messages = append(messages, chatstate.Message{
			Role:           database.ChatMessageRoleSystem,
			Content:        content,
			Visibility:     database.ChatMessageVisibilityUser,
			TurnID:         uuid.NullUUID{UUID: turnID, Valid: true},
			ModelConfigID:  uuid.NullUUID{UUID: modelConfigID, Valid: modelConfigID != uuid.Nil},
			ContentVersion: chatprompt.CurrentContentVersion,
		})
	}
	return messages, nil
}

func applyHookAllowedTools(ctx context.Context, store database.Store, chatID uuid.UUID, response agenthooks.Response) error {
	if response.AllowedTools == nil {
		return nil
	}
	encoded, err := json.Marshal(response.AllowedTools)
	if err != nil {
		return xerrors.Errorf("marshal hook allowed tools: %w", err)
	}
	if err := store.UpdateChatHookAllowedTools(ctx, database.UpdateChatHookAllowedToolsParams{
		HookAllowedTools: pqtype.NullRawMessage{RawMessage: encoded, Valid: true},
		ID:               chatID,
	}); err != nil {
		return xerrors.Errorf("update hook allowed tools: %w", err)
	}
	return nil
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
