package chatstate

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

// transitionMatrix is the in-code representation of the chat execution
// state transition table. Each entry maps an input state to the set of
// allowed transitions together with the possible classified output
// states that the transition implementation may land in. Outputs may
// depend on the post-mutation queue cardinality (for example
// DeleteQueuedMessage from E1 lands in E0 when the deleted row was the
// last queued message, or stays in E1 otherwise), which is why several
// entries list more than one output.
//
// Ownership transitions (Acquire, Abandon) are intentionally not
// included; they are orthogonal to execution state.
var transitionMatrix = map[ExecutionState]map[Transition][]ExecutionState{
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
		TransitionSetArchived:          {StateXE1},
		TransitionSendMessage:          {StateR1},
		TransitionEditMessage:          {StateR0},
		TransitionDeleteQueuedMessage:  {StateE0, StateE1},
		TransitionPromoteQueuedMessage: {StateR0, StateR1},
		TransitionRequestCompaction:    {StateR1},
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
		TransitionDeleteQueuedMessage:     {StateR0, StateR1},
		TransitionPromoteQueuedMessage:    {StateI1},
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
		TransitionSendMessage:          {StateI1},
		TransitionEditMessage:          {StateR0},
		TransitionDeleteQueuedMessage:  {StateI0, StateI1},
		TransitionPromoteQueuedMessage: {StateI1},
		TransitionFinishInterruption:   {StateR0, StateR1},
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
		TransitionDeleteQueuedMessage:    {StateA0, StateA1},
		TransitionPromoteQueuedMessage:   {StateR0, StateR1},
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
