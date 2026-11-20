package chatstate

import "slices"

// Transition is the enumeration of transitions implemented by the
// state machine. Values intentionally match the names of the public
// methods on [Tx] (and [CreateChat]). The transition matrix below
// declares the states from which each transition method is legal.
type Transition string

const (
	TransitionCreateChat              Transition = "CreateChat"
	TransitionSetArchived             Transition = "SetArchived"
	TransitionSetProject              Transition = "SetProject"
	TransitionSendMessage             Transition = "SendMessage"
	TransitionEditMessage             Transition = "EditMessage"
	TransitionRequestCompaction       Transition = "RequestCompaction"
	TransitionClearContext            Transition = "ClearContext"
	TransitionDeleteQueuedMessage     Transition = "DeleteQueuedMessage"
	TransitionSteerQueuedMessage      Transition = "SteerQueuedMessage"
	TransitionEditQueuedMessage       Transition = "EditQueuedMessage"
	TransitionInterrupt               Transition = "Interrupt"
	TransitionCompleteRequiresAction  Transition = "CompleteRequiresAction"
	TransitionAbandon                 Transition = "Abandon"
	TransitionRecordGenerationAttempt Transition = "RecordGenerationAttempt"
	TransitionRecordRetryState        Transition = "RecordRetryState"
	TransitionDeliverSteerMessages    Transition = "DeliverSteerMessages"
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

// transitionMatrix lists the transitions that each execution state
// admits. The states a transition can reach are covered by the cases
// of TestTransitionMatrix_AllCombinations.
//
// The "1P" states have a blocked queue head (see queuePaused). Each
// admits the same transitions as its "1" sibling.
//
// Ownership transitions (Acquire, Abandon) are intentionally not
// included; they are orthogonal to execution state.
var transitionMatrix = map[ExecutionState][]Transition{
	StateN: {
		TransitionCreateChat,
	},
	StateW: {
		TransitionSetArchived,
		TransitionSetProject,
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionRequestCompaction,
		TransitionClearContext,
		TransitionInterrupt,
		TransitionFinishError,
	},
	StateE0: {
		TransitionSetArchived,
		TransitionSetProject,
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionRequestCompaction,
		TransitionClearContext,
		TransitionInterrupt,
	},
	StateE1: {
		TransitionSetArchived,
		TransitionSetProject,
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionRequestCompaction,
		TransitionInterrupt,
	},
	StateE1P: {
		TransitionSetArchived,
		TransitionSetProject,
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionRequestCompaction,
		TransitionInterrupt,
	},
	StateR0: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionInterrupt,
		TransitionRecordGenerationAttempt,
		TransitionRecordRetryState,
		TransitionDeliverSteerMessages,
		TransitionCommitStep,
		TransitionEnterRequiresAction,
		TransitionFinishTurn,
		TransitionFinishError,
	},
	StateR1: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionInterrupt,
		TransitionRecordGenerationAttempt,
		TransitionRecordRetryState,
		TransitionDeliverSteerMessages,
		TransitionCommitStep,
		TransitionEnterRequiresAction,
		TransitionFinishTurn,
		TransitionFinishError,
	},
	StateR1P: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionInterrupt,
		TransitionRecordGenerationAttempt,
		TransitionRecordRetryState,
		TransitionDeliverSteerMessages,
		TransitionCommitStep,
		TransitionEnterRequiresAction,
		TransitionFinishTurn,
		TransitionFinishError,
	},
	StateI0: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionInterrupt,
		TransitionFinishInterruption,
	},
	StateI1: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionInterrupt,
		TransitionFinishInterruption,
	},
	StateI1P: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionInterrupt,
		TransitionFinishInterruption,
	},
	StateA0: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionInterrupt,
		TransitionCompleteRequiresAction,
		TransitionCancelRequiresAction,
	},
	StateA1: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionInterrupt,
		TransitionCompleteRequiresAction,
		TransitionCancelRequiresAction,
	},
	StateA1P: {
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionInterrupt,
		TransitionCompleteRequiresAction,
		TransitionCancelRequiresAction,
	},
	StateP: {
		TransitionSetProject,
		TransitionSendMessage,
		TransitionEditMessage,
		TransitionDeleteQueuedMessage,
		TransitionSteerQueuedMessage,
		TransitionEditQueuedMessage,
		TransitionInterrupt,
	},
	StateXW: {
		TransitionSetArchived,
	},
	StateXE0: {
		TransitionSetArchived,
	},
	StateXE1: {
		TransitionSetArchived,
	},
	StateXE1P: {
		TransitionSetArchived,
	},
	StateInvalid: {
		TransitionReconcileInvalidState,
	},
}

// isExecutionTransitionAllowed reports whether a transition is legal
// from the supplied input state per the matrix above. Ownership
// transitions are not stored in the matrix and always return false.
func isExecutionTransitionAllowed(t Transition, from ExecutionState) bool {
	return slices.Contains(transitionMatrix[from], t)
}

// requireExecutionTransition validates that t is legal from `from`
// and returns a typed *TransitionError otherwise.
func requireExecutionTransition(t Transition, from ExecutionState) error {
	if isExecutionTransitionAllowed(t, from) {
		return nil
	}
	return newTransitionError(t, from, "")
}
