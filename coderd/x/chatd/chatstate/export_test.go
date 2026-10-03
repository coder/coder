package chatstate

import (
	"slices"

	"github.com/google/uuid"
)

// AllowedExecutionTransitionsFrom returns a deterministic slice of
// transitions legal from `from`. Mostly used by tests to enumerate the
// matrix without leaking the internal map.
func AllowedExecutionTransitionsFrom(from ExecutionState) []Transition {
	allowed := transitionMatrix[from]
	out := make([]Transition, 0, len(allowed))
	for _, t := range AllExecutionTransitions {
		if _, ok := allowed[t]; ok {
			out = append(out, t)
		}
	}
	return out
}

// AllowedInputStates returns a deterministic slice of execution states
// from which `tr` is legal per transitionMatrix. Mostly used by tests
// to enumerate the matrix without leaking the internal map.
func AllowedInputStates(tr Transition) []ExecutionState {
	var out []ExecutionState
	for _, from := range AllExecutionStates {
		if isExecutionTransitionAllowed(tr, from) {
			out = append(out, from)
		}
	}
	return out
}

// AllowedExecutionTransitionOutputs returns the set of classified
// post-states that the transition `tr` may produce from `from` per
// transitionMatrix. The returned slice is a copy so callers may mutate
// it without affecting the underlying matrix.
//
// When `tr` is not allowed from `from`, an empty (nil) slice is
// returned. Tests use this helper to enumerate the (transition, from,
// want) triples that must be exercised by the row-level matrix tests.
func AllowedExecutionTransitionOutputs(from ExecutionState, tr Transition) []ExecutionState {
	allowed, ok := transitionMatrix[from]
	if !ok {
		return nil
	}
	outputs, ok := allowed[tr]
	if !ok {
		return nil
	}
	return slices.Clone(outputs)
}

// ChatID returns the chat ID this machine is scoped to.
func (m *ChatMachine) ChatID() uuid.UUID { return m.chatID }

// snapshotPending returns a snapshot of the buffered messages for
// tests via [PublishBuffer.BufferedChannels]. The returned slice is a
// copy and safe to inspect without holding the buffer lock.
func (b *PublishBuffer) snapshotPending() []bufferedMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]bufferedMessage, len(b.pending))
	copy(out, b.pending)
	return out
}

// BufferedChannels returns just the channels of the pending messages
// in order. Primarily useful for assertions in tests.
func (b *PublishBuffer) BufferedChannels() []string {
	pending := b.snapshotPending()
	out := make([]string, len(pending))
	for i, m := range pending {
		out[i] = m.Channel
	}
	return out
}

// AllExecutionTransitions is the canonical enumeration of every
// execution-state transition that has an entry in transitionMatrix.
// Ownership transitions (Acquire, Abandon) are intentionally not part
// of this slice because they are validated independently and do not
// have a (from->to) execution mapping.
var AllExecutionTransitions = []Transition{
	TransitionCreateChat,
	TransitionSetArchived,
	TransitionSendMessage,
	TransitionEditMessage,
	TransitionRequestCompaction,
	TransitionClearContext,
	TransitionDeleteQueuedMessage,
	TransitionPromoteQueuedMessage,
	TransitionInterrupt,
	TransitionCompleteRequiresAction,
	TransitionRecordGenerationAttempt,
	TransitionRecordRetryState,
	TransitionCommitStep,
	TransitionEnterRequiresAction,
	TransitionFinishInterruption,
	TransitionFinishTurn,
	TransitionFinishError,
	TransitionCancelRequiresAction,
	TransitionReconcileInvalidState,
}

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
	StateXW,
	StateXE0,
	StateXE1,
	StateInvalid,
}
