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
	// StateE1: error, ready queue head, not archived.
	StateE1 ExecutionState = "E1"
	// StateE1P: error, blocked queue head, not archived.
	StateE1P ExecutionState = "E1P"
	// StateR0: running, empty queue, not archived.
	StateR0 ExecutionState = "R0"
	// StateR1: running, ready queue head, not archived.
	StateR1 ExecutionState = "R1"
	// StateR1P: running, blocked queue head, not archived.
	StateR1P ExecutionState = "R1P"
	// StateI0: interrupting, empty queue, not archived.
	StateI0 ExecutionState = "I0"
	// StateI1: interrupting, ready queue head, not archived.
	StateI1 ExecutionState = "I1"
	// StateI1P: interrupting, blocked queue head, not archived.
	StateI1P ExecutionState = "I1P"
	// StateA0: requires_action, empty queue, not archived.
	StateA0 ExecutionState = "A0"
	// StateA1: requires_action, ready queue head, not archived.
	StateA1 ExecutionState = "A1"
	// StateA1P: requires_action, blocked queue head, not archived.
	StateA1P ExecutionState = "A1P"
	// StateP: paused, blocked queue head, not archived.
	StateP ExecutionState = "P"
	// StateXW: archived waiting, empty queue.
	StateXW ExecutionState = "XW"
	// StateXE0: archived error, empty queue.
	StateXE0 ExecutionState = "XE0"
	// StateXE1: archived error, ready queue head.
	StateXE1 ExecutionState = "XE1"
	// StateXE1P: archived error, blocked queue head.
	StateXE1P ExecutionState = "XE1P"

	// StateInvalid groups every status/archive/queue combination that
	// is not one of the valid states above. The state machine refuses
	// non-reconciliation transitions on invalid states and exposes the
	// [Tx.ReconcileInvalidState] transition to recover.
	StateInvalid ExecutionState = "Invalid"
)

// String implements fmt.Stringer.
func (s ExecutionState) String() string { return string(s) }

// IsRunnable returns true for the execution states that the chat
// worker is allowed to acquire and drive forward: R0, R1, R1P, I0,
// I1, I1P, A0, A1, and A1P. A blocked queue head does not stop the
// current turn, so the P variants of the busy states stay runnable.
// Requires-action states need worker ownership for timeout
// processing. Other states are idle (W, E*, XW, XE*), paused (P),
// absent (N), or invalid.
func (s ExecutionState) IsRunnable() bool {
	switch s {
	case StateR0, StateR1, StateR1P, StateI0, StateI1, StateI1P, StateA0, StateA1, StateA1P:
		return true
	default:
		return false
	}
}

// QueueState is the queue input to [ClassifyExecutionState].
type QueueState struct {
	// HasRows is the "1" queue sub-state.
	HasRows bool
	// Paused is [queuePaused] of the head: the "P" suffix of the "1"
	// sub-state, and the condition P requires.
	Paused bool
}

// queuePaused reports whether a pause condition holds for the queue
// head, so it must not be promoted. Each pause mechanism adds its
// condition here.
func queuePaused(head database.ChatQueuedMessage) bool {
	return head.EditingSince.Valid
}

// LoadQueueState reads the queue head in the caller's transaction. An
// empty queue yields the zero value.
func LoadQueueState(ctx context.Context, store database.Store, chatID uuid.UUID) (QueueState, error) {
	head, err := store.GetChatQueuedMessageHead(ctx, chatID)
	if errors.Is(err, sql.ErrNoRows) {
		return QueueState{}, nil
	}
	if err != nil {
		return QueueState{}, xerrors.Errorf("get queue head: %w", err)
	}
	return QueueState{HasRows: true, Paused: queuePaused(head)}, nil
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
// archived, queue) tuples in the chat execution state model. A
// non-empty queue classifies by its head: ready ("1") or blocked by a
// pause condition ("1P"). Anything outside that set (archived busy
// states, waiting with a non-empty queue, paused without a pause
// condition, future enum values) falls through to [StateInvalid].
//
//nolint:revive // exists is a simple classifier input.
func ClassifyExecutionState(chat database.Chat, queue QueueState, exists bool) ExecutionState {
	if !exists {
		return StateN
	}
	ready := queue.HasRows && !queue.Paused
	blocked := queue.HasRows && queue.Paused
	switch {
	case chat.Status == database.ChatStatusWaiting && !chat.Archived && !queue.HasRows:
		return StateW
	case chat.Status == database.ChatStatusPaused && !chat.Archived && blocked:
		return StateP
	case chat.Status == database.ChatStatusWaiting && chat.Archived && !queue.HasRows:
		return StateXW
	case chat.Status == database.ChatStatusError && !chat.Archived && !queue.HasRows:
		return StateE0
	case chat.Status == database.ChatStatusError && !chat.Archived && ready:
		return StateE1
	case chat.Status == database.ChatStatusError && !chat.Archived && blocked:
		return StateE1P
	case chat.Status == database.ChatStatusError && chat.Archived && !queue.HasRows:
		return StateXE0
	case chat.Status == database.ChatStatusError && chat.Archived && ready:
		return StateXE1
	case chat.Status == database.ChatStatusError && chat.Archived && blocked:
		return StateXE1P
	case chat.Status == database.ChatStatusRunning && !chat.Archived && !queue.HasRows:
		return StateR0
	case chat.Status == database.ChatStatusRunning && !chat.Archived && ready:
		return StateR1
	case chat.Status == database.ChatStatusRunning && !chat.Archived && blocked:
		return StateR1P
	case chat.Status == database.ChatStatusInterrupting && !chat.Archived && !queue.HasRows:
		return StateI0
	case chat.Status == database.ChatStatusInterrupting && !chat.Archived && ready:
		return StateI1
	case chat.Status == database.ChatStatusInterrupting && !chat.Archived && blocked:
		return StateI1P
	case chat.Status == database.ChatStatusRequiresAction && !chat.Archived && !queue.HasRows:
		return StateA0
	case chat.Status == database.ChatStatusRequiresAction && !chat.Archived && ready:
		return StateA1
	case chat.Status == database.ChatStatusRequiresAction && !chat.Archived && blocked:
		return StateA1P
	}
	return StateInvalid
}
