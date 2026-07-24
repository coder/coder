package chatstate

import (
	"errors"
	"fmt"

	"golang.org/x/xerrors"
)

// Sentinel errors returned by chatstate transitions and helpers.
// Callers should use errors.Is to test for these.
var (
	// ErrTransitionNotAllowed is returned when a transition is applied
	// to a chat whose current execution state does not permit it. The
	// concrete error returned by transition methods is a
	// *TransitionError that wraps this sentinel.
	ErrTransitionNotAllowed = xerrors.New("chat state transition not allowed")

	// ErrInvalidState is returned when the chat row, queue, and
	// archive flag together produce a combination outside the chat
	// execution state model.
	ErrInvalidState = xerrors.New("chat is in an invalid execution state")

	// ErrQueuedMessageNotFound is returned by queue-targeting
	// transitions (delete, promote) when the supplied queued message
	// ID does not match a row on the chat.
	ErrQueuedMessageNotFound = xerrors.New("queued message not found")

	// ErrMessageNotFound is returned by [Tx.EditMessage] when the
	// target chat_messages row is missing or belongs to another chat.
	ErrMessageNotFound = xerrors.New("chat message not found")

	// ErrChatNotFound is returned when a non-create transition is
	// applied to a chat row that does not exist (or has been deleted
	// since the transition started).
	ErrChatNotFound = xerrors.New("chat not found")

	// ErrChatNotRoot is returned by family-archive helpers when the
	// supplied chat is not a root chat (its parent_chat_id is set).
	ErrChatNotRoot = xerrors.New("chat is not a root chat")

	// ErrEditedMessageNotUser is returned by [Tx.EditMessage] when the
	// targeted chat_messages row exists but its role is not user.
	ErrEditedMessageNotUser = xerrors.New("only user messages can be edited")

	// ErrMessageQueueFull is returned by queue-appending transitions
	// when the per-chat queue cap has been reached. The concrete
	// error returned by transitions is a *MessageQueueFullError that
	// wraps this sentinel.
	ErrMessageQueueFull = xerrors.New("chat message queue is full")

	// ErrToolResultDuplicate is returned by [Tx.CompleteRequiresAction]
	// when the same tool_call_id appears more than once in the
	// submitted results.
	ErrToolResultDuplicate = xerrors.New("duplicate tool result")

	// ErrToolResultUnexpected is returned by
	// [Tx.CompleteRequiresAction] when a submitted tool_call_id does
	// not correspond to a pending dynamic tool call.
	ErrToolResultUnexpected = xerrors.New("unexpected tool result")

	// ErrToolResultMissing is returned by [Tx.CompleteRequiresAction]
	// when a pending dynamic tool call has no submitted result.
	ErrToolResultMissing = xerrors.New("missing tool result")

	// ErrToolResultInvalidJSON is returned by
	// [Tx.CompleteRequiresAction] when a submitted tool result output
	// is not valid JSON.
	ErrToolResultInvalidJSON = xerrors.New("tool result output is not valid JSON")
)

// MessageQueueFullError carries the per-chat queue cap so HTTP
// endpoints can include the cap in their response detail. It wraps
// [ErrMessageQueueFull] so callers can match it with errors.Is.
type MessageQueueFullError struct {
	Max int64
}

// Error implements the error interface.
func (e *MessageQueueFullError) Error() string {
	return fmt.Sprintf("chat message queue is full (max %d)", e.Max)
}

// Unwrap returns [ErrMessageQueueFull] so callers can match the
// generic sentinel.
func (*MessageQueueFullError) Unwrap() error { return ErrMessageQueueFull }

// ToolResultValidationError carries a structured tool-result
// validation failure. It always wraps a specific sentinel
// (ErrToolResultDuplicate, ErrToolResultMissing,
// ErrToolResultUnexpected, ErrToolResultInvalidJSON) so callers can
// match either the generic sentinel or the specific cause.
type ToolResultValidationError struct {
	Cause      error
	ToolCallID string
}

// Error implements the error interface.
func (e *ToolResultValidationError) Error() string {
	if e.ToolCallID != "" {
		return fmt.Sprintf("%s: %s", e.Cause.Error(), e.ToolCallID)
	}
	return e.Cause.Error()
}

// Unwrap returns the specific cause so callers can match it.
func (e *ToolResultValidationError) Unwrap() error { return e.Cause }

// TransitionError carries the structured detail for a rejected
// transition. It always wraps [ErrTransitionNotAllowed] so callers can
// match with errors.Is without losing context. When a specific
// chatstate sentinel is the proximate cause, Cause is set and
// errors.Is will match that sentinel too.
type TransitionError struct {
	Transition Transition
	From       ExecutionState
	Reason     string
	Cause      error
}

// Error implements the error interface.
func (e *TransitionError) Error() string {
	if e.Reason == "" {
		return fmt.Sprintf(
			"chat state transition %s not allowed from state %s",
			e.Transition, e.From,
		)
	}
	return fmt.Sprintf(
		"chat state transition %s not allowed from state %s: %s",
		e.Transition, e.From, e.Reason,
	)
}

// Unwrap returns the error chain attached to this error. The chain
// always includes [ErrTransitionNotAllowed], and may include a more
// specific cause through errors.Join, so callers can use errors.Is
// without custom matching logic on TransitionError.
func (e *TransitionError) Unwrap() error { return e.Cause }

// newTransitionError constructs a typed TransitionError. Returning the
// pointer type lets callers inspect the structured fields when needed.
func newTransitionError(t Transition, from ExecutionState, reason string) *TransitionError {
	return &TransitionError{Transition: t, From: from, Reason: reason, Cause: ErrTransitionNotAllowed}
}

// newTransitionErrorWithCause constructs a TransitionError carrying
// a specific underlying sentinel so callers can match the cause with
// errors.Is.
func newTransitionErrorWithCause(t Transition, from ExecutionState, cause error, reason string) *TransitionError {
	return &TransitionError{Transition: t, From: from, Reason: reason, Cause: errors.Join(ErrTransitionNotAllowed, cause)}
}
