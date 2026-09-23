package chatloop_test

import (
	"context"
	"slices"
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
)

// stageFixture wires a stage tracer to an in-memory span recorder and
// a private metrics registry.
type stageFixture struct {
	tracer   *chatloop.StageTracer
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
	return stageFixture{
		tracer:   chatloop.NewStageTracer(provider, chatloop.NewMetrics(registry)),
		spans:    recorder,
		registry: registry,
	}
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
	families, err := f.registry.Gather()
	require.NoError(t, err)
	counts := map[stageKey]uint64{}
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			key := stageKey{
				stage:    chatloop.Stage(labelValue(metric, "stage")),
				scope:    chatloop.Scope(labelValue(metric, "scope")),
				chatKind: chatloop.ChatKind(labelValue(metric, "chat_kind")),
			}
			counts[key] = metric.GetHistogram().GetSampleCount()
		}
	}
	return counts
}

// modelStageObservations returns the observation count per series
// recorded on coderd_chatd_model_stage_duration_seconds.
func (f stageFixture) modelStageObservations(t *testing.T) map[modelStageKey]uint64 {
	t.Helper()
	families, err := f.registry.Gather()
	require.NoError(t, err)
	counts := map[modelStageKey]uint64{}
	for _, family := range families {
		if family.GetName() != "coderd_chatd_model_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			key := modelStageKey{
				stage:        chatloop.Stage(labelValue(metric, "stage")),
				providerType: labelValue(metric, "provider_type"),
				chatKind:     chatloop.ChatKind(labelValue(metric, "chat_kind")),
				model:        labelValue(metric, "model"),
			}
			counts[key] = metric.GetHistogram().GetSampleCount()
		}
	}
	return counts
}

func (f stageFixture) stageSum(t *testing.T, stage chatloop.Stage) float64 {
	t.Helper()
	families, err := f.registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelValue(metric, "stage") == string(stage) {
				return metric.GetHistogram().GetSampleSum()
			}
		}
	}
	t.Fatalf("stage %q was not recorded", stage)
	return 0
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
func (f stageFixture) anomalyCount(t *testing.T, reason string) float64 {
	t.Helper()
	families, err := f.registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_anomalies_total" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelValue(metric, "reason") == reason {
				return metric.GetCounter().GetValue()
			}
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
		span.End(nil)
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
	})

	t.Run("MarksErrorStatus", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		_, span := fixture.tracer.Start(t.Context(), chatloop.StageStream)
		span.End(xerrors.New("stream failed"))

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, codes.Error, ended[0].Status().Code)
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

func TestStageTracerStartRoot(t *testing.T) {
	t.Parallel()
	fixture := newStageFixture(t)

	outerCtx, outer := fixture.tracer.Start(t.Context(), chatloop.StageGenerationStep)
	_, root := fixture.tracer.StartRoot(outerCtx, chatloop.StageChatTurn,
		[]trace.Link{{SpanContext: trace.SpanContextFromContext(outerCtx)}},
	)
	root.End(nil)
	outer.End(nil)

	ended := fixture.spans.Ended()
	require.Len(t, ended, 2)
	turn, step := ended[0], ended[1]
	require.Equal(t, string(chatloop.StageChatTurn), turn.Name())
	require.False(t, turn.Parent().IsValid())
	require.NotEqual(t, step.SpanContext().TraceID(), turn.SpanContext().TraceID())
	require.Len(t, turn.Links(), 1)
	require.Equal(t, step.SpanContext().SpanID(), turn.Links()[0].SpanContext.SpanID())
}

func TestStageTracerStartRootAt(t *testing.T) {
	t.Parallel()
	fixture := newStageFixture(t)

	start := time.Now().Add(-45 * time.Second)
	turnCtx, turn := fixture.tracer.StartRootAt(t.Context(), chatloop.StageChatTurn, start, nil)
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
	require.GreaterOrEqual(t, fixture.stageSum(t, chatloop.StageChatTurn), 45.0)
}

func TestStageTracerScope(t *testing.T) {
	t.Parallel()

	t.Run("TurnWorkStaysInTurnScope", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		turnCtx, turn := fixture.tracer.StartRoot(t.Context(), chatloop.StageChatTurn, nil)
		stepCtx, step := fixture.tracer.Start(turnCtx, chatloop.StageGenerationStep)
		_, attempt := fixture.tracer.Start(stepCtx, chatloop.StageProviderAttempt)
		attempt.End(nil)
		step.End(nil)
		turn.End(nil)

		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn}:        1,
			{stage: chatloop.StageProviderAttempt, scope: chatloop.ScopeTurn}: 1,
		}, fixture.stageObservations(t))
	})

	t.Run("DetachedWorkIsBackgroundScope", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		turnCtx, turn := fixture.tracer.StartRoot(t.Context(), chatloop.StageChatTurn, nil)
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
	})

	t.Run("RecordAsOverridesContextScope", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		start := time.Now().Add(-5 * time.Second)
		fixture.tracer.RecordAs(t.Context(), chatloop.StageCapacityWait, chatloop.ScopeTurn,
			chatloop.StageModel{}, start, start.Add(time.Second), nil)

		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageCapacityWait, scope: chatloop.ScopeTurn}: 1,
		}, fixture.stageObservations(t))
		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrScope, string(chatloop.ScopeTurn)))
	})
}

func TestStageTracerChatKind(t *testing.T) {
	t.Parallel()

	t.Run("InheritedByDerivedStages", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		ctx := chatloop.ContextWithChatKind(t.Context(), chatloop.ChatKindSubagent)
		ctx = chatloop.ContextWithOrganization(ctx, "acme")
		turnCtx, turn := fixture.tracer.StartRoot(ctx, chatloop.StageChatTurn, nil)
		stepCtx, step := fixture.tracer.Start(turnCtx, chatloop.StageGenerationStep)
		start := time.Now().Add(-time.Second)
		fixture.tracer.Record(stepCtx, chatloop.StageToolCall, chatloop.StageModel{}, start, time.Now(), nil)
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

		start := time.Now().Add(-time.Second)
		fixture.tracer.RecordAs(t.Context(), chatloop.StageCapacityWait, chatloop.ScopeTurn,
			chatloop.StageModel{}, start, time.Now(), nil)

		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageCapacityWait, scope: chatloop.ScopeTurn}: 1,
		}, fixture.stageObservations(t))

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		for _, attr := range ended[0].Attributes() {
			require.NotEqual(t, chatloop.AttrChatKind, string(attr.Key))
			require.NotEqual(t, chatloop.AttrOrganizationName, string(attr.Key))
			require.NotEqual(t, chatloop.AttrModel, string(attr.Key))
			require.NotEqual(t, chatloop.AttrReasoningEffort, string(attr.Key))
		}
	})
}

func TestStageTracerModelLabels(t *testing.T) {
	t.Parallel()

	model := chatloop.StageModel{ProviderType: "bedrock", Model: "claude-sonnet-4-5", Effort: "high"}

	t.Run("SetModelLabelsSpanAndModelDuration", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		turnCtx, turn := fixture.tracer.StartRoot(t.Context(), chatloop.StageChatTurn, nil)
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
			{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn}: 1,
			{stage: chatloop.StageStream, scope: chatloop.ScopeTurn}:   1,
		}, fixture.stageObservations(t))
		require.Equal(t, map[modelStageKey]uint64{{
			stage:        chatloop.StageStream,
			providerType: model.ProviderType,
			model:        model.Model,
		}: 1}, fixture.modelStageObservations(t))

		var sawStep bool
		for _, span := range fixture.spans.Ended() {
			if chatloop.Stage(span.Name()) != chatloop.StageGenerationStep {
				continue
			}
			sawStep = true
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

		start := time.Now().Add(-time.Second)
		fixture.tracer.Record(t.Context(), chatloop.StageStream, model, start, time.Now(), nil)

		require.Equal(t, map[stageKey]uint64{{
			stage: chatloop.StageStream,
			scope: chatloop.ScopeBackground,
		}: 1}, fixture.stageObservations(t))
		require.Equal(t, map[modelStageKey]uint64{{
			stage:        chatloop.StageStream,
			providerType: model.ProviderType,
			model:        model.Model,
		}: 1}, fixture.modelStageObservations(t))

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrProviderType, model.ProviderType))
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrModel, model.Model))
		require.Contains(t, ended[0].Attributes(),
			attribute.String(chatloop.AttrReasoningEffort, model.Effort))
	})

	t.Run("ModelStageWithoutModelObservesStageOnly", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		_, stream := fixture.tracer.Start(t.Context(), chatloop.StageStream)
		stream.End(nil)

		require.Equal(t, map[stageKey]uint64{{
			stage: chatloop.StageStream,
			scope: chatloop.ScopeBackground,
		}: 1}, fixture.stageObservations(t))
		require.Empty(t, fixture.modelStageObservations(t))
	})
}

func TestStageTracerRecord(t *testing.T) {
	t.Parallel()

	t.Run("UsesExplicitTimestamps", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		start := time.Now().Add(-90 * time.Second)
		end := start.Add(30 * time.Second)
		fixture.tracer.Record(t.Context(), chatloop.StageQueueWait, chatloop.StageModel{}, start, end, nil,
			attribute.String(chatloop.AttrChatKind, string(chatloop.ChatKindRoot)),
		)

		ended := fixture.spans.Ended()
		require.Len(t, ended, 1)
		require.Equal(t, string(chatloop.StageQueueWait), ended[0].Name())
		require.Equal(t, start.UTC(), ended[0].StartTime().UTC())
		require.Equal(t, end.UTC(), ended[0].EndTime().UTC())
		require.Equal(t, map[stageKey]uint64{
			{stage: chatloop.StageQueueWait, scope: chatloop.ScopeBackground}: 1,
		}, fixture.stageObservations(t))
		require.InDelta(t, 30, fixture.stageSum(t, chatloop.StageQueueWait), 0.001)
	})

	t.Run("DropsUnusableWindows", func(t *testing.T) {
		t.Parallel()
		fixture := newStageFixture(t)

		now := time.Now()
		fixture.tracer.Record(t.Context(), chatloop.StageAcquisition, chatloop.StageModel{}, time.Time{}, now, nil)
		fixture.tracer.Record(t.Context(), chatloop.StageAcquisition, chatloop.StageModel{}, now, time.Time{}, nil)
		fixture.tracer.Record(t.Context(), chatloop.StageAcquisition, chatloop.StageModel{}, now, now.Add(-time.Second), nil)

		require.Empty(t, fixture.spans.Ended())
		require.Empty(t, fixture.stageObservations(t))
		require.Equal(t, 3.0, fixture.anomalyCount(t, chatloop.StageAnomalyInvertedWindow))
	})
}

func TestStageSpanEndWithoutObservation(t *testing.T) {
	t.Parallel()
	fixture := newStageFixture(t)

	_, span := fixture.tracer.Start(t.Context(), chatloop.StageTimeToFirstToken)
	span.EndWithoutObservation(xerrors.New("stream ended before the first token"))
	// The first end wins, so a later End cannot revive the
	// observation.
	span.End(nil)

	ended := fixture.spans.Ended()
	require.Len(t, ended, 1)
	require.Equal(t, string(chatloop.StageTimeToFirstToken), ended[0].Name())
	require.Equal(t, codes.Error, ended[0].Status().Code)
	require.Empty(t, fixture.stageObservations(t))
}

func TestStageDurationBuckets(t *testing.T) {
	t.Parallel()

	// Alert thresholds are written against these edges, so both ladders
	// must contain them as round numbers rather than a generated ladder.
	alertEdges := []float64{1, 5, 10, 30, 60, 300, 3600}
	tests := []struct {
		name        string
		full        bool
		wantBuckets int
	}{
		{name: "basic", wantBuckets: 12},
		{name: "full", full: true, wantBuckets: 16},
	}
	var basicEdges []float64
	for _, tt := range tests {
		registry := prometheus.NewRegistry()
		metrics := chatloop.NewMetricsWithOptions(registry, chatloop.MetricsOptions{StageMetrics: true, FullStageMetrics: tt.full})
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
		require.Len(t, buckets, tt.wantBuckets, tt.name)
		// Both histograms use the same ladder.
		require.Len(t, modelBuckets, tt.wantBuckets, tt.name)
		edges := make([]float64, 0, len(buckets))
		for i, bucket := range buckets {
			require.Equal(t, bucket.GetUpperBound(), modelBuckets[i].GetUpperBound())
			edges = append(edges, bucket.GetUpperBound())
		}
		for _, want := range alertEdges {
			require.Contains(t, edges, want, "%s: bucket edge %v missing", tt.name, want)
		}
		// A 45 minute turn lands in the 1800-3600 bucket, not the overflow.
		for _, bucket := range buckets {
			if bucket.GetUpperBound() < 3600 {
				require.Zero(t, bucket.GetCumulativeCount(), "%s: le=%v", tt.name, bucket.GetUpperBound())
			} else {
				require.Equal(t, uint64(1), bucket.GetCumulativeCount(), "%s: le=%v", tt.name, bucket.GetUpperBound())
			}
		}
		if !tt.full {
			basicEdges = edges
			continue
		}
		// The full ladder is a superset of the basic one.
		for _, edge := range basicEdges {
			require.Contains(t, edges, edge, "full ladder missing basic edge %v", edge)
		}
	}
}

// TestStageMetricsEnabled covers which stage families and stages each
// combination of the stage metrics options exposes. Every setting must
// accept every recorder call, since the tracer does not know the
// setting.
func TestStageMetricsEnabled(t *testing.T) {
	t.Parallel()

	model := chatloop.StageModel{ProviderType: "p", Model: "m"}
	record := func(m *chatloop.Metrics) {
		m.RecordStageDuration(chatloop.StageTimeToFirstToken, chatloop.ScopeTurn, chatloop.ChatKindRoot, model, time.Second)
		m.RecordStageDuration(chatloop.StagePrepare, chatloop.ScopeTurn, chatloop.ChatKindRoot, model, time.Second)
		m.RecordStageDuration(chatloop.StageThinking, chatloop.ScopeTurn, chatloop.ChatKindRoot, model, time.Second)
		m.RecordStageDuration(chatloop.StageQueueWait, chatloop.ScopeTurn, chatloop.ChatKindRoot, chatloop.StageModel{}, time.Second)
		m.RecordStageDuration(chatloop.StageCommit, chatloop.ScopeTurn, chatloop.ChatKindRoot, chatloop.StageModel{}, -time.Second)
		m.RecordStageAnomaly(chatloop.StageAnomalyInvertedWindow)
	}

	basicStages := []chatloop.Stage{chatloop.StageQueueWait, chatloop.StageTimeToFirstToken}
	fullStages := []chatloop.Stage{chatloop.StagePrepare, chatloop.StageQueueWait, chatloop.StageThinking, chatloop.StageTimeToFirstToken}
	basicModelStages := []chatloop.Stage{chatloop.StageTimeToFirstToken}
	fullModelStages := []chatloop.Stage{chatloop.StageThinking, chatloop.StageTimeToFirstToken}
	tests := []struct {
		name            string
		opts            chatloop.MetricsOptions
		wantStages      []chatloop.Stage
		wantModelStages []chatloop.Stage
	}{
		{name: "off", opts: chatloop.MetricsOptions{}},
		// Full without the base option registers nothing.
		{name: "full_only", opts: chatloop.MetricsOptions{FullStageMetrics: true}},
		{
			name: "basic", opts: chatloop.MetricsOptions{StageMetrics: true},
			wantStages: basicStages, wantModelStages: basicModelStages,
		},
		{
			name: "full", opts: chatloop.MetricsOptions{StageMetrics: true, FullStageMetrics: true},
			wantStages: fullStages, wantModelStages: fullModelStages,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			registry := prometheus.NewRegistry()
			metrics := chatloop.NewMetricsWithOptions(registry, tt.opts)
			record(metrics)

			families, err := registry.Gather()
			require.NoError(t, err)
			byName := map[string]*dto.MetricFamily{}
			for _, family := range families {
				byName[family.GetName()] = family
			}

			wantFamily := tt.wantStages != nil
			_, hasDurations := byName["coderd_chatd_stage_duration_seconds"]
			_, hasModelDurations := byName["coderd_chatd_model_stage_duration_seconds"]
			_, hasAnomalies := byName["coderd_chatd_stage_anomalies_total"]
			require.Equal(t, wantFamily, hasDurations)
			require.Equal(t, wantFamily, hasModelDurations)
			require.Equal(t, wantFamily, hasAnomalies)
			if !wantFamily {
				return
			}

			var stages []chatloop.Stage
			for _, metric := range byName["coderd_chatd_stage_duration_seconds"].GetMetric() {
				stages = append(stages, chatloop.Stage(labelValue(metric, "stage")))
				require.Empty(t, labelValue(metric, "model"))
			}
			slices.Sort(stages)
			require.Equal(t, tt.wantStages, stages)

			var modelStages []chatloop.Stage
			for _, metric := range byName["coderd_chatd_model_stage_duration_seconds"].GetMetric() {
				modelStages = append(modelStages, chatloop.Stage(labelValue(metric, "stage")))
				require.Equal(t, model.ProviderType, labelValue(metric, "provider_type"))
				require.Equal(t, model.Model, labelValue(metric, "model"))
			}
			slices.Sort(modelStages)
			require.Equal(t, tt.wantModelStages, modelStages)

			// The negative commit and the inverted window are counted
			// even though commit itself would have been observed.
			anomalies := map[string]float64{}
			for _, metric := range byName["coderd_chatd_stage_anomalies_total"].GetMetric() {
				anomalies[labelValue(metric, "reason")] = metric.GetCounter().GetValue()
			}
			require.Equal(t, map[string]float64{
				chatloop.StageAnomalyNegativeElapsed: 1,
				chatloop.StageAnomalyInvertedWindow:  1,
			}, anomalies)
		})
	}
}

func TestStageTracerWithoutProvider(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	tracer := chatloop.NewStageTracer(nil, chatloop.NewMetrics(registry))
	_, span := tracer.Start(t.Context(), chatloop.StageToolCall)
	span.End(nil)
	tracer.Record(t.Context(), chatloop.StageCommit, chatloop.StageModel{}, time.Now().Add(-time.Second), time.Now(), nil)

	fixture := stageFixture{registry: registry}
	require.Equal(t, map[stageKey]uint64{
		{stage: chatloop.StageToolCall, scope: chatloop.ScopeBackground}: 1,
		{stage: chatloop.StageCommit, scope: chatloop.ScopeBackground}:   1,
	}, fixture.stageObservations(t))

	// A nil tracer discards everything and must not panic.
	var nilTracer *chatloop.StageTracer
	_, nilSpan := nilTracer.Start(t.Context(), chatloop.StageToolCall)
	nilSpan.End(nil)
	nilTracer.Record(t.Context(), chatloop.StageThinking, chatloop.StageModel{}, time.Now().Add(-time.Second), time.Now(), nil)
}

// TestStageTracerScopeWithoutProvider covers a metrics-only
// deployment: a no-op tracer produces invalid span contexts, and the
// scope must still follow the turn.
func TestStageTracerScopeWithoutProvider(t *testing.T) {
	t.Parallel()

	registry := prometheus.NewRegistry()
	tracer := chatloop.NewStageTracer(nil, chatloop.NewMetrics(registry))

	turnCtx, turn := tracer.StartRoot(t.Context(), chatloop.StageChatTurn, nil)
	require.False(t, turn.SpanContext().IsValid())
	stepCtx, step := tracer.Start(turnCtx, chatloop.StageGenerationStep)
	start := time.Now().Add(-time.Second)
	tracer.Record(stepCtx, chatloop.StageToolCall, chatloop.StageModel{}, start, time.Now(), nil)
	step.End(nil)
	turn.End(nil)

	fixture := stageFixture{registry: registry}
	require.Equal(t, map[stageKey]uint64{
		{stage: chatloop.StageChatTurn, scope: chatloop.ScopeTurn}: 1,
		{stage: chatloop.StageToolCall, scope: chatloop.ScopeTurn}: 1,
	}, fixture.stageObservations(t))
}
