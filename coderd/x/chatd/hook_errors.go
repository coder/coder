package chatd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"charm.land/fantasy"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chatstate"
	"github.com/coder/coder/v2/coderd/x/chathooks"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agenthooks"
)

// hookDeniedError is trigger's normalized form of a permission deny.
// Callers translate it per event: user_prompt_submit sites map it to
// UserPromptDeniedError, pre_tool_use sites fold it into a synthetic
// tool result.
type hookDeniedError struct {
	Event        agenthooks.EventType
	Reason       string
	ModelContext string
	UserMessage  string
}

func (e *hookDeniedError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf("%s denied by lifecycle hook", e.Event)
	}
	return fmt.Sprintf("%s denied by lifecycle hook: %s", e.Event, e.Reason)
}

// UserPromptDeniedError reports that a lifecycle hook rejected a prompt.
type UserPromptDeniedError struct {
	UserMessage string
}

// Error includes UserMessage so callers that only surface the error
// string, such as subagent tool responses, still expose the hook's
// reason. The HTTP handlers unwrap the typed error instead.
func (e *UserPromptDeniedError) Error() string {
	if e.UserMessage == "" {
		return "user prompt denied by lifecycle hook"
	}
	return "user prompt denied by lifecycle hook: " + e.UserMessage
}

func userPromptDenial(err error) error {
	var denied *hookDeniedError
	if errors.As(err, &denied) {
		return &UserPromptDeniedError{UserMessage: denied.UserMessage}
	}
	return err
}

func (p *Server) handleUserPromptDispatchError(ctx context.Context, chatID uuid.UUID, dispatchErr error) error {
	return p.handleAPIDispatchError(ctx, chatID, agenthooks.EventUserPromptSubmit, dispatchErr)
}

func (p *Server) handleAPIDispatchError(ctx context.Context, chatID uuid.UUID, eventType agenthooks.EventType, dispatchErr error) error {
	lastError, ok := hookDispatchErrorMessage(eventType, dispatchErr)
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

func hookDispatchErrorMessage(eventType agenthooks.EventType, dispatchErr error) (string, bool) {
	var structured *chathooks.DispatchError
	if !errors.As(dispatchErr, &structured) {
		return "", false
	}
	return fmt.Sprintf(
		"hook dispatch failed: %s: %s (dispatch %s)",
		eventType,
		structured.Class,
		structured.DispatchID,
	), true
}

func generationHookDispatchError(eventType agenthooks.EventType, dispatchErr error) error {
	message, ok := hookDispatchErrorMessage(eventType, dispatchErr)
	if !ok {
		message = dispatchErr.Error()
	}
	return chaterror.WithClassification(dispatchErr, chaterror.ClassifiedError{
		Message: message,
		Kind:    codersdk.ChatErrorKindHookDispatchFailed,
	})
}

// hookDispatchFailureFromResults returns the first tool result error
// whose chain contains a hook dispatch failure. Tools that dispatch
// lifecycle hooks inside Run (subagent spawn admission) must fail
// closed, but the tool loop persists Run errors as ordinary tool
// results the model can ignore, so the step has to be failed before
// commit instead.
func hookDispatchFailureFromResults(content []fantasy.Content) error {
	for _, block := range content {
		toolResult, ok := asToolResultContent(block)
		if !ok {
			continue
		}
		var resultErr error
		switch output := toolResult.Result.(type) {
		case fantasy.ToolResultOutputContentError:
			resultErr = output.Error
		case *fantasy.ToolResultOutputContentError:
			if output != nil {
				resultErr = output.Error
			}
		}
		var dispatchErr *chathooks.DispatchError
		if resultErr != nil && errors.As(resultErr, &dispatchErr) {
			return resultErr
		}
	}
	return nil
}
