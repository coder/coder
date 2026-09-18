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

// transitionMatrix is the in-code representation of the chat execution
// state transition table. Each entry maps an input state to the set of
// allowed transitions together with the possible classified output
// states that the transition implementation may reach. Outputs may
// depend on the post-mutation queue (for example DeleteQueuedMessage
// from E1 reaches E0 when the deleted row was the last queued
// message, in E1P when the row behind a deleted ready head is under
// edit, or stays in E1 otherwise), which is why several entries list
// more than one output.
//
// The "1P" states have a blocked queue head (see queuePaused). They
// admit what their "1" siblings admit; the cells below record where
// they differ.
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
		TransitionSendMessage:          {StateR1, StateR1P},
		TransitionEditMessage:          {StateR0},
		TransitionDeleteQueuedMessage:  {StateE0, StateE1, StateE1P},
		TransitionPromoteQueuedMessage: {StateR0, StateR1, StateR1P},
		TransitionEditQueuedMessage:    {StateE1, StateE1P},
		TransitionRequestCompaction:    {StateR1},
	},
	StateE1P: {
		TransitionSetArchived:          {StateXE1P},
		TransitionSendMessage:          {StateE1P},
		TransitionEditMessage:          {StateR0},
		TransitionDeleteQueuedMessage:  {StateE0, StateE1, StateE1P},
		TransitionPromoteQueuedMessage: {StateR0, StateR1, StateR1P},
		TransitionEditQueuedMessage:    {StateE1, StateE1P},
		TransitionRequestCompaction:    {StateR1P},
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
		TransitionDeleteQueuedMessage:     {StateR0, StateR1, StateR1P},
		TransitionPromoteQueuedMessage:    {StateI1},
		TransitionEditQueuedMessage:       {StateR1, StateR1P},
		TransitionInterrupt:               {StateI1},
		TransitionRecordGenerationAttempt: {StateR1},
		TransitionRecordRetryState:        {StateR1},
		TransitionCommitStep:              {StateR1},
		TransitionEnterRequiresAction:     {StateA1},
		TransitionFinishTurn:              {StateR0, StateR1, StateR1P},
		TransitionFinishError:             {StateE1},
	},
	StateR1P: {
		TransitionSendMessage:             {StateR1P, StateI1P},
		TransitionEditMessage:             {StateR0},
		TransitionDeleteQueuedMessage:     {StateR0, StateR1, StateR1P},
		TransitionPromoteQueuedMessage:    {StateI1},
		TransitionEditQueuedMessage:       {StateR1, StateR1P},
		TransitionInterrupt:               {StateI1P},
		TransitionRecordGenerationAttempt: {StateR1P},
		TransitionRecordRetryState:        {StateR1P},
		TransitionCommitStep:              {StateR1P},
		TransitionEnterRequiresAction:     {StateA1P},
		TransitionFinishTurn:              {StateP},
		TransitionFinishError:             {StateE1P},
	},
	StateI0: {
		TransitionSendMessage:        {StateI1},
		TransitionEditMessage:        {StateR0},
		TransitionFinishInterruption: {StateW},
	},
	StateI1: {
		TransitionSendMessage:          {StateI1},
		TransitionEditMessage:          {StateR0},
		TransitionDeleteQueuedMessage:  {StateI0, StateI1, StateI1P},
		TransitionPromoteQueuedMessage: {StateI1},
		TransitionEditQueuedMessage:    {StateI1, StateI1P},
		TransitionFinishInterruption:   {StateR0, StateR1, StateR1P},
	},
	StateI1P: {
		TransitionSendMessage:          {StateI1P},
		TransitionEditMessage:          {StateR0},
		TransitionDeleteQueuedMessage:  {StateI0, StateI1, StateI1P},
		TransitionPromoteQueuedMessage: {StateI1},
		TransitionEditQueuedMessage:    {StateI1, StateI1P},
		TransitionFinishInterruption:   {StateP},
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
		TransitionDeleteQueuedMessage:    {StateA0, StateA1, StateA1P},
		TransitionPromoteQueuedMessage:   {StateR0, StateR1, StateR1P},
		TransitionEditQueuedMessage:      {StateA1, StateA1P},
		TransitionInterrupt:              {StateR1},
		TransitionCompleteRequiresAction: {StateR1},
		TransitionCancelRequiresAction:   {StateR1},
	},
	StateA1P: {
		TransitionSendMessage:            {StateA1P, StateR1P},
		TransitionEditMessage:            {StateR0},
		TransitionDeleteQueuedMessage:    {StateA0, StateA1, StateA1P},
		TransitionPromoteQueuedMessage:   {StateR0, StateR1, StateR1P},
		TransitionEditQueuedMessage:      {StateA1, StateA1P},
		TransitionInterrupt:              {StateR1P},
		TransitionCompleteRequiresAction: {StateR1P},
		TransitionCancelRequiresAction:   {StateR1P},
	},
	StateP: {
		TransitionSendMessage:          {StateP},
		TransitionEditMessage:          {StateR0},
		TransitionDeleteQueuedMessage:  {StateP, StateW, StateR0, StateR1},
		TransitionPromoteQueuedMessage: {StateR0, StateR1, StateR1P},
		TransitionEditQueuedMessage:    {StateP, StateR0, StateR1},
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
	StateXE1P: {
		TransitionSetArchived: {StateE1P},
	},
	StateInvalid: {
		TransitionReconcileInvalidState: {StateE0, StateE1, StateE1P},
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
