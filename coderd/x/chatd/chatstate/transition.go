package chatstate

import "slices"

// Transition is the enumeration of transitions implemented by the
// state machine. Values intentionally match the names of the public
// methods on [Tx] (and [CreateChat]). The transition matrix below
// declares the legal (from -> to) execution-state mappings used by
// each transition method for validation.
type Transition string

const (
	TransitionCreateChat              Transition = "CreateChat"
	TransitionSetArchived             Transition = "SetArchived"
	TransitionSendMessage             Transition = "SendMessage"
	TransitionEditMessage             Transition = "EditMessage"
	TransitionRequestCompaction       Transition = "RequestCompaction"
	TransitionClearContext            Transition = "ClearContext"
	TransitionDeleteQueuedMessage     Transition = "DeleteQueuedMessage"
	TransitionPromoteQueuedMessage    Transition = "PromoteQueuedMessage"
	TransitionEditQueuedMessage       Transition = "EditQueuedMessage"
	TransitionInterrupt               Transition = "Interrupt"
	TransitionCompleteRequiresAction  Transition = "CompleteRequiresAction"
	TransitionAbandon                 Transition = "Abandon"
	TransitionRecordGenerationAttempt Transition = "RecordGenerationAttempt"
	TransitionRecordRetryState        Transition = "RecordRetryState"
	TransitionCommitStep              Transition = "CommitStep"
	TransitionEnterRequiresAction     Transition = "EnterRequiresAction"
	TransitionFinishInterruption      Transition = "FinishInterruption"
	TransitionFinishTurn              Transition = "FinishTurn"
	TransitionFinishError             Transition = "FinishError"
	TransitionCancelRequiresAction    Transition = "CancelRequiresAction"
	TransitionReconcileInvalidState   Transition = "ReconcileInvalidState"
)

// String implements fmt.Stringer.
func (t Transition) String() string { return string(t) }

// AllExecutionTransitions is the canonical enumeration of every
// execution-state transition that has an entry in the matrix below.
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
	TransitionEditQueuedMessage,
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

// transitionMatrix is the in-code representation of the chat execution
// state transition table. Each entry maps an input state to the set of
// allowed transitions together with the possible classified output
// states that the transition implementation may land in. Outputs may
// depend on the post-mutation queue state (for example FinishTurn from
// R1 lands in R0 when the promoted row was the last promotable
// message, or in R1 otherwise), which is why several entries list more
// than one output.
//
// The queue transitions (EditQueuedMessage, DeleteQueuedMessage,
// PromoteQueuedMessage) are declared once per status in
// queueTransitionRows and merged in by withQueueTransitions; see that
// table for why they do not distinguish the "0" and "1" variants.
//
// Ownership transitions (Acquire, Abandon) are intentionally not
// included; they are orthogonal to execution state.
var transitionMatrix = withQueueTransitions(map[ExecutionState]map[Transition][]ExecutionState{
	StateN: {
		TransitionCreateChat: {StateR0},
	},
	StateW: {
		TransitionSetArchived:       {StateXW},
		TransitionSendMessage:       {StateR0},
		TransitionEditMessage:       {StateR0},
		TransitionRequestCompaction: {StateR0},
		TransitionClearContext:      {StateW},
		TransitionFinishError:       {StateE0},
	},
	StateE0: {
		TransitionSetArchived:       {StateXE0},
		TransitionSendMessage:       {StateR0},
		TransitionEditMessage:       {StateR0},
		TransitionRequestCompaction: {StateR0},
		TransitionClearContext:      {StateW},
	},
	StateE1: {
		TransitionSetArchived:       {StateXE1},
		TransitionSendMessage:       {StateR1},
		TransitionEditMessage:       {StateR0},
		TransitionRequestCompaction: {StateR1},
	},
	StateR0: {
		TransitionSendMessage:             {StateR1, StateI1},
		TransitionEditMessage:             {StateR0},
		TransitionInterrupt:               {StateI0},
		TransitionRecordGenerationAttempt: {StateR0},
		TransitionRecordRetryState:        {StateR0},
		TransitionCommitStep:              {StateR0},
		TransitionEnterRequiresAction:     {StateA0},
		TransitionFinishTurn:              {StateW},
		TransitionFinishError:             {StateE0},
	},
	StateR1: {
		TransitionSendMessage:             {StateR1, StateI1},
		TransitionEditMessage:             {StateR0},
		TransitionInterrupt:               {StateI1},
		TransitionRecordGenerationAttempt: {StateR1},
		TransitionRecordRetryState:        {StateR1},
		TransitionCommitStep:              {StateR1},
		TransitionEnterRequiresAction:     {StateA1},
		TransitionFinishTurn:              {StateR0, StateR1},
		TransitionFinishError:             {StateE1},
	},
	StateI0: {
		TransitionSendMessage:        {StateI1},
		TransitionEditMessage:        {StateR0},
		TransitionFinishInterruption: {StateW},
	},
	StateI1: {
		TransitionSendMessage:        {StateI1},
		TransitionEditMessage:        {StateR0},
		TransitionFinishInterruption: {StateR0, StateR1},
	},
	StateA0: {
		TransitionSendMessage:            {StateA1, StateR1},
		TransitionEditMessage:            {StateR0},
		TransitionInterrupt:              {StateR0},
		TransitionCompleteRequiresAction: {StateR0},
		TransitionCancelRequiresAction:   {StateR0},
	},
	StateA1: {
		TransitionSendMessage:            {StateA1, StateR1},
		TransitionEditMessage:            {StateR0},
		TransitionInterrupt:              {StateR1},
		TransitionCompleteRequiresAction: {StateR1},
		TransitionCancelRequiresAction:   {StateR1},
	},
	StateXW: {
		TransitionSetArchived: {StateW},
	},
	StateXE0: {
		TransitionSetArchived: {StateE0},
	},
	StateXE1: {
		TransitionSetArchived: {StateE1},
	},
	StateInvalid: {
		TransitionReconcileInvalidState: {StateE0, StateE1},
	},
})

// queueTransitionRow declares the queue transitions for one status.
type queueTransitionRow struct {
	// states are both queue variants of the status (waiting has one).
	states []ExecutionState
	// settle is the output set shared by EditQueuedMessage and
	// DeleteQueuedMessage. Both only rewrite the queue and then settle:
	// the status is unchanged and the queue variant is whatever the
	// rewrite left behind, except that waiting with a promotable head is
	// invalid, so from W a newly exposed head is promoted and the chat
	// runs.
	settle []ExecutionState
	// promote is the output set of PromoteQueuedMessage: running and
	// interrupting chats hand the target to the interruption boundary;
	// every other status pops it into history and runs.
	promote []ExecutionState
}

// queueTransitionRows declares the queue transitions once per status.
// The "0" variant of a status is not "no rows": a held head classifies
// as "0" while it and the rows behind it still exist, so both variants
// admit the queue transitions with the same outputs. Archived states
// and Invalid admit none.
var queueTransitionRows = []queueTransitionRow{
	{states: []ExecutionState{StateW}, settle: []ExecutionState{StateW, StateR0, StateR1}, promote: []ExecutionState{StateR0, StateR1}},
	{states: []ExecutionState{StateE0, StateE1}, settle: []ExecutionState{StateE0, StateE1}, promote: []ExecutionState{StateR0, StateR1}},
	{states: []ExecutionState{StateR0, StateR1}, settle: []ExecutionState{StateR0, StateR1}, promote: []ExecutionState{StateI1}},
	{states: []ExecutionState{StateI0, StateI1}, settle: []ExecutionState{StateI0, StateI1}, promote: []ExecutionState{StateI1}},
	{states: []ExecutionState{StateA0, StateA1}, settle: []ExecutionState{StateA0, StateA1}, promote: []ExecutionState{StateR0, StateR1}},
}

// withQueueTransitions merges queueTransitionRows into m and returns m.
func withQueueTransitions(m map[ExecutionState]map[Transition][]ExecutionState) map[ExecutionState]map[Transition][]ExecutionState {
	for _, row := range queueTransitionRows {
		for _, from := range row.states {
			m[from][TransitionEditQueuedMessage] = row.settle
			m[from][TransitionDeleteQueuedMessage] = row.settle
			m[from][TransitionPromoteQueuedMessage] = row.promote
		}
	}
	return m
}

// isExecutionTransitionAllowed reports whether a transition is legal
// from the supplied input state per the matrix above. Ownership
// transitions are not stored in the matrix and always return false.
func isExecutionTransitionAllowed(t Transition, from ExecutionState) bool {
	allowed, ok := transitionMatrix[from]
	if !ok {
		return false
	}
	_, ok = allowed[t]
	return ok
}

// requireExecutionTransition validates that t is legal from `from`
// and returns a typed *TransitionError otherwise.
func requireExecutionTransition(t Transition, from ExecutionState) error {
	if isExecutionTransitionAllowed(t, from) {
		return nil
	}
	return newTransitionError(t, from, "")
}

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
// from which `tr` is legal per the matrix above. Mostly used by tests
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
// the matrix above. The returned slice is a copy so callers may mutate
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
