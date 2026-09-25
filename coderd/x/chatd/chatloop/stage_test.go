package chatloop_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chatloop"
	"github.com/coder/quartz"
)

// stageFixture wires a stage tracer to an in-memory span recorder and
// a private metrics registry.
type stageFixture struct {
	tracer   *chatloop.StageTracer
	clock    *quartz.Mock
	spans    *tracetest.SpanRecorder
	registry *prometheus.Registry
}

func newStageFixture(t *testing.T) stageFixture {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		// The test context is already canceled during cleanup, so the
		// flush uses a fresh one.
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	registry := prometheus.NewRegistry()
	clock := quartz.NewMock(t)
	return stageFixture{
		tracer:   chatloop.NewStageTracer(provider, newStageMetrics(registry), chatloop.WithClock(clock)),
		clock:    clock,
		spans:    recorder,
		registry: registry,
	}
}

// newStageMetrics returns metrics with the stage families registered
// on registry.
func newStageMetrics(registry *prometheus.Registry) *chatloop.Metrics {
	return chatloop.NewMetricsWithOptions(registry, chatloop.MetricsOptions{StageMetrics: true})
}

// stageKey identifies one stage_duration_seconds series.
type stageKey struct {
	stage    chatloop.Stage
	scope    chatloop.Scope
	chatKind chatloop.ChatKind
}

// modelStageKey identifies one model_stage_duration_seconds series.
type modelStageKey struct {
	stage        chatloop.Stage
	providerType string
	chatKind     chatloop.ChatKind
	model        string
}

// stageObservations returns the observation count per stage series
// recorded on coderd_chatd_stage_duration_seconds.
func (f stageFixture) stageObservations(t *testing.T) map[stageKey]uint64 {
	t.Helper()
	counts := map[stageKey]uint64{}
	for _, metric := range gatherFamily(t, f.registry, "coderd_chatd_stage_duration_seconds") {
		key := stageKey{
			stage:    chatloop.Stage(labelValue(metric, "stage")),
			scope:    chatloop.Scope(labelValue(metric, "scope")),
			chatKind: chatloop.ChatKind(labelValue(metric, "chat_kind")),
		}
		counts[key] = metric.GetHistogram().GetSampleCount()
	}
	return counts
}

// modelStageObservations returns the observation count per series
// recorded on coderd_chatd_model_stage_duration_seconds.
func (f stageFixture) modelStageObservations(t *testing.T) map[modelStageKey]uint64 {
	t.Helper()
	counts := map[modelStageKey]uint64{}
	for _, metric := range gatherFamily(t, f.registry, "coderd_chatd_model_stage_duration_seconds") {
		key := modelStageKey{
			stage:        chatloop.Stage(labelValue(metric, "stage")),
			providerType: labelValue(metric, "provider_type"),
			chatKind:     chatloop.ChatKind(labelValue(metric, "chat_kind")),
			model:        labelValue(metric, "model"),
		}
		counts[key] = metric.GetHistogram().GetSampleCount()
	}
	return counts
}

func (f stageFixture) stageSum(t *testing.T, stage chatloop.Stage) float64 {
	t.Helper()
	for _, metric := range gatherFamily(t, f.registry, "coderd_chatd_stage_duration_seconds") {
		if labelValue(metric, "stage") == string(stage) {
			return metric.GetHistogram().GetSampleSum()
		}
	}
	t.Fatalf("stage %q was not recorded", stage)
	return 0
}

// spanDuration returns the duration of the single ended span named
// after stage.
func (f stageFixture) spanDuration(t *testing.T, stage chatloop.Stage) time.Duration {
	t.Helper()
	var found []sdktrace.ReadOnlySpan
	for _, span := range f.spans.Ended() {
		if span.Name() == string(stage) {
			found = append(found, span)
		}
	}
	require.Len(t, found, 1)
	return found[0].EndTime().Sub(found[0].StartTime())
}

// gatherFamily returns the metrics of the named family, or nil when
// the family has no series.
func gatherFamily(t *testing.T, registry *prometheus.Registry, name string) []*dto.Metric {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() == name {
			return family.GetMetric()
		}
	}
	return nil
}

func labelValue(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

// anomalyCount returns the coderd_chatd_stage_anomalies_total value for
// reason, or 0 when the series does not exist.
func (f stageFixture) anomalyCount(t *testing.T, reason chatloop.StageAnomaly) float64 {
	t.Helper()
	for _, metric := range gatherFamily(t, f.registry, "coderd_chatd_stage_anomalies_total") {
		if labelValue(metric, "reason") == string(reason) {
			return metric.GetCounter().GetValue()
		}
	}
	return 0
}

func TestStageTracerStart(t *testing.T) {
	t.Parallel()

	t.Run("RecordsSpanAndMetricOnce", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		_, span := fixture.tracer.Start(t.Context(), chatloop.StageCommit,
			attribute.String(chatloop.AttrProvider, "anthropic"),
		)
		fixture.clock.Advance(5 * time.Second)
		span.End(nil)
		fixture.clock.Advance(time.Second)
		span.End(nil)

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, string(chatloop.StageCommit), ended[0].Name())
		require.Equal(t, codes.Unset, ended[0].Status().Code)
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrProvider, "anthropic"))
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrScope, string(chatloop.ScopeBackground)))
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageCommit, scope: chatloop.ScopeBackground}: 1,
		}, fixture.stageObservations(t))
		// The span and the observation share the tracer clock's window.
		require.Equal(t, 5*time.Second, fixture.spanDuration(t, chatloop.StageCommit))
		require.InDelta(t, 5.0, fixture.stageSum(t, chatloop.StageCommit), 0.001)
	})

	t.Run("SetAttributesBeforeEndOnly", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		_, span := fixture.tracer.Start(t.Context(), chatloop.StageCommit)
		span.SetAttributes(attribute.String(chatloop.AttrToolName, "before"))
		span.End(nil)
		span.SetAttributes(attribute.String(chatloop.AttrHTTPMethod, "after"))

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrToolName, "before"))
		for _, attr := range ended[0].Attributes() {
			require.NotEqual(t, chatloop.AttrHTTPMethod, string(attr.Key))
		}
	})

	t.Run("ErrorEndsMarkStatus", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		_, stream := fixture.tracer.Start(t.Context(), chatloop.StageStream)
		stream.End(xerrors.New("stream failed"))
		_, ttft := fixture.tracer.Start(t.Context(), chatloop.StageTimeToFirstToken)
		ttft.EndWithoutObservation(xerrors.New("stream ended before the first token"))
		// The first end wins, so a later End cannot revive the
		// observation.
		ttft.End(nil)

		ended := fixture.spans.Ended()
		require.Len(t, ended, 2)
		for _, span := range ended {
			require.Equal(t, codes.Error, span.Status().Code, span.Name())
		}
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageStream, scope: chatloop.ScopeBackground}: 1,
		}, fixture.stageObservations(t))
	})

	t.Run("NestsUnderParent", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		parentCtx, parent := fixture.tracer.Start(t.Context(), chatloop.StageGenerationStep)
		_, child := fixture.tracer.Start(parentCtx, chatloop.StagePrepare)
		child.End(nil)
		parent.End(nil)

		ended := fixture.spans.Ended()
		require.Len(t, ended, 2)
		require.Equal(t, string(chatloop.StagePrepare), ended[0].Name())
		require.Equal(t, ended[1].SpanContext().SpanID(), ended[0].Parent().SpanID())
		require.Equal(t, ended[1].SpanContext().TraceID(), ended[0].SpanContext().TraceID())
	})
}

func TestStageTracerStartRootAtIgnoresParent(t *testing.T) {
	t.Parallel()
	fixture := newStageFixture(t)

	outerCtx, outer := fixture.tracer.Start(t.Context(), chatloop.StageGenerationStep)
	_, root := fixture.tracer.StartRootAt(outerCtx, chatloop.StageChatTurn, time.Time{})
	fixture.clock.Advance(time.Second)
	root.End(nil)
	outer.End(nil)

	ended := fixture.spans.Ended()
	require.Len(t, ended, 2)
	turn, step := ended[0], ended[1]
	require.Equal(t, string(chatloop.StageChatTurn), turn.Name())
	require.False(t, turn.Parent().IsValid())
	require.NotEqual(t, step.SpanContext().TraceID(), turn.SpanContext().TraceID())
	// A zero start opens the span now.
	require.Equal(t, time.Second, fixture.spanDuration(t, chatloop.StageChatTurn))
}

func TestStageTracerStartRootAt(t *testing.T) {
	t.Parallel()
	fixture := newStageFixture(t)

	start := fixture.clock.Now().Add(-45 * time.Second)
	turnCtx, turn := fixture.tracer.StartRootAt(t.Context(), chatloop.StageChatTurn, start)
	fixture.tracer.Record(turnCtx, chatloop.StageAcquisition, chatloop.StageModel{}, start, start.Add(time.Second), nil)
	turn.End(nil)

	var chatTurn, acquisition sdktrace.ReadOnlySpan
	for _, span := range fixture.spans.Ended() {
		switch chatloop.Stage(span.Name()) {
		case chatloop.StageChatTurn:
			chatTurn = span
		case chatloop.StageAcquisition:
			acquisition = span
		}
	}
	require.NotNil(t, chatTurn)
	require.NotNil(t, acquisition)
	require.Equal(t, start.UTC(), chatTurn.StartTime().UTC())
	require.False(t, acquisition.StartTime().Before(chatTurn.StartTime()))
	require.Equal(t, chatTurn.SpanContext().SpanID(), acquisition.Parent().SpanID())
	// A root turn span opens a turn, and stages recorded inside it
	// inherit the turn scope from its context.
	require.Contains(t, chatTurn.Attributes(),
		attribute.String(chatloop.AttrScope, string(chatloop.ScopeTurn)))
	require.Contains(t, acquisition.Attributes(),
		attribute.String(chatloop.AttrScope, string(chatloop.ScopeTurn)))
	require.Equal(t, map[stageKey]uint64{
		{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn}:    1,
		{stage: chatloop.StageAcquisition, scope: chatloop.ScopeTurn}: 1,
	}, fixture.stageObservations(t))
	// The histogram observation runs from the explicit start, so it
	// covers the same window the span reports.
	require.InDelta(t, 45.0, fixture.stageSum(t, chatloop.StageChatTurn), 0.001)
	require.Equal(t, 45*time.Second, fixture.spanDuration(t, chatloop.StageChatTurn))
	require.Zero(t, fixture.anomalyCount(t, chatloop.StageAnomalyFutureStart))
}

func TestStageTracerStartRootAtFutureStart(t *testing.T) {
	t.Parallel()

	t.Run("ObservedStage", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		// A start stamped ahead of this replica's clock begins now and
		// is counted, so cross-host skew stays visible.
		_, turn := fixture.tracer.StartRootAt(t.Context(), chatloop.StageChatTurn, fixture.clock.Now().Add(time.Minute))
		fixture.clock.Advance(2 * time.Second)
		turn.End(nil)

		require.InDelta(t, 2.0, fixture.stageSum(t, chatloop.StageChatTurn), 0.001)
		require.Equal(t, 2*time.Second, fixture.spanDuration(t, chatloop.StageChatTurn))
		require.Equal(t, 1.0, fixture.anomalyCount(t, chatloop.StageAnomalyFutureStart))
	})

	t.Run("SpanOnlyStage", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		// A span-only stage has no observation to adjust, so it adds no
		// anomaly.
		_, step := fixture.tracer.StartRootAt(t.Context(), chatloop.StageGenerationStep, fixture.clock.Now().Add(time.Minute))
		fixture.clock.Advance(2 * time.Second)
		step.End(nil)

		require.Equal(t, 2*time.Second, fixture.spanDuration(t, chatloop.StageGenerationStep))
		require.Empty(t, fixture.stageObservations(t))
		require.Zero(t, fixture.anomalyCount(t, chatloop.StageAnomalyFutureStart))
	})
}

func TestStageTracerDetachedScope(t *testing.T) {
	t.Parallel()
	fixture := newStageFixture(t)

	turnCtx, turn := fixture.tracer.StartRootAt(t.Context(), chatloop.StageChatTurn, time.Time{})
	// Background work detaches from the turn by stripping the span
	// and marking the context background scoped.
	detachedCtx := chatloop.ContextWithScope(
		trace.ContextWithSpanContext(turnCtx, trace.SpanContext{}),
		chatloop.ScopeBackground,
	)
	_, attempt := fixture.tracer.Start(detachedCtx, chatloop.StageProviderAttempt)
	attempt.End(nil)
	turn.End(nil)

	require.Equal(t, map[stageKey]uint64{
		{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn}:              1,
		{stage: chatloop.StageProviderAttempt, scope: chatloop.ScopeBackground}: 1,
	}, fixture.stageObservations(t))

	for _, span := range fixture.spans.Ended() {
		if chatloop.Stage(span.Name()) != chatloop.StageProviderAttempt {
			continue
		}
		require.False(t, span.Parent().IsValid())
		require.Contains(t, span.Attributes(),
			attribute.String(chatloop.AttrScope, string(chatloop.ScopeBackground)))
	}
}

func TestStageTracerChatKind(t *testing.T) {
	t.Parallel()

	t.Run("InheritedByDerivedStages", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		ctx := chatloop.ContextWithChatKind(t.Context(), chatloop.ChatKindSubagent)
		ctx = chatloop.ContextWithOrganization(ctx, "acme")
		turnCtx, turn := fixture.tracer.StartRootAt(ctx, chatloop.StageChatTurn, time.Time{})
		stepCtx, step := fixture.tracer.Start(turnCtx, chatloop.StageGenerationStep)
		start := fixture.clock.Now().Add(-time.Second)
		fixture.tracer.Record(stepCtx, chatloop.StageToolCall, chatloop.StageModel{}, start, fixture.clock.Now(), nil)
		step.End(nil)
		turn.End(nil)

		// The organization is a span attribute only; the histogram
		// carries the chat kind.
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn, chatKind: chatloop.ChatKindSubagent}: 1,
			{stage: chatloop.StageToolCall, scope: chatloop.ScopeTurn, chatKind: chatloop.ChatKindSubagent}: 1,
		}, fixture.stageObservations(t))
		for _, metric := range gatherFamily(t, fixture.registry, "coderd_chatd_stage_duration_seconds") {
			require.Empty(t, labelValue(metric, "organization_name"))
		}

		for _, span := range fixture.spans.Ended() {
			require.Contains(t, span.Attributes(),
				attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindSubagent)))
			require.Contains(t, span.Attributes(),
				attribute.String(chatloop.AttrOrganizationName, "acme"))
		}
	})

	t.Run("UnknownChatLeavesLabelsEmpty", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		start := fixture.clock.Now().Add(-time.Second)
		fixture.tracer.Record(chatloop.ContextWithScope(t.Context(), chatloop.ScopeTurn), chatloop.StageQueueWait,
			chatloop.StageModel{}, start, fixture.clock.Now(), nil)

		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageQueueWait, scope: chatloop.ScopeTurn}: 1,
		}, fixture.stageObservations(t))

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrScope, string(chatloop.ScopeTurn)))
		for _, attr := range ended[0].Attributes() {
			require.NotEqual(t, chatloop.AttrChatKind, string(attr.Key))
			require.NotEqual(t, chatloop.AttrOrganizationName, string(attr.Key))
			require.NotEqual(t, chatloop.AttrProvider, string(attr.Key))
			require.NotEqual(t, chatloop.AttrModel, string(attr.Key))
			require.NotEqual(t, chatloop.AttrReasoningEffort, string(attr.Key))
		}
	})
}

func TestStageTracerModelLabels(t *testing.T) {
	t.Parallel()

	model := chatloop.StageModel{Provider: "anthropic", ProviderType: "bedrock", Model: "claude-sonnet-4-5", Effort: "high"}

	t.Run("SetModelLabelsSpanAndModelDuration", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		ctx := chatloop.ContextWithChatKind(t.Context(), chatloop.ChatKindRoot)
		turnCtx, turn := fixture.tracer.StartRootAt(ctx, chatloop.StageChatTurn, time.Time{})
		stepCtx, step := fixture.tracer.Start(turnCtx, chatloop.StageGenerationStep)
		step.SetModel(model)
		_, stream := fixture.tracer.Start(stepCtx, chatloop.StageStream)
		stream.SetModel(model)
		stream.End(nil)
		step.End(nil)
		turn.End(nil)

		// The stage histogram carries no model; the model histogram
		// carries the stream, whose duration is the provider's work,
		// but not the generation step, which only wraps it.
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn, chatKind: chatloop.ChatKindRoot}: 1,
			{stage: chatloop.StageStream, scope: chatloop.ScopeTurn, chatKind: chatloop.ChatKindRoot}:   1,
		}, fixture.stageObservations(t))
		require.Equal(t, map[modelStageKey]uint64{{
			stage:        chatloop.StageStream,
			providerType: model.ProviderType,
			chatKind:     chatloop.ChatKindRoot,
			model:        model.Model,
		}: 1}, fixture.modelStageObservations(t))

		var sawStep bool
		for _, span := range fixture.spans.Ended() {
			if chatloop.Stage(span.Name()) != chatloop.StageGenerationStep {
				continue
			}
			sawStep = true
			require.Contains(t, span.Attributes(),
				attribute.String(chatloop.AttrProvider, model.Provider))
			require.Contains(t, span.Attributes(),
				attribute.String(chatloop.AttrProviderType, model.ProviderType))
			require.Contains(t, span.Attributes(),
				attribute.String(chatloop.AttrModel, model.Model))
			require.Contains(t, span.Attributes(),
				attribute.String(chatloop.AttrReasoningEffort, model.Effort))
		}
		require.True(t, sawStep)
	})

	t.Run("RecordCarriesModelLabels", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		start := fixture.clock.Now().Add(-time.Second)
		fixture.tracer.Record(chatloop.ContextWithScope(t.Context(), chatloop.ScopeTurn), chatloop.StageStream, model, start, fixture.clock.Now(), nil)

		require.Equal(t, map[stageKey]uint64{{
			stage: chatloop.StageStream,
			scope: chatloop.ScopeTurn,
		}: 1}, fixture.stageObservations(t))
		require.Equal(t, map[modelStageKey]uint64{{
			stage:        chatloop.StageStream,
			providerType: model.ProviderType,
			model:        model.Model,
		}: 1}, fixture.modelStageObservations(t))

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrProvider, model.Provider))
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrProviderType, model.ProviderType))
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrModel, model.Model))
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrReasoningEffort, model.Effort))
	})

	t.Run("BackgroundScopeSkipsModelDuration", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		start := fixture.clock.Now().Add(-time.Second)
		fixture.tracer.Record(t.Context(), chatloop.StageProviderAttempt, model, start, fixture.clock.Now(), nil)

		require.Equal(t, map[stageKey]uint64{{
			stage: chatloop.StageProviderAttempt,
			scope: chatloop.ScopeBackground,
		}: 1}, fixture.stageObservations(t))
		require.Empty(t, fixture.modelStageObservations(t))
		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrModel, model.Model))
	})

	t.Run("ModelStageWithoutModelObservesStageOnly", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		// A turn-scoped model stage with a provider type but no model
		// stays off the model histogram.
		start := fixture.clock.Now().Add(-time.Second)
		fixture.tracer.Record(chatloop.ContextWithScope(t.Context(), chatloop.ScopeTurn), chatloop.StageStream,
			chatloop.StageModel{ProviderType: model.ProviderType}, start, fixture.clock.Now(), nil)

		require.Equal(t, map[stageKey]uint64{{
			stage: chatloop.StageStream,
			scope: chatloop.ScopeTurn,
		}: 1}, fixture.stageObservations(t))
		require.Empty(t, fixture.modelStageObservations(t))
	})
}

func TestStageTracerRecord(t *testing.T) {
	t.Parallel()

	t.Run("UsesExplicitTimestamps", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		start := fixture.clock.Now().Add(-90 * time.Second)
		end := start.Add(30 * time.Second)
		ctx := chatloop.ContextWithScope(chatloop.ContextWithChatKind(t.Context(), chatloop.ChatKindRoot), chatloop.ScopeTurn)
		fixture.tracer.Record(ctx, chatloop.StageQueueWait, chatloop.StageModel{}, start, end, nil)

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, string(chatloop.StageQueueWait), ended[0].Name())
		require.Equal(t, start.UTC(), ended[0].StartTime().UTC())
		require.Equal(t, end.UTC(), ended[0].EndTime().UTC())
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindRoot)))
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageQueueWait, scope: chatloop.ScopeTurn, chatKind: chatloop.ChatKindRoot}: 1,
		}, fixture.stageObservations(t))
		require.InDelta(t, 30, fixture.stageSum(t, chatloop.StageQueueWait), 0.001)
	})

	t.Run("DropsUnusableWindows", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		now := fixture.clock.Now()
		fixture.tracer.Record(t.Context(), chatloop.StageAcquisition, chatloop.StageModel{}, time.Time{}, now, nil)
		fixture.tracer.Record(t.Context(), chatloop.StageAcquisition, chatloop.StageModel{}, now, time.Time{}, nil)
		fixture.tracer.Record(t.Context(), chatloop.StageAcquisition, chatloop.StageModel{}, now, now.Add(-time.Second), nil)
		// Span-only stages have no observation to drop, so they add no
		// anomaly.
		fixture.tracer.Record(t.Context(), chatloop.StageCompaction, chatloop.StageModel{}, time.Time{}, now, nil)
		fixture.tracer.Record(t.Context(), chatloop.StageThinking, chatloop.StageModel{}, now, now.Add(-time.Second), nil)

		require.Empty(t, fixture.spans.Ended())
		require.Empty(t, fixture.stageObservations(t))
		require.Equal(t, 2.0, fixture.anomalyCount(t, chatloop.StageAnomalyMissingTimestamp))
		require.Equal(t, 1.0, fixture.anomalyCount(t, chatloop.StageAnomalyInvertedWindow))
	})

	t.Run("MarksErrorStatus", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		start := fixture.clock.Now().Add(-time.Second)
		fixture.tracer.Record(t.Context(), chatloop.StageToolCall, chatloop.StageModel{}, start, fixture.clock.Now(), xerrors.New("tool failed"))

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, codes.Error, ended[0].Status().Code)
		require.Equal(t, "tool failed", ended[0].Status().Description)
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageToolCall, scope: chatloop.ScopeBackground}: 1,
		}, fixture.stageObservations(t))
	})

	t.Run("ObservesZeroWidthWindow", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		now := fixture.clock.Now()
		fixture.tracer.Record(t.Context(), chatloop.StageCommit, chatloop.StageModel{}, now, now, nil)

		require.Len(t, fixture.spans.Ended(), 1)
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageCommit, scope: chatloop.ScopeBackground}: 1,
		}, fixture.stageObservations(t))
		require.Zero(t, fixture.stageSum(t, chatloop.StageCommit))
	})
}

func TestStageDurationBuckets(t *testing.T) {
	t.Parallel()

	// Alert thresholds are written against these edges, so the ladder
	// must contain them as round numbers rather than a generated ladder.
	alertEdges := []float64{1, 5, 10, 30, 60, 300, 3600}
	registry := prometheus.NewRegistry()
	metrics := newStageMetrics(registry)
	metrics.RecordStageDuration(chatloop.StageStream, chatloop.ScopeTurn, chatloop.ChatKindRoot,
		chatloop.StageModel{ProviderType: "p", Model: "m"}, 45*time.Minute)

	families, err := registry.Gather()
	require.NoError(t, err)
	var buckets, modelBuckets []*dto.Bucket
	for _, family := range families {
		switch family.GetName() {
		case "coderd_chatd_stage_duration_seconds":
			require.Len(t, family.GetMetric(), 1)
			buckets = family.GetMetric()[0].GetHistogram().GetBucket()
		case "coderd_chatd_model_stage_duration_seconds":
			require.Len(t, family.GetMetric(), 1)
			modelBuckets = family.GetMetric()[0].GetHistogram().GetBucket()
		}
	}
	// Both histograms use the same ladder.
	require.Len(t, modelBuckets, len(buckets))
	edges := make([]float64, 0, len(buckets))
	for i, bucket := range buckets {
		require.Equal(t, bucket.GetUpperBound(), modelBuckets[i].GetUpperBound())
		edges = append(edges, bucket.GetUpperBound())
	}
	for _, want := range alertEdges {
		require.Contains(t, edges, want, "bucket edge %v missing", want)
	}
	// A 45 minute stream falls in the 1800 to 3600 bucket, not +Inf.
	for _, bucket := range buckets {
		if bucket.GetUpperBound() < 3600 {
			require.Zero(t, bucket.GetCumulativeCount(), "le=%v", bucket.GetUpperBound())
		} else {
			require.Equal(t, uint64(1), bucket.GetCumulativeCount(), "le=%v", bucket.GetUpperBound())
		}
	}
}

// TestStageMembership pins which stages each histogram observes. A
// turn-scoped observation with a known model reaches every histogram
// its stage belongs to.
func TestStageMembership(t *testing.T) {
	t.Parallel()

	stages := []struct {
		stage    chatloop.Stage
		observed bool
		model    bool
	}{
		{stage: chatloop.StageChatTurn, observed: true},
		{stage: chatloop.StageQueueWait, observed: true},
		{stage: chatloop.StageAcquisition, observed: true},
		{stage: chatloop.StageGenerationStep},
		{stage: chatloop.StagePrepare},
		{stage: chatloop.StageMCPConnect, observed: true},
		{stage: chatloop.StageProviderAttempt, observed: true, model: true},
		{stage: chatloop.StageStream, observed: true, model: true},
		// Per-model time to first token is ttft_seconds.
		{stage: chatloop.StageTimeToFirstToken, observed: true},
		{stage: chatloop.StageThinking},
		{stage: chatloop.StageToolCall, observed: true},
		{stage: chatloop.StageCommit, observed: true},
		{stage: chatloop.StageCompaction},
		{stage: chatloop.StageRetryBackoff, observed: true},
	}

	registry := prometheus.NewRegistry()
	metrics := newStageMetrics(registry)
	model := chatloop.StageModel{ProviderType: "p", Model: "m"}
	wantStages := map[stageKey]uint64{}
	wantModelStages := map[modelStageKey]uint64{}
	for _, tc := range stages {
		metrics.RecordStageDuration(tc.stage, chatloop.ScopeTurn, chatloop.ChatKindRoot, model, time.Second)
		if tc.observed {
			wantStages[stageKey{stage: tc.stage, scope: chatloop.ScopeTurn, chatKind: chatloop.ChatKindRoot}] = 1
		}
		if tc.model {
			wantModelStages[modelStageKey{stage: tc.stage, providerType: model.ProviderType, chatKind: chatloop.ChatKindRoot, model: model.Model}] = 1
		}
	}

	fixture := stageFixture{registry: registry}
	require.Equal(t, wantStages, fixture.stageObservations(t))
	require.Equal(t, wantModelStages, fixture.modelStageObservations(t))
}

// TestStageMetricsEnabled covers which stage families are exposed with
// and without the stage metrics option. Both settings must accept every
// recorder call, since the tracer does not know the setting.
func TestStageMetricsEnabled(t *testing.T) {
	t.Parallel()

	record := func(m *chatloop.Metrics) {
		m.RecordStageDuration(chatloop.StageStream, chatloop.ScopeTurn, chatloop.ChatKindRoot,
			chatloop.StageModel{ProviderType: "p", Model: "m"}, time.Second)
		m.RecordStageDuration(chatloop.StageCommit, chatloop.ScopeTurn, chatloop.ChatKindRoot, chatloop.StageModel{}, -time.Second)
		// Span-only stages are filtered before the elapsed check, so this
		// adds no anomaly.
		m.RecordStageDuration(chatloop.StageGenerationStep, chatloop.ScopeTurn, chatloop.ChatKindRoot, chatloop.StageModel{}, -time.Second)
		m.RecordStageAnomaly(chatloop.StageAnomalyInvertedWindow)
	}

	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			metrics := chatloop.NewMetricsWithOptions(registry, chatloop.MetricsOptions{StageMetrics: enabled})
			record(metrics)

			fixture := stageFixture{registry: registry}
			require.Equal(t, enabled, len(fixture.stageObservations(t)) > 0)
			require.Equal(t, enabled, len(fixture.modelStageObservations(t)) > 0)
			// The negative commit counts as negative_elapsed; the direct
			// call counts as inverted_window.
			anomalies := map[chatloop.StageAnomaly]float64{}
			for _, metric := range gatherFamily(t, registry, "coderd_chatd_stage_anomalies_total") {
				anomalies[chatloop.StageAnomaly(labelValue(metric, "reason"))] = metric.GetCounter().GetValue()
			}
			if !enabled {
				require.Empty(t, anomalies)
				return
			}
			require.Equal(t, map[chatloop.StageAnomaly]float64{
				chatloop.StageAnomalyNegativeElapsed: 1,
				chatloop.StageAnomalyInvertedWindow:  1,
			}, anomalies)
		})
	}
}

// TestStageTracerWithoutProvider covers a metrics-only deployment: a
// no-op tracer produces invalid span contexts, and the scope must still
// follow the turn.
func TestStageTracerWithoutProvider(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	clock := quartz.NewMock(t)
	tracer := chatloop.NewStageTracer(nil, newStageMetrics(registry), chatloop.WithClock(clock))

	turnCtx, turn := tracer.StartRootAt(t.Context(), chatloop.StageChatTurn, time.Time{})
	require.False(t, turn.SpanContext().IsValid())
	stepCtx, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	tracer.Record(stepCtx, chatloop.StageToolCall, chatloop.StageModel{}, clock.Now().Add(-time.Second), clock.Now(), nil)
	step.End(nil)
	turn.End(nil)

	fixture := stageFixture{registry: registry}
	require.Equal(t, map[stageKey]uint64{
		{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn}: 1,
		{stage: chatloop.StageToolCall, scope: chatloop.ScopeTurn}: 1,
	}, fixture.stageObservations(t))
}

func TestNilStageTracer(t *testing.T) {
	t.Parallel()

	// A nil tracer and the nil span it returns discard everything and
	// must not panic.
	var tracer *chatloop.StageTracer
	now := tracer.Now()
	_, span := tracer.Start(t.Context(), chatloop.StageToolCall)
	require.Nil(t, span)
	span.SetModel(chatloop.StageModel{Model: "m"})
	require.False(t, span.SpanContext().IsValid())
	span.End(nil)
	tracer.Record(t.Context(), chatloop.StageThinking, chatloop.StageModel{}, now.Add(-time.Second), now, nil)
	tracer.RecordAnomaly(chatloop.StageAnomalyInvertedWindow)
}
