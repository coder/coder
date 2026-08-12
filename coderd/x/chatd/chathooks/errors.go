package chathooks

import (
	"errors"
	"fmt"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/x/agenthooks/dispatch"
	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/x/agenthooks"
)

// deniedError is trigger's normalized form of a permission deny.
// Callers translate it per event: user_prompt_submit sites map it to
// UserPromptDeniedError, pre_tool_use sites fold it into a synthetic
// tool result.
type deniedError struct {
	Event        agenthooks.EventType
	Reason       string
	ModelContext string
	UserMessage  string
}

func (e *deniedError) Error() string {
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
// string, such as subagent tool responses, still expose the user-facing
// denial message. The HTTP handlers unwrap the typed error instead.
func (e *UserPromptDeniedError) Error() string {
	if e.UserMessage == "" {
		return "user prompt denied by lifecycle hook"
	}
	return "user prompt denied by lifecycle hook: " + e.UserMessage
}

func UserPromptDenial(err error) error {
	if denied, ok := errors.AsType[*deniedError](err); ok {
		return &UserPromptDeniedError{UserMessage: denied.UserMessage}
	}
	return err
}

func DispatchErrorMessage(eventType agenthooks.EventType, dispatchErr error) (string, bool) {
	structured, ok := errors.AsType[*dispatch.Error](dispatchErr)
	if !ok {
		return "", false
	}
	return fmt.Sprintf(
		"hook dispatch failed: %s: %s (dispatch %s)",
		eventType,
		structured.Class,
		structured.DispatchID,
	), true
}

func GenerationDispatchError(eventType agenthooks.EventType, dispatchErr error) error {
	message, ok := DispatchErrorMessage(eventType, dispatchErr)
	if !ok {
		message = dispatchErr.Error()
	}
	return chaterror.WithClassification(dispatchErr, chaterror.ClassifiedError{
		Message: message,
		Kind:    codersdk.ChatErrorKindHookDispatchFailed,
	})
}

// DispatchFailureFromResults returns the first tool result error
// whose chain contains a hook dispatch failure. Tools that dispatch
// lifecycle hooks inside Run (subagent spawn admission) must fail
// closed, but the tool loop persists Run errors as ordinary tool
// results the model can ignore, so the turn has to be failed even
// though the step commits.
func DispatchFailureFromResults(content []fantasy.Content) error {
	for _, block := range content {
		toolResult, ok := asToolResultContent(block)
		if !ok {
			continue
		}
		if resultErr := dispatchFailureFromResult(toolResult); resultErr != nil {
			return resultErr
		}
	}
	return nil
}

func dispatchFailureFromResult(toolResult fantasy.ToolResultContent) error {
	var resultErr error
	switch output := toolResult.Result.(type) {
	case fantasy.ToolResultOutputContentError:
		resultErr = output.Error
	case *fantasy.ToolResultOutputContentError:
		if output != nil {
			resultErr = output.Error
		}
	}
	if resultErr == nil {
		return nil
	}
	if _, ok := errors.AsType[*dispatch.Error](resultErr); !ok {
		return nil
	}
	return resultErr
}

func asToolResultContent(block fantasy.Content) (fantasy.ToolResultContent, bool) {
	if tr, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
		return tr, true
	}
	if tr, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && tr != nil {
		return *tr, true
	}
	return fantasy.ToolResultContent{}, false
}
