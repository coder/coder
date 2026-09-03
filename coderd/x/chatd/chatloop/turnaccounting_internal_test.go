package chatloop

import (
	"context"
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

// sums returns the sample sum of each series of a histogram family,
// keyed by the value of the named label.
func (f turnFixture) sums(t *testing.T, family, label string) map[string]float64 {
	t.Helper()
	families, err := f.registry.Gather()
	require.NoError(t, err)
	out := map[string]float64{}
	for _, metricFamily := range families {
		if metricFamily.GetName() != family {
			continue
		}
		for _, metric := range metricFamily.GetMetric() {
			out[metricLabel(metric, label)] = metric.GetHistogram().GetSampleSum()
		}
	}
	return out
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

func metricLabel(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

// syntheticTurn runs one turn made of the stages the categories are
// built from and returns the turn's accumulator and total duration.
func (f turnFixture) syntheticTurn(t *testing.T, model StageModel) (*TurnAccumulator, time.Duration) {
	t.Helper()
	acc := NewTurnAccumulator()
	ctx := ContextWithTurnAccumulator(ContextWithChatKind(t.Context(), ChatKindRoot), acc)
	turnStart := f.clock.Now()
	turnCtx, turnSpan := f.tracer.StartRootAt(ctx, StageChatTurn, turnStart, nil)

	// Scheduling: a queue wait reconstructed from the queued row.
	f.tracer.RecordAs(turnCtx, StageQueueWait, ScopeTurn, StageModel{},
		turnStart.Add(-2*time.Second), turnStart, nil)

	// A generate_assistant step: prepare with an MCP connect, then a
	// stream that produced its first part, then the commit.
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
	require.Equal(t, 39*time.Second, turnDuration)

	categories := fixture.sums(t, "coderd_chatd_turn_time_seconds", "category")
	require.Equal(t, map[string]float64{
		CategoryScheduling:       2,
		CategoryTimeToFirstToken: 3,
		CategoryStreaming:        4,
		CategoryProviderError:    3,
		CategoryRetryBackoff:     6,
		CategoryToolExecution:    5,
		CategoryCompaction:       8,
		CategoryChatdOverhead:    5,
		CategoryUnattributed:     3,
	}, categories)

	var total float64
	for _, seconds := range categories {
		total += seconds
	}
	require.InDelta(t, turnDuration.Seconds(), total, 0.001,
		"the categories must partition the turn")

	shares := fixture.sums(t, "coderd_chatd_turn_time_share", "category")
	require.Len(t, shares, len(TurnTimeCategories))
	var shareTotal float64
	for category, share := range shares {
		require.InDelta(t, categories[category]/turnDuration.Seconds(), share, 0.001)
		shareTotal += share
	}
	require.InDelta(t, 1, shareTotal, 0.001)
}

func TestTurnAccountingStageTotals(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)
	model := StageModel{Model: "claude-sonnet-4-5", Effort: "high"}

	_, turnDuration := fixture.syntheticTurn(t, model)

	stages := fixture.sums(t, "coderd_chatd_turn_stage_seconds", "stage")
	require.Equal(t, map[string]float64{
		StageQueueWait:        2,
		StageGenerationStep:   10 + 7 + 9 + 8,
		StagePrepare:          2 + 1,
		StageMCPConnect:       1,
		StageStream:           7 + 3,
		StageTimeToFirstToken: 3,
		StageCommit:           1 + 1,
		StageRetryBackoff:     6,
		StageCompaction:       8,
	}, stages)
	require.NotContains(t, stages, StageChatTurn,
		"the turn's own duration is the denominator, not a stage of itself")

	shares := fixture.sums(t, "coderd_chatd_stage_share_of_turn", "stage")
	for stage, seconds := range stages {
		require.InDelta(t, seconds/turnDuration.Seconds(), shares[stage], 0.001, stage)
	}

	// One observation per stage per turn, valued at how many times the
	// stage occurred.
	counts := fixture.sums(t, "coderd_chatd_turn_stage_count", "stage")
	require.Equal(t, map[string]float64{
		StageQueueWait:        1,
		StageGenerationStep:   4,
		StagePrepare:          2,
		StageMCPConnect:       1,
		StageStream:           2,
		StageTimeToFirstToken: 1,
		StageCommit:           2,
		StageRetryBackoff:     1,
		StageCompaction:       1,
	}, counts)
	for stage := range counts {
		require.Contains(t, stages, stage)
	}

	// The turn's stage rows carry the model the turn resolved.
	labels := fixture.labelsOf(t, "coderd_chatd_turn_stage_seconds", "stage", StageStream)
	require.Equal(t, model.Model, labels["model"])
	require.Equal(t, model.Effort, labels["effort"])
	require.Equal(t, ChatKindRoot, labels["chat_kind"])
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
	categories := fixture.sums(t, "coderd_chatd_turn_time_seconds", "category")
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

	categories := fixture.sums(t, "coderd_chatd_turn_time_seconds", "category")
	require.InDelta(t, 2, categories[CategoryScheduling], 0.001)
	require.InDelta(t, 1, categories[CategoryChatdOverhead], 0.001)

	var total float64
	for _, seconds := range categories {
		total += seconds
	}
	require.InDelta(t, turnDuration.Seconds(), total, 0.001)

	// The turn observed into stage_duration_seconds is the same window
	// the categories partition.
	stageTurn := fixture.sums(t, "coderd_chatd_stage_duration_seconds", "stage")[StageChatTurn]
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

		require.Empty(t, fixture.sums(t, "coderd_chatd_turn_time_seconds", "category"))
		require.Empty(t, fixture.sums(t, "coderd_chatd_turn_stage_seconds", "stage"))
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

		require.Empty(t, fixture.sums(t, "coderd_chatd_turn_time_seconds", "category"))
	})

	t.Run("EndedWithError", func(t *testing.T) {
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
		turnSpan.End(xerrors.New("turn failed"))

		require.Empty(t, fixture.sums(t, "coderd_chatd_turn_time_seconds", "category"))
	})
}

func TestTurnAccountingStampsModelOnRoot(t *testing.T) {
	t.Parallel()
	fixture := newTurnFixture(t)
	model := StageModel{Model: "gpt-5", Effort: "medium"}

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
		if span.Name() == StageChatTurn {
			turn = span
		}
	}
	require.NotNil(t, turn)
	require.Contains(t, turn.Attributes(), attribute.String(AttrModel, model.Model))
	require.Contains(t, turn.Attributes(), attribute.String(AttrReasoningEffort, model.Effort))

	labels := fixture.labelsOf(t, "coderd_chatd_stage_duration_seconds", "stage", StageChatTurn)
	require.Equal(t, model.Model, labels["model"])
	require.Equal(t, model.Effort, labels["effort"])
}
