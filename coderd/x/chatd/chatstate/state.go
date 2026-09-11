package chatstate

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

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
	// StateP: waiting, non-empty queue whose head is held, not
	// archived. The last turn ended while the owner was editing the
	// head, so the chat paused instead of promoting it. Idle to the
	// worker like W; a send queues instead of running. Left by
	// releasing the head, sending it now, deleting it, or editing
	// history.
	StateP ExecutionState = "P"
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

// AllExecutionStates is the canonical enumeration of every value the
// classifier can return. Tests rely on this list to iterate over every
// state when verifying transition coverage.
var AllExecutionStates = []ExecutionState{
	StateN,
	StateW,
	StateE0,
	StateE1,
	StateR0,
	StateR1,
	StateI0,
	StateI1,
	StateA0,
	StateA1,
	StateP,
	StateXW,
	StateXE0,
	StateXE1,
	StateInvalid,
}

// IsRunnable returns true for the execution states that the chat
// worker is allowed to acquire and drive forward: R0, R1, I0, I1,
// A0, and A1. Requires-action states need worker ownership for
// timeout processing. Other states are idle (W, P, E*, XW, XE*),
// absent (N), or invalid.
func (s ExecutionState) IsRunnable() bool {
	switch s {
	case StateR0, StateR1, StateI0, StateI1, StateA0, StateA1:
		return true
	default:
		return false
	}
}

// QueueState is the queue input to [ClassifyExecutionState].
type QueueState struct {
	// HasRows is true when the queue is non-empty. This is the "1"
	// queue sub-state.
	HasRows bool
	// HeadHeld is true when the head row has held_at set. It only
	// matters together with a waiting status, where it distinguishes
	// P from the invalid waiting-with-rows shape.
	HeadHeld bool
}

// LoadQueueState reads the queue head in the caller's transaction and
// derives the classifier input. An empty queue yields the zero value.
func LoadQueueState(ctx context.Context, store database.Store, chatID uuid.UUID) (QueueState, error) {
	head, err := store.GetChatQueuedMessageHead(ctx, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueState{}, nil
	}
	if err != nil {
		return QueueState{}, xerrors.Errorf("get queue head: %w", err)
	}
	return QueueState{HasRows: true, HeadHeld: head.HeldAt.Valid}, nil
}

// ClassifyExecutionState turns the chat row, queue state, and whether
// the chat row exists into an [ExecutionState]. The caller is
// responsible for loading the chat under the row lock and reading the
// queue state in the same transaction.
//
// Callers that have no chat row (lookup returned sql.ErrNoRows)
// should pass exists=false; the chat, status, and archive arguments
// are then ignored.
//
// The classifier is a single flat switch over the valid (status,
// archived, queue) tuples in the chat execution state model. Anything
// outside that set (archived busy states, waiting with a non-empty
// queue whose head is not held, future enum values) falls through to
// [StateInvalid].
//
//nolint:revive // exists is a simple classifier input.
func ClassifyExecutionState(chat database.Chat, queue QueueState, exists bool) ExecutionState {
	if !exists {
		return StateN
	}
	queueNonEmpty := queue.HasRows
	switch {
	case chat.Status == database.ChatStatusWaiting && !chat.Archived && !queueNonEmpty:
		return StateW
	case chat.Status == database.ChatStatusWaiting && !chat.Archived && queueNonEmpty && queue.HeadHeld:
		return StateP
	case chat.Status == database.ChatStatusWaiting && chat.Archived && !queueNonEmpty:
		return StateXW
	case chat.Status == database.ChatStatusError && !chat.Archived && !queueNonEmpty:
		return StateE0
	case chat.Status == database.ChatStatusError && !chat.Archived && queueNonEmpty:
		return StateE1
	case chat.Status == database.ChatStatusError && chat.Archived && !queueNonEmpty:
		return StateXE0
	case chat.Status == database.ChatStatusError && chat.Archived && queueNonEmpty:
		return StateXE1
	case chat.Status == database.ChatStatusRunning && !chat.Archived && !queueNonEmpty:
		return StateR0
	case chat.Status == database.ChatStatusRunning && !chat.Archived && queueNonEmpty:
		return StateR1
	case chat.Status == database.ChatStatusInterrupting && !chat.Archived && !queueNonEmpty:
		return StateI0
	case chat.Status == database.ChatStatusInterrupting && !chat.Archived && queueNonEmpty:
		return StateI1
	case chat.Status == database.ChatStatusRequiresAction && !chat.Archived && !queueNonEmpty:
		return StateA0
	case chat.Status == database.ChatStatusRequiresAction && !chat.Archived && queueNonEmpty:
		return StateA1
	}
	return StateInvalid
}
