package chatloop

import (
	"context"
	"errors"
	"maps"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// TurnCategory is a category of a turn's time partition.
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

// attributingStages count their own time (duration minus attributing
// children) in the mapped category. provider_attempt, thinking, and
// tool_call are absent because they overlap stages listed here.
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

// recordedStageCategories count their full duration; they have no
// attributing children.
var recordedStageCategories = map[Stage]TurnCategory{
	StageAcquisition: TurnCategoryScheduling,
}

type turnAccumulatorKey struct{}

// stageNodeKey keys the innermost attributing stage.
type stageNodeKey struct{}

// ContextWithTurnAccumulator returns ctx whose stages report to acc.
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

// TurnAccumulator sums one turn's category times. It is safe for
// concurrent use.
type TurnAccumulator struct {
	mu         sync.Mutex
	categories map[TurnCategory]time.Duration
	stageModel StageModel
	endAt      time.Time
}

// NewTurnAccumulator returns an empty TurnAccumulator.
func NewTurnAccumulator() *TurnAccumulator {
	return &TurnAccumulator{
		categories: map[TurnCategory]time.Duration{},
	}
}

// SetEnd fixes the turn's end time; stages reported later count only
// the part of their window before end.
func (a *TurnAccumulator) SetEnd(end time.Time) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.endAt = end
}

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

// stageNode tracks time spent in attributing children so the parent's
// own category excludes it.
type stageNode struct {
	stage  Stage
	parent *stageNode

	mu         sync.Mutex
	childTotal time.Duration
	action     string
}

func (n *stageNode) setAction(action string) {
	if n == nil {
		return
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	n.action = action
}

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

type nodeState struct {
	childTotal time.Duration
	action     string
}

func (n *stageNode) category(state nodeState, err error) TurnCategory {
	switch {
	case n.stage == StageGenerationStep && state.action == GenerationActionExecuteLocalTools:
		return TurnCategoryToolExecution
	// Cancellation is not a provider error.
	case (n.stage == StageStream || n.stage == StageTimeToFirstToken) && err != nil && !errors.Is(err, context.Canceled):
		return TurnCategoryProviderError
	default:
		return attributingStages[n.stage]
	}
}

// report clamps to the turn's end before splitting own time from the
// duration passed to the parent.
func (s *StageSpan) report(elapsed time.Duration, err error) {
	if s.acc == nil || s.node == nil {
		return
	}
	elapsed = s.acc.clamp(s.start, elapsed)
	state := s.node.state()
	s.acc.addCategory(s.node.category(state, err), elapsed-state.childTotal)
	s.node.parent.addChild(elapsed)
}

// EndTurn closes a chat_turn span at end and emits its time partition.
// Only completed turns are observed on the histogram.
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

// turnPartition reports whether the categories exceed turnDuration;
// unattributed is then zero rather than negative.
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

// emitTurnAccounting records the partition before the outcome, so a
// counted outcome implies a recorded partition.
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
