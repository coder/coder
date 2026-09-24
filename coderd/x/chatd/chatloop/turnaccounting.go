package chatloop

import (
	"context"
	"sync"
	"time"
)

// Category is a turn time category, the `category` label of the
// turn-end families. The categories partition a turn's wall time:
// every category is disjoint from the others and unattributed carries
// the remainder, so the categories of one turn sum to its duration.
type Category string

// Category values.
const (
	CategoryScheduling       Category = "scheduling"
	CategoryTimeToFirstToken Category = "time_to_first_token"
	CategoryStreaming        Category = "streaming"
	CategoryProviderError    Category = "provider_error"
	CategoryRetryBackoff     Category = "retry_backoff"
	CategoryToolExecution    Category = "tool_execution"
	CategoryCompaction       Category = "compaction"
	CategoryPreparation      Category = "preparation"
	CategoryPersistence      Category = "persistence"
	CategoryChatdOverhead    Category = "chatd_overhead"
	CategoryUnattributed     Category = "unattributed"
)

// TurnTimeCategories lists every category in a fixed order. All of
// them are emitted for each accounted turn, including the ones with no
// time, so a share is comparable across turns.
var TurnTimeCategories = []Category{
	CategoryScheduling,
	CategoryTimeToFirstToken,
	CategoryStreaming,
	CategoryProviderError,
	CategoryRetryBackoff,
	CategoryToolExecution,
	CategoryCompaction,
	CategoryPreparation,
	CategoryPersistence,
	CategoryChatdOverhead,
	CategoryUnattributed,
}

// attributingStages are the stages that take time from the stage they
// run inside and contribute their remaining time to a category. A
// stage outside this set neither claims time from its parent nor lands
// in a category, because such stages overlap the ones that do:
// provider_attempt overlaps time_to_first_token, and thinking and
// tool_call are reconstructed from timestamps inside a stream that is
// already accounted for.
var attributingStages = map[Stage]struct{}{
	StageGenerationStep:   {},
	StagePrepare:          {},
	StageMCPConnect:       {},
	StageCommit:           {},
	StageStream:           {},
	StageTimeToFirstToken: {},
	StageCompaction:       {},
	StageRetryBackoff:     {},
}

// recordedStageCategories maps the stages reconstructed from explicit
// timestamps to their category. They run outside any attributing stage
// and cannot nest, so their full duration is categorized. Recorded
// stages absent from the map are not categorized. capacity_wait is
// absent: it is a sub-window of acquisition, measured
// before the turn exists, and categorizing it would count that window
// twice.
var recordedStageCategories = map[Stage]Category{
	StageAcquisition: CategoryScheduling,
	StageQueueWait:   CategoryScheduling,
}

// turnAccumulatorKey keys the accumulator of the turn a context runs
// in. It is private so the accumulator can only be attached through
// ContextWithTurnAccumulator.
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

// TurnAccumulator sums the category times of one chat turn so they can
// be emitted together when the turn ends, instead of arriving spread
// over the turn as each stage ends.
//
// It is safe for concurrent use: parallel tool calls and the stages
// under them end on different goroutines.
type TurnAccumulator struct {
	mu         sync.Mutex
	categories map[Category]time.Duration
	model      StageModel
	completed  bool
	invalid    bool
}

// NewTurnAccumulator returns an accumulator for one turn. The turn is
// not accounted until MarkCompleted is called, so a turn that never
// reaches its finish transition emits nothing.
func NewTurnAccumulator() *TurnAccumulator {
	return &TurnAccumulator{
		categories: map[Category]time.Duration{},
	}
}

func (a *TurnAccumulator) addCategory(category Category, elapsed time.Duration) {
	if a == nil || category == "" || elapsed <= 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.categories[category] += elapsed
}

// setModel records the model of the turn on first call. Later calls
// are ignored, so a turn that switches models keeps the identity it
// started with rather than the one it happened to end with.
func (a *TurnAccumulator) setModel(model StageModel) {
	if a == nil || model.Model == "" {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.model.Model == "" {
		a.model = model
	}
}

// Model returns the turn's model identity, empty until a stage
// resolves one.
func (a *TurnAccumulator) Model() StageModel {
	if a == nil {
		return StageModel{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.model
}

// MarkCompleted marks the turn as finished normally, which is what
// makes its accounting emittable.
func (a *TurnAccumulator) MarkCompleted() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.completed = true
}

// Invalidate drops the turn's accounting. An errored or interrupted
// turn stops partway through its stages, so its category totals do not
// describe a full turn.
func (a *TurnAccumulator) Invalidate() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.invalid = true
}

// turnAccounting is the emittable state of one turn.
type turnAccounting struct {
	categories map[Category]time.Duration
	emit       bool
}

func (a *TurnAccumulator) snapshot() turnAccounting {
	if a == nil {
		return turnAccounting{}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	snapshot := turnAccounting{
		categories: make(map[Category]time.Duration, len(a.categories)),
		emit:       a.completed && !a.invalid,
	}
	for category, elapsed := range a.categories {
		snapshot.categories[category] = elapsed
	}
	return snapshot
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
	// action is the generation action of a generation_step, which
	// decides whether its own time is tool execution or overhead.
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

// category returns the category that the stage's own time belongs to.
// An unknown stage returns an empty category, which drops its time
// into the turn's unattributed remainder. preparation covers the
// prepare phase including MCP connects, persistence covers the commit
// transaction, and chatd_overhead is the step's own time outside every
// child stage.
func (n *stageNode) category(state nodeState, err error) Category {
	switch n.stage {
	case StageGenerationStep:
		if state.action == GenerationActionExecuteLocalTools {
			return CategoryToolExecution
		}
		return CategoryChatdOverhead
	case StagePrepare, StageMCPConnect:
		return CategoryPreparation
	case StageCommit:
		return CategoryPersistence
	case StageCompaction:
		return CategoryCompaction
	case StageRetryBackoff:
		return CategoryRetryBackoff
	case StageStream:
		if state.failed || err != nil {
			return CategoryProviderError
		}
		return CategoryStreaming
	case StageTimeToFirstToken:
		if err != nil {
			return CategoryProviderError
		}
		return CategoryTimeToFirstToken
	default:
		return ""
	}
}

// report folds a finished stage into the turn it ran in. elapsed is
// the stage's full duration; the category receives only the part not
// spent inside a nested attributing stage.
func (s *StageSpan) report(elapsed time.Duration, err error) {
	if s.acc == nil {
		return
	}
	if s.stage == StageChatTurn {
		s.tracer.emitTurnAccounting(s.acc, s.chatKind, elapsed)
		return
	}
	if s.node == nil {
		return
	}
	state := s.node.state()
	own := elapsed - state.childTotal
	if own < 0 {
		own = 0
	}
	s.acc.addCategory(s.node.category(state, err), own)
	if s.stage == StageTimeToFirstToken && err != nil {
		s.node.parent.markFailed()
	}
	s.node.parent.addChild(elapsed)
}

// RecordTurnOutcome counts a closed turn under outcome. Completed
// turns are counted by emitTurnAccounting when their partition is
// recorded; this is for interrupted, error, and abandoned turns. It is
// safe to call on a nil tracer.
func (t *StageTracer) RecordTurnOutcome(outcome TurnOutcome, chatKind ChatKind) {
	if t == nil || t.metrics == nil {
		return
	}
	t.metrics.RecordTurnOutcome(outcome, chatKind)
}

// recordAttribution folds a stage reconstructed from timestamps into
// the turn on ctx.
func recordAttribution(ctx context.Context, stage Stage, elapsed time.Duration) {
	acc := turnAccumulatorFromContext(ctx)
	if acc == nil {
		return
	}
	acc.addCategory(recordedStageCategories[stage], elapsed)
}

// emitTurnAccounting records the category partition of one turn.
// turnDuration is the turn's own wall time, and the categories that
// did not add up to it become the unattributed remainder. Categories
// that add up to more than the turn are emitted as measured, with no
// unattributed time, and counted as an anomaly.
func (t *StageTracer) emitTurnAccounting(acc *TurnAccumulator, chatKind ChatKind, turnDuration time.Duration) {
	if t == nil || t.metrics == nil {
		return
	}
	snapshot := acc.snapshot()
	if !snapshot.emit {
		return
	}
	// The outcome is counted before the duration check so a completed
	// turn is counted even when its partition is dropped.
	t.metrics.RecordTurnOutcome(TurnOutcomeCompleted, chatKind)
	if turnDuration <= 0 {
		t.RecordAnomaly(StageAnomalyNonPositiveTurn)
		return
	}
	var attributed time.Duration
	for _, category := range TurnTimeCategories {
		attributed += snapshot.categories[category]
	}
	unattributed := turnDuration - attributed
	if unattributed < 0 {
		unattributed = 0
		t.RecordAnomaly(StageAnomalyOverattributed)
	}
	snapshot.categories[CategoryUnattributed] = unattributed
	t.metrics.RecordTurn(chatKind)
	for _, category := range TurnTimeCategories {
		t.metrics.RecordTurnCategory(category, chatKind, snapshot.categories[category])
	}
}
