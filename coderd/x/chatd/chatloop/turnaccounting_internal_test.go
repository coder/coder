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
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/quartz"
)

// turnFixture drives a synthetic turn on a mock clock, so every stage
// window is an exact duration.
type turnFixture struct {
	tracer   *StageTracer
	clock    *quartz.Mock
	spans    *tracetest.SpanRecorder
	registry *prometheus.Registry
}

func newTurnFixture(t *testing.T) turnFixture {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	registry := prometheus.NewRegistry()
	clock := quartz.NewMock(t)
	tracer := NewStageTracer(provider, NewMetrics(registry))
	tracer.clock = clock
	return turnFixture{tracer: tracer, clock: clock, spans: recorder, registry: registry}
}

// sums returns the value of each series of a counter family, or the
// sample sum of each series of a histogram family, keyed by the value
// of the named label.
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
			out[K(metricLabel(metric, label))] = value
		}
	}
	return out
}

// categorySums returns the per-category values of a family.
func (f turnFixture) categorySums(t *testing.T, family string) map[Category]float64 {
	t.Helper()
	return sums[Category](t, f.registry, family, "category")
}

// stageSums returns the per-stage values of a family.
func (f turnFixture) stageSums(t *testing.T, family string) map[Stage]float64 {
	t.Helper()
	return sums[Stage](t, f.registry, family, "stage")
}

func (f turnFixture) labelsOf(t *testing.T, family, label, value string) map[string]string {
	t.Helper()
	families, err := f.registry.Gather()
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

// syntheticTurn runs one turn made of the stages the categories are
// built from and returns the turn's accumulator and total duration.
func (f turnFixture) syntheticTurn(t *testing.T, model StageModel) (*TurnAccumulator, time.Duration) {
	t.Helper()
	acc := NewTurnAccumulator()
	ctx := ContextWithChatKind(t.Context(), ChatKindRoot)
	ctx = ContextWithOrganization(ctx, "acme")
	ctx = ContextWithTurnAccumulator(ctx, acc)
	turnStart := f.clock.Now()
	turnCtx, turnSpan := f.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)

	// Scheduling: a queue wait reconstructed from the queued row.
	f.tracer.RecordAs(turnCtx, StageQueueWait, ScopeTurn, StageModel{},
		turnStart.Add(-2*time.Second), turnStart, nil)

	// A generate_assistant step: prepare with an MCP connect, then a
	// stream that produced its first part, then the commit, then a
	// second of the step's own work after the commit.
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

	// An execute_local_tools step: the time between its prepare and
	// its commit is the tool execution.
	toolStepCtx, toolStep := f.tracer.Start(turnCtx, StageGenerationStep)
	toolStep.SetGenerationAction(GenerationActionExecuteLocalTools)
	_, toolPrepare := f.tracer.Start(toolStepCtx, StagePrepare)
	f.clock.Advance(time.Second)
	toolPrepare.End(nil)
	f.clock.Advance(5 * time.Second)
	_, toolCommit := f.tracer.Start(toolStepCtx, StageCommit)
	f.clock.Advance(time.Second)
	toolCommit.End(nil)
	toolStep.End(nil)

	// A failed step: the stream never produced a first part, and the
	// retry delay that follows it is its own category.
	failedStepCtx, failedStep := f.tracer.Start(turnCtx, StageGenerationStep)
	failedStep.SetGenerationAction("generate_assistant")
	failedStreamCtx, failedStream := f.tracer.Start(failedStepCtx, StageStream)
	_, failedTTFT := f.tracer.Start(failedStreamCtx, StageTimeToFirstToken)
	f.clock.Advance(2 * time.Second)
	failedTTFT.EndWithoutObservation(xerrors.New("stream ended before the first token"))
	f.clock.Advance(time.Second)
	failedStream.End(xerrors.New("provider unavailable"))
	_, backoff := f.tracer.Start(failedStepCtx, StageRetryBackoff)
	f.clock.Advance(6 * time.Second)
	backoff.End(nil)
	failedStep.End(nil)

	// A compaction step.
	compactionStepCtx, compactionStep := f.tracer.Start(turnCtx, StageGenerationStep)
	compactionStep.SetGenerationAction("compact")
	_, compaction := f.tracer.Start(compactionStepCtx, StageCompaction)
	f.clock.Advance(8 * time.Second)
	compaction.End(nil)
	compactionStep.End(nil)

	// Time between steps belongs to no stage.
	f.clock.Advance(5 * time.Second)

	acc.MarkCompleted()
	turnSpan.End(nil)
	return acc, f.clock.Now().Sub(turnStart)
}

func TestTurnAccountingPartition(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)
	model := StageModel{Model: "claude-sonnet-4-5", Effort: "high"}

	_, turnDuration := fixture.syntheticTurn(t, model)
	require.Equal(t, 40*time.Second, turnDuration)

	categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
	require.Equal(t, map[Category]float64{
		CategoryScheduling:       2,
		CategoryTimeToFirstToken: 3,
		CategoryStreaming:        4,
		CategoryProviderError:    3,
		CategoryRetryBackoff:     6,
		CategoryToolExecution:    5,
		CategoryCompaction:       8,
		CategoryPreparation:      2 + 1,
		CategoryPersistence:      1 + 1,
		CategoryChatdOverhead:    1,
		CategoryUnattributed:     3,
	}, categories)

	var total float64
	for _, seconds := range categories {
		total += seconds
	}
	require.InDelta(t, turnDuration.Seconds(), total, 0.001,
		"the categories must partition the turn")

	// The turn families carry chat_kind and no model, effort, or
	// organization label.
	labels := fixture.labelsOf(t, "coderd_chatd_turn_time_seconds_total", "category", string(CategoryStreaming))
	require.Equal(t, map[string]string{"category": string(CategoryStreaming), "chat_kind": string(ChatKindRoot)}, labels)
}

func TestTurnAccountingStreamWithoutFirstToken(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)

	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindRoot), acc)
	turnStart := fixture.clock.Now()
	turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)

	model := &chattest.FakeModel{
		ProviderName: "google",
		ModelName:    "test-model",
		StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			fixture.clock.Advance(2 * time.Second)
			return streamFromParts(nil), nil
		},
	}
	_, err := GenerateAssistant(turnCtx, GenerateAssistantOptions{
		Model:    model,
		Messages: []fantasy.Message{},
		Clock:    fixture.clock,
		Metrics:  NewMetrics(prometheus.NewRegistry()),
		Stages:   fixture.tracer,
	})
	require.NoError(t, err)

	acc.MarkCompleted()
	turnSpan.End(nil)

	// The stream never produced a part, so its whole window is provider
	// error time, counted once even though the window closes with the
	// attempt rather than with the stream.
	categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
	require.InDelta(t, 2, categories[CategoryProviderError], 0.001)
	require.Zero(t, categories[CategoryStreaming])
	require.Zero(t, categories[CategoryTimeToFirstToken])

	var total float64
	for _, seconds := range categories {
		total += seconds
	}
	require.InDelta(t, fixture.clock.Now().Sub(turnStart).Seconds(), total, 0.001)
}

func TestTurnAccountingStreamErrorPart(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)

	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindRoot), acc)
	turnStart := fixture.clock.Now()
	turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)

	streamErr := xerrors.New("provider returned 500")
	model := &chattest.FakeModel{
		ProviderName: "google",
		ModelName:    "test-model",
		StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			fixture.clock.Advance(2 * time.Second)
			return streamFromParts([]fantasy.StreamPart{{Type: fantasy.StreamPartTypeError, Error: streamErr}}), nil
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

	acc.MarkCompleted()
	turnSpan.End(nil)

	// An error part closes the first-token window with the error, so
	// the attempt's time is provider error rather than time to first
	// token or streaming.
	categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
	require.InDelta(t, 2, categories[CategoryProviderError], 0.001)
	require.Zero(t, categories[CategoryStreaming])
	require.Zero(t, categories[CategoryTimeToFirstToken])

	var total float64
	for _, seconds := range categories {
		total += seconds
	}
	require.InDelta(t, fixture.clock.Now().Sub(turnStart).Seconds(), total, 0.001)
}

func TestTurnAccountingRotatedTurnPartition(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)

	// A rotated turn is anchored at the moment its message was queued,
	// so the queue wait covers the head of the turn and nothing else
	// accounts for that window.
	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindRoot), acc)
	turnStart := fixture.clock.Now()
	turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)

	fixture.clock.Advance(2 * time.Second)
	fixture.tracer.RecordAs(turnCtx, StageQueueWait, ScopeTurn, StageModel{},
		turnStart, fixture.clock.Now(), nil)

	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	_, prepare := fixture.tracer.Start(stepCtx, StagePrepare)
	fixture.clock.Advance(time.Second)
	prepare.End(nil)
	step.End(nil)

	acc.MarkCompleted()
	turnSpan.End(nil)

	turnDuration := fixture.clock.Now().Sub(turnStart)
	require.Equal(t, 3*time.Second, turnDuration)

	categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
	require.InDelta(t, 2, categories[CategoryScheduling], 0.001)
	require.InDelta(t, 1, categories[CategoryPreparation], 0.001)
	require.InDelta(t, 0, categories[CategoryChatdOverhead], 0.001)

	var total float64
	for _, seconds := range categories {
		total += seconds
	}
	require.InDelta(t, turnDuration.Seconds(), total, 0.001)

	// The turn observed into stage_duration_seconds is the same window
	// the categories partition.
	stageTurn := fixture.stageSums(t, "coderd_chatd_stage_duration_seconds")[StageChatTurn]
	require.InDelta(t, total, stageTurn, 0.001)
}

func TestTurnAccountingSkipsUnfinishedTurns(t *testing.T) {
	t.Parallel()

	t.Run("NeverCompleted", func(t *testing.T) {
		t.Parallel()
		fixture := newTurnFixture(t)

		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(t.Context(), acc)
		turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, fixture.clock.Now(), nil)
		_, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
		fixture.clock.Advance(time.Second)
		step.End(nil)
		turnSpan.End(nil)

		require.Empty(t, fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total"))
	})

	t.Run("Invalidated", func(t *testing.T) {
		t.Parallel()
		fixture := newTurnFixture(t)

		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(t.Context(), acc)
		turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, fixture.clock.Now(), nil)
		_, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
		fixture.clock.Advance(time.Second)
		step.End(nil)
		acc.MarkCompleted()
		acc.Invalidate()
		turnSpan.End(nil)

		require.Empty(t, fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total"))
	})
}

func TestTurnAccountingIgnoresWorkOutsideTheTurn(t *testing.T) {
	t.Parallel()

	t.Run("BackgroundScope", func(t *testing.T) {
		t.Parallel()
		fixture := newTurnFixture(t)

		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(t.Context(), acc)
		turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, fixture.clock.Now(), nil)
		stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
		step.SetGenerationAction("generate_assistant")
		fixture.clock.Advance(time.Second)

		// Detached work derives its context from the step's but runs
		// in the background scope, so the accumulator it inherits must
		// not receive its stages.
		backgroundCtx := ContextWithScope(stepCtx, ScopeBackground)
		bgCtx, bgStream := fixture.tracer.Start(backgroundCtx, StageStream)
		_, bgTTFT := fixture.tracer.Start(bgCtx, StageTimeToFirstToken)
		fixture.clock.Advance(3 * time.Second)
		bgTTFT.End(nil)
		fixture.clock.Advance(4 * time.Second)
		bgStream.End(nil)
		fixture.tracer.Record(backgroundCtx, StageQueueWait, StageModel{},
			fixture.clock.Now().Add(-time.Second), fixture.clock.Now(), nil)

		step.End(nil)
		acc.MarkCompleted()
		turnSpan.End(nil)

		categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
		require.Zero(t, categories[CategoryStreaming])
		require.Zero(t, categories[CategoryTimeToFirstToken])
		require.Zero(t, categories[CategoryScheduling])
		// The step's own time is intact: the background stream did not
		// report itself as the step's child.
		require.Equal(t, 8.0, categories[CategoryChatdOverhead])
	})

	t.Run("NilTracer", func(t *testing.T) {
		t.Parallel()
		fixture := newTurnFixture(t)

		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(t.Context(), acc)
		turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, fixture.clock.Now(), nil)
		stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
		step.SetGenerationAction("generate_assistant")
		fixture.clock.Advance(time.Second)

		// A caller without a tracer runs on the turn's context. Its
		// stages are discarded rather than folded into the turn.
		var none *StageTracer
		_, stream := none.Start(stepCtx, StageStream)
		fixture.clock.Advance(2 * time.Second)
		stream.End(nil)

		step.End(nil)
		acc.MarkCompleted()
		turnSpan.End(nil)

		categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
		require.Zero(t, categories[CategoryStreaming])
		require.Equal(t, 3.0, categories[CategoryChatdOverhead])
	})
}

func TestTurnAccountingCapacityWaitIsNotCategorized(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)

	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindRoot), acc)
	turnStart := fixture.clock.Now()
	turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)

	// The capacity wait is a sub-window of the acquisition, so it is
	// summed into the stage totals but not into scheduling.
	fixture.tracer.RecordAs(turnCtx, StageAcquisition, ScopeTurn, StageModel{},
		turnStart, turnStart.Add(5*time.Second), nil)
	fixture.tracer.RecordAs(turnCtx, StageCapacityWait, ScopeTurn, StageModel{},
		turnStart.Add(time.Second), turnStart.Add(4*time.Second), nil)
	fixture.clock.Advance(5 * time.Second)
	acc.MarkCompleted()
	turnSpan.End(nil)

	categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
	require.Equal(t, 5.0, categories[CategoryScheduling])
	require.Equal(t, 0.0, categories[CategoryUnattributed])
}

func TestTurnAccountingClampsOwnTime(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)

	// Two prepare stages under one step run in parallel, so together
	// they report more child time than the step itself lasted.
	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(t.Context(), acc)
	turnStart := fixture.clock.Now()
	turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)
	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction("generate_assistant")
	_, first := fixture.tracer.Start(stepCtx, StagePrepare)
	_, second := fixture.tracer.Start(stepCtx, StagePrepare)
	fixture.clock.Advance(4 * time.Second)
	first.End(nil)
	second.End(nil)
	step.End(nil)
	acc.MarkCompleted()
	turnSpan.End(nil)

	categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
	require.Equal(t, 8.0, categories[CategoryPreparation])
	require.Equal(t, 0.0, categories[CategoryChatdOverhead], "the step's own time is clamped at zero")
	require.Equal(t, 1.0, fixture.anomalies(t)[StageAnomalyOverattributed])
}

func TestTurnAccountingConcurrentStageEnds(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)

	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(t.Context(), acc)
	turnStart := fixture.clock.Now()
	turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)
	stepCtx, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetGenerationAction(GenerationActionExecuteLocalTools)

	const workers = 16
	commits := make([]*StageSpan, workers)
	for i := range commits {
		_, commits[i] = fixture.tracer.Start(stepCtx, StageCommit)
	}
	fixture.clock.Advance(time.Second)
	end := fixture.clock.Now()

	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fixture.tracer.Record(stepCtx, StageToolCall, StageModel{}, turnStart, end, nil)
			commits[i].End(nil)
		}()
	}
	wg.Wait()
	step.End(nil)
	acc.MarkCompleted()
	turnSpan.End(nil)

	// Every commit's second reached the partition; a lost update under
	// the concurrent ends would show as a smaller total.
	categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
	require.Equal(t, float64(workers), categories[CategoryPersistence])
}

// anomalies returns the anomaly counter values keyed by reason.
func (f turnFixture) anomalies(t *testing.T) map[string]float64 {
	t.Helper()
	families, err := f.registry.Gather()
	require.NoError(t, err)
	out := map[string]float64{}
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_anomalies_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			out[metricLabel(metric, "reason")] = metric.GetCounter().GetValue()
		}
	}
	return out
}

func TestTurnAccountingAnomalies(t *testing.T) {
	t.Parallel()

	t.Run("Overattributed", func(t *testing.T) {
		t.Parallel()
		fixture := newTurnFixture(t)

		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(t.Context(), acc)
		turnStart := fixture.clock.Now()
		turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)
		// Two steps overlapping in wall time each report their full
		// duration, so the categories exceed the turn.
		_, first := fixture.tracer.Start(turnCtx, StageGenerationStep)
		_, second := fixture.tracer.Start(turnCtx, StageGenerationStep)
		fixture.clock.Advance(4 * time.Second)
		first.End(nil)
		second.End(nil)
		acc.MarkCompleted()
		turnSpan.End(nil)

		categories := fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total")
		require.Equal(t, 8.0, categories[CategoryChatdOverhead], "categories are emitted as measured")
		require.Equal(t, 0.0, categories[CategoryUnattributed])
		require.Equal(t, 1.0, fixture.anomalies(t)[StageAnomalyOverattributed])
	})

	t.Run("NonPositiveTurn", func(t *testing.T) {
		t.Parallel()
		fixture := newTurnFixture(t)

		acc := NewTurnAccumulator()
		ctx := ContextWithTurnAccumulator(t.Context(), acc)
		_, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, fixture.clock.Now(), nil)
		acc.MarkCompleted()
		turnSpan.End(nil)

		require.Empty(t, fixture.categorySums(t, "coderd_chatd_turn_time_seconds_total"))
		require.Equal(t, 1.0, fixture.anomalies(t)[StageAnomalyNonPositiveTurn])
	})

	t.Run("CleanTurnCountsNothing", func(t *testing.T) {
		t.Parallel()
		fixture := newTurnFixture(t)
		fixture.syntheticTurn(t, StageModel{})
		require.Empty(t, fixture.anomalies(t))
	})
}

func TestTurnAccountingStampsModelOnRoot(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)
	model := StageModel{ProviderType: "openai", Model: "gpt-5", Effort: "medium"}

	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindSubagent), acc)
	turnCtx, turnSpan := fixture.tracer.StartRootAt(ctx, StageChatTurn, fixture.clock.Now(), nil)
	_, step := fixture.tracer.Start(turnCtx, StageGenerationStep)
	step.SetModel(model)
	fixture.clock.Advance(time.Second)
	step.End(nil)
	// A later step on another model does not rename the turn.
	_, second := fixture.tracer.Start(turnCtx, StageGenerationStep)
	second.SetModel(StageModel{Model: "gpt-5-mini"})
	fixture.clock.Advance(time.Second)
	second.End(nil)
	acc.MarkCompleted()
	turnSpan.End(nil)

	var turn sdktrace.ReadOnlySpan
	for _, span := range fixture.spans.Ended() {
		if Stage(span.Name()) == StageChatTurn {
			turn = span
		}
	}
	require.NotNil(t, turn)
	require.Contains(t, turn.Attributes(), attribute.String(AttrProviderType, model.ProviderType))
	require.Contains(t, turn.Attributes(), attribute.String(AttrModel, model.Model))
	require.Contains(t, turn.Attributes(), attribute.String(AttrReasoningEffort, model.Effort))

	labels := fixture.labelsOf(t, "coderd_chatd_stage_duration_seconds", "stage", string(StageChatTurn))
	require.NotContains(t, labels, "model", "the model is a span attribute on the turn, not a metric label")
}

// TestTurnMetricsEnabled covers that the turn families follow the stage
// metrics option and that the recorders accept calls either way,
// because emitTurnAccounting does not know the setting.
func TestTurnMetricsEnabled(t *testing.T) {
	t.Parallel()

	turnFamilies := []string{
		"coderd_chatd_turn_outcomes_total",
		"coderd_chatd_turn_time_seconds_total",
		"coderd_chatd_turns_total",
	}
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			metrics := NewMetricsWithOptions(registry, MetricsOptions{StageMetrics: enabled})
			metrics.RecordTurn(ChatKindRoot)
			metrics.RecordTurnOutcome(TurnOutcomeCompleted, ChatKindRoot)
			metrics.RecordTurnOutcome(TurnOutcomeError, ChatKindRoot)
			metrics.RecordTurnCategory(CategoryStreaming, ChatKindRoot, time.Minute)
			metrics.RecordTurnCategory(CategoryScheduling, ChatKindRoot, 0)

			families, err := registry.Gather()
			require.NoError(t, err)
			var got []string
			byName := map[string]*dto.MetricFamily{}
			for _, family := range families {
				if slices.Contains(turnFamilies, family.GetName()) {
					got = append(got, family.GetName())
					byName[family.GetName()] = family
				}
			}
			slices.Sort(got)
			if !enabled {
				require.Empty(t, got)
				return
			}
			require.Equal(t, turnFamilies, got)
			// One turn, sixty seconds streaming, zero scheduling.
			turns := byName["coderd_chatd_turns_total"].GetMetric()
			require.Len(t, turns, 1)
			require.Equal(t, float64(1), turns[0].GetCounter().GetValue())
			seconds := map[Category]float64{}
			for _, metric := range byName["coderd_chatd_turn_time_seconds_total"].GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "category" {
						seconds[Category(label.GetValue())] = metric.GetCounter().GetValue()
					}
				}
			}
			require.Equal(t, map[Category]float64{CategoryStreaming: 60, CategoryScheduling: 0}, seconds)
			outcomes := map[TurnOutcome]float64{}
			for _, metric := range byName["coderd_chatd_turn_outcomes_total"].GetMetric() {
				for _, label := range metric.GetLabel() {
					if label.GetName() == "outcome" {
						outcomes[TurnOutcome(label.GetValue())] = metric.GetCounter().GetValue()
					}
				}
			}
			require.Equal(t, map[TurnOutcome]float64{TurnOutcomeCompleted: 1, TurnOutcomeError: 1}, outcomes)
		})
	}
}
