package chatstate

import (
	"github.com/coder/coder/v2/coderd/database"
)

// ExecutionState identifies a chat's current execution state. Values
// outside the chat execution state model are represented by
// [StateInvalid].
type ExecutionState string

const (
	// StateN: chat does not exist.
	StateN ExecutionState = "N"
	// StateW: waiting, empty queue, not archived.
	StateW ExecutionState = "W"
	// StateE0: error, empty queue, not archived.
	StateE0 ExecutionState = "E0"
	// StateE1: error, non-empty queue, not archived.
	StateE1 ExecutionState = "E1"
	// StateR0: running, empty queue, not archived.
	StateR0 ExecutionState = "R0"
	// StateR1: running, non-empty queue, not archived.
	StateR1 ExecutionState = "R1"
	// StateI0: interrupting, empty queue, not archived.
	StateI0 ExecutionState = "I0"
	// StateI1: interrupting, non-empty queue, not archived.
	StateI1 ExecutionState = "I1"
	// StateA0: requires_action, empty queue, not archived.
	StateA0 ExecutionState = "A0"
	// StateA1: requires_action, non-empty queue, not archived.
	StateA1 ExecutionState = "A1"
	// StateXW: archived waiting, empty queue.
	StateXW ExecutionState = "XW"
	// StateXE0: archived error, empty queue.
	StateXE0 ExecutionState = "XE0"
	// StateXE1: archived error, non-empty queue.
	StateXE1 ExecutionState = "XE1"

	// StateInvalid groups every status/archive/queue combination that
	// is not one of the valid states above. The state machine refuses
	// non-reconciliation transitions on invalid states and exposes the
	// [Tx.ReconcileInvalidState] transition to recover.
	StateInvalid ExecutionState = "Invalid"
)

// String implements fmt.Stringer.
func (s ExecutionState) String() string { return string(s) }

// IsRunnable returns true for the execution states that the chat
// worker is allowed to acquire and drive forward: R0, R1, I0, I1,
// A0, and A1. Requires-action states need worker ownership for
// timeout processing. Other states are idle (W, E*, XW, XE*), absent
// (N), or invalid.
func (s ExecutionState) IsRunnable() bool {
	switch s {
	case StateR0, StateR1, StateI0, StateI1, StateA0, StateA1:
		return true
	default:
		return false
	}
}

// ClassifyExecutionState turns the chat row, queue cardinality, and
// whether the chat row exists into an [ExecutionState]. The caller is
// responsible for loading the chat under the row lock and reading the
// queue count in the same transaction.
//
// Callers that have no chat row (lookup returned sql.ErrNoRows)
// should pass exists=false; the chat, status, and archive arguments
// are then ignored.
//
// The classifier is a single flat switch over the valid (status,
// archived, queue) tuples in the chat execution state model. Anything
// outside that set (archived busy states, waiting with a non-empty
// queue, future enum values) falls through to [StateInvalid].
//
//nolint:revive // queueNonEmpty/exists are simple classifier inputs.
func ClassifyExecutionState(chat database.Chat, queueNonEmpty, exists bool) ExecutionState {
	return classifyExecutionState(chat.Status, chat.Archived, queueNonEmpty, exists)
}

//nolint:revive // queueNonEmpty/exists are simple classifier inputs.
func classifyExecutionState(status database.ChatStatus, archived, queueNonEmpty, exists bool) ExecutionState {
	if !exists {
		return StateN
	}
	switch {
	case status == database.ChatStatusWaiting && !archived && !queueNonEmpty:
		return StateW
	case status == database.ChatStatusWaiting && archived && !queueNonEmpty:
		return StateXW
	case status == database.ChatStatusError && !archived && !queueNonEmpty:
		return StateE0
	case status == database.ChatStatusError && !archived && queueNonEmpty:
		return StateE1
	case status == database.ChatStatusError && archived && !queueNonEmpty:
		return StateXE0
	case status == database.ChatStatusError && archived && queueNonEmpty:
		return StateXE1
	case status == database.ChatStatusRunning && !archived && !queueNonEmpty:
		return StateR0
	case status == database.ChatStatusRunning && !archived && queueNonEmpty:
		return StateR1
	case status == database.ChatStatusInterrupting && !archived && !queueNonEmpty:
		return StateI0
	case status == database.ChatStatusInterrupting && !archived && queueNonEmpty:
		return StateI1
	case status == database.ChatStatusRequiresAction && !archived && !queueNonEmpty:
		return StateA0
	case status == database.ChatStatusRequiresAction && !archived && queueNonEmpty:
		return StateA1
	}
	return StateInvalid
}
