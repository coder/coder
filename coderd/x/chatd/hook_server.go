package chatd

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chathooks"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

// applyHookResultMessages inserts hook event rows before the step's
// own rows so injected model context precedes the assistant content it
// steers; providers require tool results to directly follow the
// assistant tool calls.
func applyHookResultMessages(
	messages stepMessagesForCommit,
	results []*chathooks.Result,
	modelConfigID uuid.UUID,
) (stepMessagesForCommit, error) {
	return insertHookResultMessages(messages, results, modelConfigID, hookRowsBeforeStep)
}

func appendHookResultMessages(
	messages stepMessagesForCommit,
	results []*chathooks.Result,
	modelConfigID uuid.UUID,
) (stepMessagesForCommit, error) {
	return insertHookResultMessages(messages, results, modelConfigID, hookRowsAfterStep)
}

type hookRowPlacement int

const (
	hookRowsBeforeStep hookRowPlacement = iota
	hookRowsAfterStep
)

func insertHookResultMessages(
	messages stepMessagesForCommit,
	results []*chathooks.Result,
	modelConfigID uuid.UUID,
	placement hookRowPlacement,
) (stepMessagesForCommit, error) {
	rows, err := chathooks.EventMessagesForResults(results, modelConfigID)
	if err != nil {
		return stepMessagesForCommit{}, err
	}
	if len(rows) > 0 {
		if placement == hookRowsBeforeStep {
			messages.Messages = append(rows, messages.Messages...)
		} else {
			messages.Messages = append(messages.Messages, rows...)
		}
		messages.VisibleIndexes = visibleMessageIndexes(messages.Messages)
	}
	return messages, nil
}

func (p *Server) handleUserPromptDispatchError(ctx context.Context, chatID uuid.UUID, dispatchErr error) error {
	return p.handleAPIDispatchError(ctx, chatID, agenthooks.EventUserPromptSubmit, dispatchErr)
}

func (p *Server) handleAPIDispatchError(ctx context.Context, chatID uuid.UUID, eventType agenthooks.EventType, dispatchErr error) error {
	lastError, ok := chathooks.DispatchErrorMessage(eventType, dispatchErr)
	if !ok {
		return dispatchErr
	}
	encoded, marshalErr := json.Marshal(codersdk.ChatError{
		Message: lastError,
		Kind:    codersdk.ChatErrorKindHookDispatchFailed,
	})
	if marshalErr != nil {
		return errors.Join(dispatchErr, xerrors.Errorf("encode hook dispatch error: %w", marshalErr))
	}
	var failedChat database.Chat
	machine := p.newChatMachine(chatID)
	err := machine.Update(ctx, func(tx *chatstate.Tx, store database.Store) error {
		current, err := store.GetChatByID(ctx, chatID)
		if err != nil {
			return xerrors.Errorf("load chat for hook failure: %w", err)
		}
		// Park only idle chats. FinishError is also allowed from running
		// states, but a running chat keeps its active turn and the
		// request error alone surfaces to the caller.
		if current.Status != database.ChatStatusWaiting {
			return chatstate.ErrTransitionNotAllowed
		}
		if _, err := tx.FinishError(chatstate.FinishErrorInput{
			LastError: pqtype.NullRawMessage{RawMessage: encoded, Valid: true},
		}); err != nil {
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

type dynamicPostToolUseState struct {
	chat          database.Chat
	modelConfigID uuid.UUID
	toolNames     map[string]string
}

func loadDynamicPostToolUseState(
	ctx context.Context,
	machine *chatstate.ChatMachine,
	opts SubmitToolResultsOptions,
) (dynamicPostToolUseState, error) {
	var state dynamicPostToolUseState
	err := machine.ReadLock(ctx, func(store database.Store) error {
		chat, err := store.GetChatByID(ctx, opts.ChatID)
		if err != nil {
			return xerrors.Errorf("load chat: %w", err)
		}
		if chat.Archived {
			return ErrChatArchived
		}
		if chat.Status != database.ChatStatusRequiresAction {
			return &ToolResultStatusConflictError{ActualStatus: chat.Status}
		}
		messages, err := store.GetChatMessagesByChatID(ctx, database.GetChatMessagesByChatIDParams{
			ChatID:  opts.ChatID,
			AfterID: 0,
		})
		if err != nil {
			return xerrors.Errorf("load chat messages: %w", err)
		}
		_, pending, err := unresolvedToolCallsFromHistory(messages, dynamicToolNamesFromChat(chat))
		if err != nil {
			return xerrors.Errorf("load pending dynamic tool calls: %w", err)
		}
		toolNames := make(map[string]string, len(pending))
		for _, call := range pending {
			toolNames[call.ToolCallID] = call.ToolName
		}
		if err := validateSubmittedToolResults(opts.Results, toolNames); err != nil {
			return err
		}
		modelConfigID := opts.ModelConfigID
		if modelConfigID == uuid.Nil {
			modelConfigID = chat.LastModelConfigID
		}
		state = dynamicPostToolUseState{
			chat:          chat,
			modelConfigID: modelConfigID,
			toolNames:     toolNames,
		}
		return nil
	})
	return state, err
}

// validateSubmittedToolResults rejects invalid results before hook dispatch,
// using the same rules as CompleteRequiresAction.
func validateSubmittedToolResults(results []codersdk.ToolResult, toolNames map[string]string) error {
	inputs := make([]chatstate.ToolResultInput, 0, len(results))
	for _, result := range results {
		inputs = append(inputs, chatstate.ToolResultInput{
			ToolCallID: result.ToolCallID,
			Output:     result.Output,
		})
	}
	if invalid := chatstate.ValidateToolResults(inputs, toolNames); invalid != nil {
		return translateToolResultValidationError(invalid)
	}
	return nil
}
