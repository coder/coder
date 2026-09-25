package chatloop

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
)

// sums returns the value of each series of a counter family, or the
// sample sum of each series of a histogram family, keyed by the value
// of the named label and summed across the other labels.
func sums[K ~string](t *testing.T, registry *prometheus.Registry, family, label string) map[K]float64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	out := map[K]float64{}
	for _, metricFamily := range families {
		if metricFamily.GetName() != family {
			continue
		}
		for _, metric := range metricFamily.GetMetric() {
			value := metric.GetCounter().GetValue()
			if metric.GetHistogram() != nil {
				value = metric.GetHistogram().GetSampleSum()
			}
			out[K(metricLabel(metric, label))] += value
		}
	}
	return out
}

func turnCategories(t *testing.T, registry *prometheus.Registry) map[TurnCategory]float64 {
	t.Helper()
	return sums[TurnCategory](t, registry, "coderd_chatd_turn_time_seconds_total", "category")
}

func turnOutcomes(t *testing.T, registry *prometheus.Registry) map[TurnOutcome]float64 {
	t.Helper()
	return sums[TurnOutcome](t, registry, "coderd_chatd_turn_outcomes_total", "outcome")
}

func stageAnomalies(t *testing.T, registry *prometheus.Registry) map[StageAnomaly]float64 {
	t.Helper()
	return sums[StageAnomaly](t, registry, "coderd_chatd_stage_anomalies_total", "reason")
}

func categoryTotal(categories map[TurnCategory]float64) float64 {
	var total float64
	for _, seconds := range categories {
		total += seconds
	}
	return total
}

func labelsOf(t *testing.T, registry *prometheus.Registry, family, label, value string) map[string]string {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, metricFamily := range families {
		if metricFamily.GetName() != family {
			continue
		}
		for _, metric := range metricFamily.GetMetric() {
			if metricLabel(metric, label) != value {
				continue
			}
			labels := map[string]string{}
			for _, pair := range metric.GetLabel() {
				labels[pair.GetName()] = pair.GetValue()
			}
			return labels
		}
	}
	return nil
}

// startTurn opens a root chat_turn span at the fixture's current time
// with a fresh accumulator on its context.
func (f stageMetricsFixture) startTurn(t *testing.T) (context.Context, *StageSpan, time.Time) {
	t.Helper()
	ctx := ContextWithChatKind(t.Context(), ChatKindRoot)
	ctx = ContextWithOrganization(ctx, "acme")
	ctx = ContextWithTurnAccumulator(ctx, NewTurnAccumulator())
	turnStart := f.clock.Now()
	turnCtx, turnSpan := f.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)
	return turnCtx, turnSpan, turnStart
}

// syntheticTurn runs one completed turn that exercises every category
// and returns its duration.
func (f stageMetricsFixture) syntheticTurn(t *testing.T, model StageModel) time.Duration {
	t.Helper()
	turnCtx, turnSpan, turnStart := f.startTurn(t)

	f.clock.Advance(2 * time.Second)
	f.tracer.Record(turnCtx, StageAcquisition, StageModel{}, turnStart, f.clock.Now(), nil)

	// A generate_assistant step; its one second after the commit is
	// chatd_overhead.
	stepCtx, step := f.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	step.SetModel(model)
	prepareCtx, prepare := f.tracer.Start(stepCtx, StagePrepare)
	_, mcp := f.tracer.Start(prepareCtx, StageMCPConnect)
	f.clock.Advance(time.Second)
	mcp.End(nil)
	f.clock.Advance(time.Second)
	prepare.End(nil)
	streamCtx, stream := f.tracer.Start(stepCtx, StageStream)
	_, ttft := f.tracer.Start(streamCtx, StageTimeToFirstToken)
	f.clock.Advance(3 * time.Second)
	ttft.End(nil)
	f.clock.Advance(4 * time.Second)
	stream.End(nil)
	_, commit := f.tracer.Start(stepCtx, StageCommit)
	f.clock.Advance(time.Second)
	commit.End(nil)
	f.clock.Advance(time.Second)
	step.End(nil)

	// An execute_local_tools step: the time between its prepare and its
	// commit is the tool execution, which the tool_call stage inside it
	// does not change.
	toolStepCtx, toolStep := f.tracer.Start(turnCtx, StageGenerationStep)
	toolStep.SetGenerationAction(GenerationActionExecuteLocalTools)
	_, toolPrepare := f.tracer.Start(toolStepCtx, StagePrepare)
	f.clock.Advance(time.Second)
	toolPrepare.End(nil)
	_, toolCall := f.tracer.Start(toolStepCtx, StageToolCall)
	f.clock.Advance(5 * time.Second)
	toolCall.End(nil)
	_, toolCommit := f.tracer.Start(toolStepCtx, StageCommit)
	f.clock.Advance(time.Second)
	toolCommit.End(nil)
	toolStep.End(nil)

	// A failed step: the stream never produced a first part, and the
	// retry delay that follows it is its own category. The stream ends
	// without an error, so only its failed first-token window makes the
	// rest of it provider error.
	failedStepCtx, failedStep := f.tracer.Start(turnCtx, StageGenerationStep)
	failedStep.SetGenerationAction("generate_assistant")
	failedStreamCtx, failedStream := f.tracer.Start(failedStepCtx, StageStream)
	_, failedTTFT := f.tracer.Start(failedStreamCtx, StageTimeToFirstToken)
	f.clock.Advance(2 * time.Second)
	failedTTFT.EndWithoutObservation(xerrors.New("stream ended before the first token"))
	f.clock.Advance(time.Second)
	failedStream.End(nil)
	_, backoff := f.tracer.Start(failedStepCtx, StageRetryBackoff)
	f.clock.Advance(6 * time.Second)
	backoff.End(nil)
	failedStep.End(nil)

	compactionStepCtx, compactionStep := f.tracer.Start(turnCtx, StageGenerationStep)
	compactionStep.SetGenerationAction("compact")
	_, compaction := f.tracer.Start(compactionStepCtx, StageCompaction)
	f.clock.Advance(8 * time.Second)
	compaction.End(nil)
	compactionStep.End(nil)

	// Time between steps belongs to no stage.
	f.clock.Advance(5 * time.Second)

	turnSpan.EndTurn(TurnOutcomeCompleted, nil)
	return f.clock.Now().Sub(turnStart)
}

func TestTurnAccountingPartition(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	model := StageModel{Model: "claude-sonnet-4-5", Effort: "high"}

	turnDuration := fixture.syntheticTurn(t, model)
	require.Equal(t, 42*time.Second, turnDuration)

	categories := turnCategories(t, fixture.registry)
	require.Equal(t, map[TurnCategory]float64{
		TurnCategoryScheduling:       2,
		TurnCategoryTimeToFirstToken: 3,
		TurnCategoryStreaming:        4,
		TurnCategoryProviderError:    3,
		TurnCategoryRetryBackoff:     6,
		TurnCategoryToolExecution:    5,
		TurnCategoryCompaction:       8,
		TurnCategoryPreparation:      2 + 1,
		TurnCategoryPersistence:      1 + 1,
		TurnCategoryChatdOverhead:    1,
		TurnCategoryUnattributed:     5,
	}, categories)
	require.InDelta(t, turnDuration.Seconds(), categoryTotal(categories), 0.001)
	stageTurn := sums[Stage](t, fixture.registry, "coderd_chatd_stage_duration_seconds", "stage")[StageChatTurn]
	require.InDelta(t, turnDuration.Seconds(), stageTurn, 0.001)
	require.Equal(t, map[TurnOutcome]float64{TurnOutcomeCompleted: 1}, turnOutcomes(t, fixture.registry))
	require.Empty(t, stageAnomalies(t, fixture.registry))

	labels := labelsOf(t, fixture.registry, "coderd_chatd_turn_time_seconds_total", "category", string(TurnCategoryStreaming))
	require.Equal(t, map[string]string{
		"category":  string(TurnCategoryStreaming),
		"chat_kind": string(ChatKindRoot),
		"outcome":   string(TurnOutcomeCompleted),
	}, labels)
}

func TestTurnAccountingEmitsEveryOutcome(t *testing.T) {
	t.Parallel()

	for _, outcome := range []TurnOutcome{
		TurnOutcomeCompleted,
		TurnOutcomeInterrupted,
		TurnOutcomeError,
		TurnOutcomeAbandoned,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			fixture := newStageMetricsFixture(t)
			turnCtx, turnSpan, _ := fixture.startTurn(t)
			stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
			step.SetGenerationAction("generate_assistant")
			_, stream := fixture.tracer.Start(stepCtx, StageStream)
			fixture.clock.Advance(3 * time.Second)
			stream.End(nil)
			step.End(nil)
			fixture.clock.Advance(time.Second)
			var err error
			if outcome != TurnOutcomeCompleted {
				err = xerrors.New("turn stopped")
			}
			turnSpan.EndTurn(outcome, err)

			require.Equal(t, map[TurnOutcome]float64{outcome: 1}, turnOutcomes(t, fixture.registry))
			categories := turnCategories(t, fixture.registry)
			require.Len(t, categories, len(turnTimeCategories))
			require.Equal(t, 3.0, categories[TurnCategoryStreaming])
			require.Equal(t, 1.0, categories[TurnCategoryUnattributed])
			labels := labelsOf(t, fixture.registry, "coderd_chatd_turn_time_seconds_total", "category", string(TurnCategoryStreaming))
			require.Equal(t, string(outcome), labels["outcome"])

			turn := endedSpan(t, fixture.spans, StageChatTurn)
			require.Contains(t, turn.Attributes(), attribute.String(AttrTurnOutcome, string(outcome)))
		})
	}
}

// TestTurnAccountingStreamWithoutFirstToken covers streams whose
// first-token window closes with an error: at the attempt's release
// when no part arrived, or at an error part. Either way the whole
// stream is provider error time, counted once.
func TestTurnAccountingStreamWithoutFirstToken(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		parts   []fantasy.StreamPart
		wantErr bool
	}{
		{name: "NoParts"},
		{
			name:    "ErrorPart",
			parts:   []fantasy.StreamPart{{Type: fantasy.StreamPartTypeError, Error: xerrors.New("provider returned 500")}},
			wantErr: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newStageMetricsFixture(t)
			turnCtx, turnSpan, turnStart := fixture.startTurn(t)

			model := &chattest.FakeModel{
				ProviderName: "google",
				ModelName:    "test-model",
				StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
					fixture.clock.Advance(2 * time.Second)
					return streamFromParts(tc.parts), nil
				},
			}
			_, err := GenerateAssistant(turnCtx, GenerateAssistantOptions{
				Model:    model,
				Messages: []fantasy.Message{},
				Clock:    fixture.clock,
				Metrics:  NewMetrics(prometheus.NewRegistry()),
				Stages:   fixture.tracer,
			})
			outcome := TurnOutcomeCompleted
			if tc.wantErr {
				require.Error(t, err)
				outcome = TurnOutcomeError
			} else {
				require.NoError(t, err)
			}
			turnSpan.EndTurn(outcome, err)

			categories := turnCategories(t, fixture.registry)
			require.InDelta(t, 2, categories[TurnCategoryProviderError], 0.001)
			require.Zero(t, categories[TurnCategoryStreaming])
			require.Zero(t, categories[TurnCategoryTimeToFirstToken])
			require.InDelta(t, fixture.clock.Now().Sub(turnStart).Seconds(), categoryTotal(categories), 0.001)
		})
	}
}

func TestTurnAccountingStreamFailsAfterFirstToken(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, turnStart := fixture.startTurn(t)

	streamErr := xerrors.New("connection reset")
	model := &chattest.FakeModel{
		ProviderName: "google",
		ModelName:    "test-model",
		StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			return func(yield func(fantasy.StreamPart) bool) {
				fixture.clock.Advance(2 * time.Second)
				if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"}) {
					return
				}
				fixture.clock.Advance(3 * time.Second)
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: streamErr})
			}, nil
		},
	}
	_, err := GenerateAssistant(turnCtx, GenerateAssistantOptions{
		Model:    model,
		Messages: []fantasy.Message{},
		Clock:    fixture.clock,
		Metrics:  NewMetrics(prometheus.NewRegistry()),
		Stages:   fixture.tracer,
	})
	require.Error(t, err)
	turnSpan.EndTurn(TurnOutcomeError, err)

	// The first-token window closed with an output part; the rest of the
	// failed stream is provider error.
	categories := turnCategories(t, fixture.registry)
	require.InDelta(t, 2, categories[TurnCategoryTimeToFirstToken], 0.001)
	require.InDelta(t, 3, categories[TurnCategoryProviderError], 0.001)
	require.Zero(t, categories[TurnCategoryStreaming])
	require.InDelta(t, fixture.clock.Now().Sub(turnStart).Seconds(), categoryTotal(categories), 0.001)
}

func TestTurnAccountingSchedulingIsAcquisitionOnly(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, turnStart := fixture.startTurn(t)

	// capacity_wait lies inside acquisition, and a turn-scoped
	// queue_wait is not categorized.
	fixture.tracer.Record(turnCtx, StageAcquisition, StageModel{},
		turnStart, turnStart.Add(5*time.Second), nil)
	fixture.tracer.Record(turnCtx, StageCapacityWait, StageModel{},
		turnStart.Add(time.Second), turnStart.Add(4*time.Second), nil)
	fixture.tracer.Record(turnCtx, StageQueueWait, StageModel{},
		turnStart, turnStart.Add(2*time.Second), nil)
	fixture.clock.Advance(6 * time.Second)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil)

	categories := turnCategories(t, fixture.registry)
	require.Equal(t, 5.0, categories[TurnCategoryScheduling])
	require.Equal(t, 1.0, categories[TurnCategoryUnattributed])
	require.Empty(t, stageAnomalies(t, fixture.registry))
}

func TestTurnAccountingIgnoresWorkOutsideTheTurn(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, _ := fixture.startTurn(t)
	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	fixture.clock.Advance(time.Second)

	// Detached work derives its context from the step's but runs in the
	// background scope, so the accumulator it inherits must not receive
	// its stages.
	backgroundCtx := ContextWithScope(stepCtx, ScopeBackground)
	bgCtx, bgStream := fixture.tracer.Start(backgroundCtx, StageStream)
	_, bgTTFT := fixture.tracer.Start(bgCtx, StageTimeToFirstToken)
	fixture.clock.Advance(3 * time.Second)
	bgTTFT.End(nil)
	fixture.clock.Advance(4 * time.Second)
	bgStream.End(nil)
	fixture.tracer.Record(backgroundCtx, StageAcquisition, StageModel{},
		fixture.clock.Now().Add(-time.Second), fixture.clock.Now(), nil)

	step.End(nil)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil)

	categories := turnCategories(t, fixture.registry)
	require.Zero(t, categories[TurnCategoryStreaming])
	require.Zero(t, categories[TurnCategoryTimeToFirstToken])
	require.Zero(t, categories[TurnCategoryScheduling])
	// The step's own time is intact: the background stream did not
	// report itself as the step's child.
	require.Equal(t, 8.0, categories[TurnCategoryChatdOverhead])
}

func TestTurnAccountingNonAttributingStages(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, _ := fixture.startTurn(t)

	// Parallel tool calls overlap each other and the step that runs
	// them, and a provider attempt runs under one of them. None of them
	// takes time from the step.
	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction(GenerationActionExecuteLocalTools)
	firstCtx, first := fixture.tracer.Start(stepCtx, StageToolCall)
	_, second := fixture.tracer.Start(stepCtx, StageToolCall)
	_, attempt := fixture.tracer.Start(firstCtx, StageProviderAttempt)
	fixture.clock.Advance(4 * time.Second)
	attempt.End(nil)
	first.End(nil)
	second.End(nil)
	fixture.clock.Advance(time.Second)
	step.End(nil)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil)

	categories := turnCategories(t, fixture.registry)
	require.Equal(t, 5.0, categories[TurnCategoryToolExecution])
	require.Equal(t, 5.0, categoryTotal(categories))
	require.Empty(t, stageAnomalies(t, fixture.registry))
}

func TestTurnAccountingConcurrentStageEnds(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, _ := fixture.startTurn(t)

	// Parallel mcp_connect stages end, and provider attempts set the
	// turn's model, on separate goroutines.
	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	prepareCtx, prepare := fixture.tracer.Start(stepCtx, StagePrepare)
	const workers = 16
	connects := make([]*StageSpan, workers)
	attempts := make([]*StageSpan, workers)
	for i := range workers {
		_, connects[i] = fixture.tracer.Start(prepareCtx, StageMCPConnect)
		_, attempts[i] = fixture.tracer.Start(prepareCtx, StageProviderAttempt)
	}
	fixture.clock.Advance(time.Second)

	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			attempts[i].SetModel(StageModel{Model: fmt.Sprintf("model-%d", i)})
			attempts[i].End(nil)
			connects[i].End(nil)
		})
	}
	wg.Wait()
	prepare.End(nil)
	step.End(nil)
	// The connects overlap, so the turn runs long enough for their
	// summed time to fit inside it.
	fixture.clock.Advance((workers - 1) * time.Second)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil)

	// A lost update under the concurrent ends would show as less
	// preparation and a positive unattributed remainder.
	categories := turnCategories(t, fixture.registry)
	require.Equal(t, float64(workers), categories[TurnCategoryPreparation])
	require.Zero(t, categories[TurnCategoryUnattributed])
	require.Equal(t, float64(workers), categoryTotal(categories))
	require.Empty(t, stageAnomalies(t, fixture.registry))
	var turnModel string
	for _, attr := range endedSpan(t, fixture.spans, StageChatTurn).Attributes() {
		if attr.Key == AttrModel {
			turnModel = attr.Value.AsString()
		}
	}
	require.Regexp(t, `^model-\d+$`, turnModel)
}

func TestTurnAccountingAnomalies(t *testing.T) {
	t.Parallel()

	t.Run("Overattributed", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		turnCtx, turnSpan, _ := fixture.startTurn(t)
		// Two steps overlapping in wall time each report their full
		// duration, so the categories exceed the turn.
		_, first := fixture.tracer.Start(turnCtx, StageGenerationStep)
		_, second := fixture.tracer.Start(turnCtx, StageGenerationStep)
		fixture.clock.Advance(4 * time.Second)
		first.End(nil)
		second.End(nil)
		turnSpan.EndTurn(TurnOutcomeCompleted, nil)

		categories := turnCategories(t, fixture.registry)
		require.Equal(t, 8.0, categories[TurnCategoryChatdOverhead], "categories are emitted as measured")
		require.Contains(t, categories, TurnCategoryUnattributed, "the unattributed series is recorded at zero")
		require.Equal(t, 0.0, categories[TurnCategoryUnattributed])
		require.Equal(t, 1.0, stageAnomalies(t, fixture.registry)[StageAnomalyOverattributed])
	})

	t.Run("NonPositiveTurn", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		_, turnSpan, _ := fixture.startTurn(t)
		turnSpan.EndTurn(TurnOutcomeInterrupted, nil)

		require.Empty(t, turnCategories(t, fixture.registry))
		require.Equal(t, map[TurnOutcome]float64{TurnOutcomeInterrupted: 1}, turnOutcomes(t, fixture.registry))
		require.Equal(t, 1.0, stageAnomalies(t, fixture.registry)[StageAnomalyNonPositiveTurn])
	})
}

func TestTurnAccountingStampsModelOnRoot(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	model := StageModel{ProviderType: "openai", Model: "gpt-5", Effort: "medium"}

	turnCtx, turnSpan, _ := fixture.startTurn(t)
	_, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetModel(model)
	fixture.clock.Advance(time.Second)
	step.End(nil)
	// A later step on another model does not rename the turn.
	_, second := fixture.tracer.Start(turnCtx, StageGenerationStep)
	second.SetModel(StageModel{Model: "gpt-5-mini"})
	fixture.clock.Advance(time.Second)
	second.End(nil)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil)

	turn := endedSpan(t, fixture.spans, StageChatTurn)
	require.Contains(t, turn.Attributes(), attribute.String(AttrProviderType, model.ProviderType))
	require.Contains(t, turn.Attributes(), attribute.String(AttrModel, model.Model))
	require.Contains(t, turn.Attributes(), attribute.String(AttrReasoningEffort, model.Effort))

	labels := labelsOf(t, fixture.registry, "coderd_chatd_stage_duration_seconds", "stage", string(StageChatTurn))
	require.NotContains(t, labels, "model", "the model is a span attribute on the turn, not a metric label")
}

// TestTurnMetricsEnabled covers that the turn families follow the stage
// metrics option and that the recorders accept calls either way.
func TestTurnMetricsEnabled(t *testing.T) {
	t.Parallel()

	turnFamilies := []string{
		"coderd_chatd_turn_outcomes_total",
		"coderd_chatd_turn_time_seconds_total",
	}
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			metrics := NewMetricsWithOptions(registry, MetricsOptions{StageMetrics: enabled})
			metrics.RecordTurnOutcome(TurnOutcomeCompleted, ChatKindRoot)
			metrics.RecordTurnOutcome(TurnOutcomeError, ChatKindRoot)
			metrics.RecordTurnCategory(TurnCategoryStreaming, ChatKindRoot, TurnOutcomeCompleted, time.Minute)
			metrics.RecordTurnCategory(TurnCategoryScheduling, ChatKindRoot, TurnOutcomeCompleted, 0)

			families, err := registry.Gather()
			require.NoError(t, err)
			var got []string
			for _, family := range families {
				if slices.Contains(turnFamilies, family.GetName()) {
					got = append(got, family.GetName())
				}
			}
			slices.Sort(got)
			if !enabled {
				require.Empty(t, got)
				return
			}
			require.Equal(t, turnFamilies, got)
			require.Equal(t, map[TurnCategory]float64{TurnCategoryStreaming: 60, TurnCategoryScheduling: 0}, turnCategories(t, registry))
			require.Equal(t, map[TurnOutcome]float64{TurnOutcomeCompleted: 1, TurnOutcomeError: 1}, turnOutcomes(t, registry))
		})
	}
}
