package chatloop

import (
	"context"
	"errors"
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
// from timestamps; they have no attributing children.
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

// TurnAccumulator sums one turn's category times until the turn is
// emitted. It is safe for concurrent use: parallel tool calls set
// the turn's model from their own goroutines, and a canceled task's
// step can end while another goroutine closes the turn.
type TurnAccumulator struct {
	mu         sync.Mutex
	categories map[TurnCategory]time.Duration
	stageModel StageModel
	// endAt is the turn's end time once SetEnd fixes it; zero until then.
	endAt time.Time
}

// NewTurnAccumulator returns an empty accumulator for one turn.
func NewTurnAccumulator() *TurnAccumulator {
	return &TurnAccumulator{
		categories: map[TurnCategory]time.Duration{},
	}
}

// SetEnd fixes the turn's end time. A stage reported afterwards counts
// only the part of its window before end, and nothing when it started
// at or after end.
func (a *TurnAccumulator) SetEnd(end time.Time) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.endAt = end
}

// clamp returns the part of the window of length elapsed starting at
// start that falls before the turn's end time, or elapsed when no end
// time is set.
func (a *TurnAccumulator) clamp(start time.Time, elapsed time.Duration) time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.endAt.IsZero() {
		return elapsed
	}
	return max(min(elapsed, a.endAt.Sub(start)), 0)
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
}

func (n *stageNode) setAction(action string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.action = action
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
	return nodeState{childTotal: n.childTotal, action: n.action}
}

// nodeState is a stage's attribution state at the moment it ends.
type nodeState struct {
	childTotal time.Duration
	action     string
}

// category returns the category of the stage's own time.
func (n *stageNode) category(state nodeState, err error) TurnCategory {
	switch {
	case n.stage == StageGenerationStep && state.action == GenerationActionExecuteLocalTools:
		return TurnCategoryToolExecution
	// A stream canceled with its context, as when a chat is stopped or
	// coderd shuts down, keeps the stage's category.
	case (n.stage == StageStream || n.stage == StageTimeToFirstToken) && err != nil && !errors.Is(err, context.Canceled):
		return TurnCategoryProviderError
	default:
		return attributingStages[n.stage]
	}
}

// report adds the stage's own time to its category and its duration
// to its parent, both limited to the turn's end time; a negative own
// time is dropped.
func (s *StageSpan) report(elapsed time.Duration, err error) {
	if s.acc == nil || s.node == nil {
		return
	}
	elapsed = s.acc.clamp(s.start, elapsed)
	state := s.node.state()
	s.acc.addCategory(s.node.category(state, err), elapsed-state.childTotal)
	s.node.parent.addChild(elapsed)
}

// EndTurn closes a chat_turn span at end, sets its turn_outcome and
// overattributed attributes, counts the outcome, and emits the turn's
// category partition labeled with it. Only a completed turn is
// observed on the stage histogram. Calls after the first are ignored.
func (s *StageSpan) EndTurn(outcome TurnOutcome, err error, end time.Time) {
	if s == nil || s.ended {
		return
	}
	elapsed := end.Sub(s.start)
	categories, overattributed := turnPartition(s.acc, elapsed)
	s.span.SetAttributes(
		attribute.String(AttrTurnOutcome, string(outcome)),
		attribute.Bool(AttrOverattributed, overattributed),
	)
	if model := s.acc.model(); model.Model != "" {
		s.SetModel(model)
	}
	s.closeSpanAt(err, end)
	if outcome == TurnOutcomeCompleted {
		s.tracer.observe(s.stage, s.scope, s.chatKind, s.model, elapsed)
	}
	if overattributed {
		s.tracer.RecordAnomaly(StageAnomalyOverattributed)
	}
	s.tracer.emitTurnAccounting(categories, s.chatKind, outcome)
}

// categorizeRecordedStage adds the duration of a stage built from
// timestamps, limited to the turn's end time, to the turn on ctx, and
// takes it out of the own time of the attributing stage on ctx, if
// any, so the categories stay disjoint.
func categorizeRecordedStage(ctx context.Context, stage Stage, start time.Time, elapsed time.Duration) {
	category, ok := recordedStageCategories[stage]
	acc := turnAccumulatorFromContext(ctx)
	if !ok || acc == nil {
		return
	}
	elapsed = acc.clamp(start, elapsed)
	acc.addCategory(category, elapsed)
	stageNodeFromContext(ctx).addChild(elapsed)
}

// turnPartition returns a turn's category times with unattributed set
// to the remainder of turnDuration, and whether the categories summed
// to more than turnDuration. It returns nil for a non-positive
// turnDuration.
func turnPartition(acc *TurnAccumulator, turnDuration time.Duration) (map[TurnCategory]time.Duration, bool) {
	if turnDuration <= 0 {
		return nil, false
	}
	categories := acc.snapshot()
	var attributed time.Duration
	for _, category := range turnTimeCategories {
		attributed += categories[category]
	}
	unattributed := turnDuration - attributed
	categories[TurnCategoryUnattributed] = max(unattributed, 0)
	return categories, unattributed < 0
}

// emitTurnAccounting records a closed turn's category partition, then
// counts its outcome, so a counted outcome implies its partition is
// recorded. A nil partition is counted as a nonpositive_turn anomaly.
func (t *StageTracer) emitTurnAccounting(categories map[TurnCategory]time.Duration, chatKind ChatKind, outcome TurnOutcome) {
	if t == nil || t.metrics == nil {
		return
	}
	if categories == nil {
		t.RecordAnomaly(StageAnomalyNonPositiveTurn)
	} else {
		for _, category := range turnTimeCategories {
			t.metrics.RecordTurnCategory(category, chatKind, outcome, categories[category])
		}
	}
	t.metrics.RecordTurnOutcome(outcome, chatKind)
}
