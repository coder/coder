package chatloop

import (
	"context"
	"maps"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// TurnCategory is the category label of turn_time_seconds_total.
// unattributed carries the remainder, so a turn's categories sum to its
// duration unless it is overattributed.
type TurnCategory string

// TurnCategory values.
const (
	TurnCategoryScheduling       TurnCategory = "scheduling"
	TurnCategoryTimeToFirstToken TurnCategory = "time_to_first_token"
	TurnCategoryStreaming        TurnCategory = "streaming"
	TurnCategoryProviderError    TurnCategory = "provider_error"
	TurnCategoryRetryBackoff     TurnCategory = "retry_backoff"
	TurnCategoryToolExecution    TurnCategory = "tool_execution"
	TurnCategoryCompaction       TurnCategory = "compaction"
	TurnCategoryPreparation      TurnCategory = "preparation"
	TurnCategoryPersistence      TurnCategory = "persistence"
	TurnCategoryChatdOverhead    TurnCategory = "chatd_overhead"
	TurnCategoryUnattributed     TurnCategory = "unattributed"
)

// turnTimeCategories lists every TurnCategory; each is emitted for
// every accounted turn.
var turnTimeCategories = []TurnCategory{
	TurnCategoryScheduling,
	TurnCategoryTimeToFirstToken,
	TurnCategoryStreaming,
	TurnCategoryProviderError,
	TurnCategoryRetryBackoff,
	TurnCategoryToolExecution,
	TurnCategoryCompaction,
	TurnCategoryPreparation,
	TurnCategoryPersistence,
	TurnCategoryChatdOverhead,
	TurnCategoryUnattributed,
}

// attributingStages take their duration out of their parent's own time
// and put their own time in the mapped category, which stageNode.category
// can override. provider_attempt, thinking, and tool_call are absent
// because they overlap stages already categorized.
var attributingStages = map[Stage]TurnCategory{
	StageGenerationStep:   TurnCategoryChatdOverhead,
	StagePrepare:          TurnCategoryPreparation,
	StageMCPConnect:       TurnCategoryPreparation,
	StageCommit:           TurnCategoryPersistence,
	StageStream:           TurnCategoryStreaming,
	StageTimeToFirstToken: TurnCategoryTimeToFirstToken,
	StageCompaction:       TurnCategoryCompaction,
	StageRetryBackoff:     TurnCategoryRetryBackoff,
}

// recordedStageCategories categorizes the full duration of stages built
// from timestamps; they have no attributing children. capacity_wait is
// absent: its window lies inside acquisition.
var recordedStageCategories = map[Stage]TurnCategory{
	StageAcquisition: TurnCategoryScheduling,
}

// turnAccumulatorKey keys the turn accumulator carried by a context.
type turnAccumulatorKey struct{}

// stageNodeKey keys the innermost attributing stage of a context.
type stageNodeKey struct{}

// ContextWithTurnAccumulator returns ctx carrying acc, so the stages
// started on it and on its descendants accumulate into the same turn.
func ContextWithTurnAccumulator(ctx context.Context, acc *TurnAccumulator) context.Context {
	return context.WithValue(ctx, turnAccumulatorKey{}, acc)
}

func turnAccumulatorFromContext(ctx context.Context) *TurnAccumulator {
	acc, _ := ctx.Value(turnAccumulatorKey{}).(*TurnAccumulator)
	return acc
}

func stageNodeFromContext(ctx context.Context) *stageNode {
	node, _ := ctx.Value(stageNodeKey{}).(*stageNode)
	return node
}

// TurnAccumulator sums one turn's category times for emission when the
// turn closes. It is safe for concurrent use: stages of one turn, such
// as parallel mcp_connect stages, end on different goroutines.
type TurnAccumulator struct {
	mu         sync.Mutex
	categories map[TurnCategory]time.Duration
	stageModel StageModel
}

// NewTurnAccumulator returns an empty accumulator for one turn.
func NewTurnAccumulator() *TurnAccumulator {
	return &TurnAccumulator{
		categories: map[TurnCategory]time.Duration{},
	}
}

func (a *TurnAccumulator) addCategory(category TurnCategory, elapsed time.Duration) {
	if a == nil || category == "" || elapsed <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.categories[category] += elapsed
}

func (a *TurnAccumulator) setModel(model StageModel) {
	if a == nil || model.Model == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stageModel.Model == "" {
		a.stageModel = model
	}
}

// model returns the first model a stage of the turn resolved, empty
// until one does.
func (a *TurnAccumulator) model() StageModel {
	if a == nil {
		return StageModel{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stageModel
}

// snapshot returns a copy of the category totals; it is empty for a nil
// accumulator.
func (a *TurnAccumulator) snapshot() map[TurnCategory]time.Duration {
	if a == nil {
		return map[TurnCategory]time.Duration{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return maps.Clone(a.categories)
}

// stageNode is the attribution parent of the attributing stages
// started beneath it. A child reports its full duration to its parent
// so the parent's own category only receives the time it did not spend
// inside a child, which keeps the categories disjoint.
type stageNode struct {
	stage  Stage
	parent *stageNode

	mu         sync.Mutex
	childTotal time.Duration
	// action is set only on a generation_step.
	action string
	// failed marks a stream stage whose time_to_first_token child
	// ended with an error.
	failed bool
}

func (n *stageNode) setAction(action string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.action = action
}

func (n *stageNode) markFailed() {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.failed = true
}

// addChild takes elapsed out of the parent's own time.
func (n *stageNode) addChild(elapsed time.Duration) {
	if n == nil || elapsed <= 0 {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.childTotal += elapsed
}

func (n *stageNode) state() nodeState {
	if n == nil {
		return nodeState{}
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return nodeState{childTotal: n.childTotal, action: n.action, failed: n.failed}
}

// nodeState is a stage's attribution state at the moment it ends.
type nodeState struct {
	childTotal time.Duration
	action     string
	failed     bool
}

// category returns the category of the stage's own time.
func (n *stageNode) category(state nodeState, err error) TurnCategory {
	switch {
	case n.stage == StageGenerationStep && state.action == GenerationActionExecuteLocalTools:
		return TurnCategoryToolExecution
	case n.stage == StageStream && (state.failed || err != nil),
		n.stage == StageTimeToFirstToken && err != nil:
		return TurnCategoryProviderError
	default:
		return attributingStages[n.stage]
	}
}

// report adds the stage's own time, elapsed minus the time of its
// attributing children, to its category, and elapsed to its parent's
// child time. A negative own time is dropped.
func (s *StageSpan) report(elapsed time.Duration, err error) {
	if s.acc == nil || s.node == nil {
		return
	}
	state := s.node.state()
	s.acc.addCategory(s.node.category(state, err), elapsed-state.childTotal)
	if s.stage == StageTimeToFirstToken && err != nil {
		s.node.parent.markFailed()
	}
	s.node.parent.addChild(elapsed)
}

// EndTurn closes a chat_turn span like End, sets its turn_outcome
// attribute, counts the outcome, and emits the turn's category
// partition labeled with it. Only a completed turn is observed on the
// stage histogram. Calls after the first are ignored.
func (s *StageSpan) EndTurn(outcome TurnOutcome, err error) {
	if s == nil || s.ended {
		return
	}
	s.span.SetAttributes(attribute.String(AttrTurnOutcome, string(outcome)))
	s.adoptTurnModel()
	elapsed, ok := s.closeSpan(err)
	if !ok {
		return
	}
	if outcome == TurnOutcomeCompleted {
		s.tracer.observe(s.stage, s.scope, s.chatKind, s.model, elapsed)
	}
	s.tracer.emitTurnAccounting(s.acc, s.chatKind, outcome, elapsed)
}

// categorizeRecordedStage adds the full duration of a stage built from
// timestamps to the turn on ctx.
func categorizeRecordedStage(ctx context.Context, stage Stage, elapsed time.Duration) {
	acc := turnAccumulatorFromContext(ctx)
	if acc == nil {
		return
	}
	acc.addCategory(recordedStageCategories[stage], elapsed)
}

// emitTurnAccounting counts a closed turn's outcome and records its
// category partition. turnDuration is the chat_turn span's wall time; a
// non-positive one records no partition.
func (t *StageTracer) emitTurnAccounting(acc *TurnAccumulator, chatKind ChatKind, outcome TurnOutcome, turnDuration time.Duration) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordTurnOutcome(outcome, chatKind)
	if turnDuration <= 0 {
		t.RecordAnomaly(StageAnomalyNonPositiveTurn)
		return
	}
	categories := acc.snapshot()
	var attributed time.Duration
	for _, category := range turnTimeCategories {
		attributed += categories[category]
	}
	unattributed := turnDuration - attributed
	if unattributed < 0 {
		unattributed = 0
		t.RecordAnomaly(StageAnomalyOverattributed)
	}
	categories[TurnCategoryUnattributed] = unattributed
	for _, category := range turnTimeCategories {
		t.metrics.RecordTurnCategory(category, chatKind, outcome, categories[category])
	}
}
