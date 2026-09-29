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

// sums totals a counter, or a histogram's sample sums, by label.
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

func (f stageMetricsFixture) startTurn(t *testing.T) (context.Context, *StageSpan, time.Time) {
	t.Helper()
	ctx := ContextWithChatKind(t.Context(), ChatKindRoot)
	ctx = ContextWithTurnAccumulator(ctx, NewTurnAccumulator())
	turnStart := f.clock.Now()
	turnCtx, turnSpan := f.tracer.StartRootAt(ctx, StageChatTurn, turnStart)
	return turnCtx, turnSpan, turnStart
}

func (f stageMetricsFixture) syntheticTurn(t *testing.T, model StageModel) time.Duration {
	t.Helper()
	turnCtx, turnSpan, turnStart := f.startTurn(t)

	f.clock.Advance(2 * time.Second)
	f.tracer.Record(turnCtx, StageAcquisition, StageModel{}, turnStart, f.clock.Now(), nil)

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

	// tool_call is not attributing, so it does not reduce tool_execution.
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

	retriedStepCtx, retriedStep := f.tracer.Start(turnCtx, StageGenerationStep)
	retriedStep.SetGenerationAction("generate_assistant")
	noTokenStreamCtx, noTokenStream := f.tracer.Start(retriedStepCtx, StageStream)
	_, failedTTFT := f.tracer.Start(noTokenStreamCtx, StageTimeToFirstToken)
	f.clock.Advance(2 * time.Second)
	failedTTFT.EndWithoutObservation(xerrors.New("stream ended before the first token"))
	f.clock.Advance(time.Second)
	noTokenStream.End(nil)
	_, backoff := f.tracer.Start(retriedStepCtx, StageRetryBackoff)
	f.clock.Advance(6 * time.Second)
	backoff.End(nil)
	retriedStep.End(nil)

	compactionStepCtx, compactionStep := f.tracer.Start(turnCtx, StageGenerationStep)
	compactionStep.SetGenerationAction("compact")
	_, compaction := f.tracer.Start(compactionStepCtx, StageCompaction)
	f.clock.Advance(8 * time.Second)
	compaction.End(nil)
	compactionStep.End(nil)

	// Time between steps belongs to no stage.
	f.clock.Advance(5 * time.Second)

	turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())
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
		TurnCategoryStreaming:        4 + 1,
		TurnCategoryProviderError:    2,
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
	require.Contains(t, endedSpan(t, fixture.spans, StageChatTurn).Attributes(), attribute.Bool(AttrOverattributed, false))

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
			turnSpan.EndTurn(outcome, err, turnSpan.tracer.Now())

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

// TestTurnAccountingStreamWithoutFirstToken counts the whole stream as
// provider error, once.
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
			turnSpan.EndTurn(outcome, err, turnSpan.tracer.Now())

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
	turnSpan.EndTurn(TurnOutcomeError, err, turnSpan.tracer.Now())

	// A first token arrived, so only the rest of the stream is provider error.
	categories := turnCategories(t, fixture.registry)
	require.InDelta(t, 2, categories[TurnCategoryTimeToFirstToken], 0.001)
	require.InDelta(t, 3, categories[TurnCategoryProviderError], 0.001)
	require.Zero(t, categories[TurnCategoryStreaming])
	require.InDelta(t, fixture.clock.Now().Sub(turnStart).Seconds(), categoryTotal(categories), 0.001)
}

func TestTurnAccountingCanceledStream(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name      string
		firstPart bool
		want      map[TurnCategory]float64
	}{
		{
			name: "BeforeFirstToken",
			want: map[TurnCategory]float64{TurnCategoryTimeToFirstToken: 4},
		},
		{
			name:      "AfterFirstToken",
			firstPart: true,
			want:      map[TurnCategory]float64{TurnCategoryTimeToFirstToken: 2, TurnCategoryStreaming: 2},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newStageMetricsFixture(t)
			turnCtx, turnSpan, turnStart := fixture.startTurn(t)
			streamCtx, cancel := context.WithCancel(turnCtx)
			defer cancel()

			model := &chattest.FakeModel{
				ProviderName: "google",
				ModelName:    "test-model",
				StreamFn: func(ctx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
					return func(yield func(fantasy.StreamPart) bool) {
						if tc.firstPart {
							fixture.clock.Advance(2 * time.Second)
							if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"}) {
								return
							}
							fixture.clock.Advance(2 * time.Second)
						} else {
							fixture.clock.Advance(4 * time.Second)
						}
						cancel()
						yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: ctx.Err()})
					}, nil
				},
			}
			_, err := GenerateAssistant(streamCtx, GenerateAssistantOptions{
				Model:    model,
				Messages: []fantasy.Message{},
				Clock:    fixture.clock,
				Metrics:  NewMetrics(prometheus.NewRegistry()),
				Stages:   fixture.tracer,
			})
			require.ErrorIs(t, err, context.Canceled)
			turnSpan.EndTurn(TurnOutcomeInterrupted, nil, turnSpan.tracer.Now())

			categories := turnCategories(t, fixture.registry)
			for _, category := range []TurnCategory{TurnCategoryTimeToFirstToken, TurnCategoryStreaming, TurnCategoryProviderError} {
				require.InDelta(t, tc.want[category], categories[category], 0.001, category)
			}
			require.InDelta(t, fixture.clock.Now().Sub(turnStart).Seconds(), categoryTotal(categories), 0.001)
		})
	}
}

// TestTurnAccountingNilTracerStream checks a nil-tracer stream does not
// report its first-token window, so all of the time stays compaction.
func TestTurnAccountingNilTracerStream(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, turnStart := fixture.startTurn(t)

	compactionCtx, compaction := fixture.tracer.Start(turnCtx, StageCompaction)
	attempt, err := guardedStream(compactionCtx, "anthropic", "claude", fixture.clock, time.Minute,
		func(context.Context) (fantasy.StreamResponse, error) {
			return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
				fixture.clock.Advance(3 * time.Second)
				yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "summary"})
			}), nil
		},
		NopMetrics(), nil, StageModel{},
	)
	require.NoError(t, err)
	drainStream(attempt.stream)
	fixture.clock.Advance(time.Second)
	attempt.release()
	compaction.End(nil)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())

	categories := turnCategories(t, fixture.registry)
	require.Equal(t, 4.0, categories[TurnCategoryCompaction])
	require.Zero(t, categories[TurnCategoryTimeToFirstToken])
	require.Zero(t, categories[TurnCategoryProviderError])
	require.InDelta(t, fixture.clock.Now().Sub(turnStart).Seconds(), categoryTotal(categories), 0.001)
}

func TestTurnAccountingSchedulingIsAcquisitionOnly(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, turnStart := fixture.startTurn(t)

	// A turn-scoped queue_wait is not categorized.
	fixture.tracer.Record(turnCtx, StageAcquisition, StageModel{},
		turnStart, turnStart.Add(5*time.Second), nil)
	fixture.tracer.Record(turnCtx, StageQueueWait, StageModel{},
		turnStart, turnStart.Add(2*time.Second), nil)
	fixture.clock.Advance(6 * time.Second)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())

	categories := turnCategories(t, fixture.registry)
	require.Equal(t, 5.0, categories[TurnCategoryScheduling])
	require.Equal(t, 1.0, categories[TurnCategoryUnattributed])
	require.Empty(t, stageAnomalies(t, fixture.registry))
}

func TestTurnAccountingRecordedStageUnderStep(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, _ := fixture.startTurn(t)

	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	fixture.clock.Advance(3 * time.Second)
	fixture.tracer.Record(stepCtx, StageAcquisition, StageModel{},
		fixture.clock.Now().Add(-2*time.Second), fixture.clock.Now(), nil)
	step.End(nil)
	turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())

	categories := turnCategories(t, fixture.registry)
	require.Equal(t, 2.0, categories[TurnCategoryScheduling])
	require.Equal(t, 1.0, categories[TurnCategoryChatdOverhead])
	require.Zero(t, categories[TurnCategoryUnattributed])
	require.Empty(t, stageAnomalies(t, fixture.registry))
}

func TestTurnAccountingIgnoresWorkOutsideTheTurn(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, _ := fixture.startTurn(t)
	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	fixture.clock.Advance(time.Second)

	// Background stages must not report to the inherited accumulator.
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
	turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())

	categories := turnCategories(t, fixture.registry)
	require.Zero(t, categories[TurnCategoryStreaming])
	require.Zero(t, categories[TurnCategoryTimeToFirstToken])
	require.Zero(t, categories[TurnCategoryScheduling])
	require.Equal(t, 8.0, categories[TurnCategoryChatdOverhead])
}

func TestTurnAccountingNonAttributingStages(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, _ := fixture.startTurn(t)

	// Overlapping non-attributing stages take no time from the step.
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
	turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())

	categories := turnCategories(t, fixture.registry)
	require.Equal(t, 5.0, categories[TurnCategoryToolExecution])
	require.Equal(t, 5.0, categoryTotal(categories))
	require.Empty(t, stageAnomalies(t, fixture.registry))
}

func TestTurnAccountingConcurrentStageEnds(t *testing.T) {
	t.Parallel()
	fixture := newStageMetricsFixture(t)
	turnCtx, turnSpan, _ := fixture.startTurn(t)

	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	const workers = 16
	toolCalls := make([]*StageSpan, workers)
	for i := range workers {
		_, toolCalls[i] = fixture.tracer.Start(stepCtx, StageToolCall)
	}
	fixture.clock.Advance(2 * time.Second)

	var wg sync.WaitGroup
	for i := range workers {
		wg.Go(func() {
			toolCalls[i].SetModel(StageModel{Model: fmt.Sprintf("model-%d", i)})
			toolCalls[i].End(nil)
		})
	}
	wg.Wait()
	wg.Go(func() { step.End(nil) })
	wg.Go(func() { turnSpan.EndTurn(TurnOutcomeInterrupted, nil, turnSpan.tracer.Now()) })
	wg.Wait()

	// Which category gets the step depends on whether it ends before the
	// turn closes.
	categories := turnCategories(t, fixture.registry)
	require.Equal(t, 2.0, categories[TurnCategoryChatdOverhead]+categories[TurnCategoryUnattributed])
	require.Equal(t, 2.0, categoryTotal(categories))
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
		// Overlapping steps each report their full duration.
		_, first := fixture.tracer.Start(turnCtx, StageGenerationStep)
		_, second := fixture.tracer.Start(turnCtx, StageGenerationStep)
		fixture.clock.Advance(4 * time.Second)
		first.End(nil)
		second.End(nil)
		turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())

		categories := turnCategories(t, fixture.registry)
		require.Equal(t, 8.0, categories[TurnCategoryChatdOverhead], "categories are emitted as measured")
		require.Contains(t, categories, TurnCategoryUnattributed, "the unattributed series is recorded at zero")
		require.Equal(t, 0.0, categories[TurnCategoryUnattributed])
		require.Equal(t, 1.0, stageAnomalies(t, fixture.registry)[StageAnomalyOverattributed])
		require.Contains(t, endedSpan(t, fixture.spans, StageChatTurn).Attributes(), attribute.Bool(AttrOverattributed, true))
	})

	t.Run("NonPositiveTurn", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		_, turnSpan, _ := fixture.startTurn(t)
		turnSpan.EndTurn(TurnOutcomeInterrupted, nil, turnSpan.tracer.Now())

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
	turnSpan.EndTurn(TurnOutcomeCompleted, nil, turnSpan.tracer.Now())

	turn := endedSpan(t, fixture.spans, StageChatTurn)
	require.Contains(t, turn.Attributes(), attribute.String(AttrProviderType, model.ProviderType))
	require.Contains(t, turn.Attributes(), attribute.String(AttrModel, model.Model))
	require.Contains(t, turn.Attributes(), attribute.String(AttrReasoningEffort, model.Effort))

	labels := labelsOf(t, fixture.registry, "coderd_chatd_stage_duration_seconds", "stage", string(StageChatTurn))
	require.NotContains(t, labels, "model", "the model is a span attribute on the turn, not a metric label")
}

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

func TestTurnAccountingClampsToTurnEnd(t *testing.T) {
	t.Parallel()

	t.Run("LiveStages", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindRoot), acc)
		turnStart := fixture.clock.Now()
		turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart)

		stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
		step.SetGenerationAction(GenerationActionExecuteLocalTools)
		fixture.clock.Advance(time.Second)
		_, prepare := fixture.tracer.Start(stepCtx, StagePrepare)
		fixture.clock.Advance(time.Second)
		prepare.End(nil)
		fixture.clock.Advance(3 * time.Second)
		turnEnd := fixture.clock.Now()
		acc.SetEnd(turnEnd)
		// The step is still running and its commit starts after the end.
		fixture.clock.Advance(time.Second)
		_, commit := fixture.tracer.Start(stepCtx, StageCommit)
		fixture.clock.Advance(time.Second)
		commit.End(nil)
		fixture.clock.Advance(3 * time.Second)
		step.End(nil)
		turnSpan.EndTurn(TurnOutcomeInterrupted, nil, turnEnd)

		categories := turnCategories(t, fixture.registry)
		require.Equal(t, 1.0, categories[TurnCategoryPreparation])
		require.Equal(t, 4.0, categories[TurnCategoryToolExecution])
		require.Zero(t, categories[TurnCategoryPersistence])
		require.Zero(t, categories[TurnCategoryUnattributed])
		require.Equal(t, turnEnd.Sub(turnStart).Seconds(), categoryTotal(categories))
		require.Zero(t, stageAnomalies(t, fixture.registry)[StageAnomalyOverattributed])
	})

	t.Run("RecordedStage", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindRoot), acc)
		turnStart := fixture.clock.Now()
		turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart)
		turnEnd := turnStart.Add(time.Second)
		acc.SetEnd(turnEnd)
		fixture.tracer.Record(turnCtx, StageAcquisition, StageModel{}, turnStart, turnStart.Add(3*time.Second), nil)
		turnSpan.EndTurn(TurnOutcomeAbandoned, nil, turnEnd)

		categories := turnCategories(t, fixture.registry)
		require.Equal(t, 1.0, categories[TurnCategoryScheduling])
		require.Equal(t, 1.0, categoryTotal(categories))
	})
}
