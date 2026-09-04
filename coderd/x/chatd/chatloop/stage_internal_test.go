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
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/quartz"
)

// stageSampleCount returns the observation count of stage on
// coderd_chatd_stage_duration_seconds, and whether the series exists.
func stageSampleCount(t *testing.T, registry *prometheus.Registry, stage Stage) (uint64, bool) {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabel(metric, "stage") == string(stage) {
				return metric.GetHistogram().GetSampleCount(), true
			}
		}
	}
	return 0, false
}

// modelStageSeries returns the single coderd_chatd_model_stage_duration_seconds
// series for stage, and whether it exists.
func modelStageSeries(t *testing.T, registry *prometheus.Registry, stage Stage) (*dto.Metric, bool) {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "coderd_chatd_model_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if metricLabel(metric, "stage") == string(stage) {
				return metric, true
			}
		}
	}
	return nil, false
}

// ttftSampleCount returns the total observation count across every
// coderd_chatd_ttft_seconds series.
func ttftSampleCount(t *testing.T, registry *prometheus.Registry) uint64 {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	var count uint64
	for _, family := range families {
		if family.GetName() != "coderd_chatd_ttft_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			count += metric.GetHistogram().GetSampleCount()
		}
	}
	return count
}

func metricLabel(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

// endedSpan returns the single ended span named stage.
func endedSpan(t *testing.T, spans *tracetest.SpanRecorder, stage Stage) sdktrace.ReadOnlySpan {
	t.Helper()
	var found sdktrace.ReadOnlySpan
	for _, span := range spans.Ended() {
		if span.Name() != string(stage) {
			continue
		}
		require.Nil(t, found, "more than one %s span was recorded", stage)
		found = span
	}
	require.NotNil(t, found, "no %s span was recorded", stage)
	return found
}

// recordedErrorMessage returns the exception message of the span's
// single recorded error event.
func recordedErrorMessage(t *testing.T, span sdktrace.ReadOnlySpan) string {
	t.Helper()
	for _, event := range span.Events() {
		if event.Name != semconv.ExceptionEventName {
			continue
		}
		for _, attr := range event.Attributes {
			if attr.Key == semconv.ExceptionMessageKey {
				return attr.Value.AsString()
			}
		}
	}
	t.Fatalf("span %s recorded no error", span.Name())
	return ""
}

type stageMetricsFixture struct {
	tracer   *StageTracer
	spans    *tracetest.SpanRecorder
	registry *prometheus.Registry
	metrics  *Metrics
}

func newStageMetricsFixture(t *testing.T) stageMetricsFixture {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	registry := prometheus.NewRegistry()
	metrics := NewMetrics(registry)
	return stageMetricsFixture{
		tracer:   NewStageTracer(provider, metrics),
		spans:    recorder,
		registry: registry,
		metrics:  metrics,
	}
}

func TestGuardedStreamTTFTStage(t *testing.T) {
	t.Parallel()

	t.Run("FirstPartObservesStage", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		attempt, err := guardedStream(
			t.Context(), "anthropic", "claude", quartz.NewMock(t), time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"})
				}), nil
			},
			fixture.metrics, fixture.tracer, StageModel{ProviderType: "bedrock", Model: "claude", Effort: "high"},
		)
		require.NoError(t, err)
		parts := 0
		for range attempt.stream {
			parts++
		}
		require.Equal(t, 1, parts)
		attempt.release()

		count, ok := stageSampleCount(t, fixture.registry, StageTimeToFirstToken)
		require.True(t, ok)
		require.Equal(t, uint64(1), count)
		modelSeries, ok := modelStageSeries(t, fixture.registry, StageTimeToFirstToken)
		require.True(t, ok)
		require.Equal(t, uint64(1), modelSeries.GetHistogram().GetSampleCount())
		require.Equal(t, "bedrock", metricLabel(modelSeries, "provider_type"))
		require.Equal(t, "claude", metricLabel(modelSeries, "model"))
		require.Equal(t, uint64(1), ttftSampleCount(t, fixture.registry))
		ttftSpan := endedSpan(t, fixture.spans, StageTimeToFirstToken)
		require.Equal(t, codes.Unset, ttftSpan.Status().Code)
		// The span carries the transport provider and the configured
		// provider type as separate attributes, each once.
		providers := map[attribute.Key][]string{}
		for _, attr := range ttftSpan.Attributes() {
			if attr.Key == AttrProvider || attr.Key == AttrProviderType {
				providers[attr.Key] = append(providers[attr.Key], attr.Value.AsString())
			}
		}
		require.Equal(t, map[attribute.Key][]string{
			AttrProvider:     {"anthropic"},
			AttrProviderType: {"bedrock"},
		}, providers)
	})

	t.Run("OpenFailureEndsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		openErr := xerrors.New("provider refused the request")
		_, err := guardedStream(
			t.Context(), "anthropic", "claude", quartz.NewMock(t), time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return nil, openErr
			},
			fixture.metrics, fixture.tracer, StageModel{Model: "claude"},
		)
		require.ErrorIs(t, err, openErr)

		span := endedSpan(t, fixture.spans, StageTimeToFirstToken)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, openErr.Error(), recordedErrorMessage(t, span))
		_, ok := stageSampleCount(t, fixture.registry, StageTimeToFirstToken)
		require.False(t, ok, "a failed window must not be observed")
		require.Zero(t, ttftSampleCount(t, fixture.registry), "a failed window is not model latency")
	})

	t.Run("ErrorPartEndsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		streamErr := xerrors.New("provider returned 500")
		attempt, err := guardedStream(
			t.Context(), "anthropic", "claude", quartz.NewMock(t), time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: streamErr})
				}), nil
			},
			fixture.metrics, fixture.tracer, StageModel{Model: "claude"},
		)
		require.NoError(t, err)
		parts := 0
		for range attempt.stream {
			parts++
		}
		require.Equal(t, 1, parts)
		attempt.release()

		span := endedSpan(t, fixture.spans, StageTimeToFirstToken)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, streamErr.Error(), recordedErrorMessage(t, span))
		_, ok := stageSampleCount(t, fixture.registry, StageTimeToFirstToken)
		require.False(t, ok, "an error part must not be observed")
		require.Zero(t, ttftSampleCount(t, fixture.registry), "an error part is not a first token")
	})

	t.Run("ReleaseWithoutPartRecordsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		attempt, err := guardedStream(
			t.Context(), "anthropic", "claude", quartz.NewMock(t), time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(func(fantasy.StreamPart) bool) {}), nil
			},
			fixture.metrics, fixture.tracer, StageModel{Model: "claude"},
		)
		require.NoError(t, err)
		attempt.release()

		span := endedSpan(t, fixture.spans, StageTimeToFirstToken)
		require.Equal(t, codes.Error, span.Status().Code)
		require.Equal(t, errNoFirstToken.Error(), recordedErrorMessage(t, span))
		_, ok := stageSampleCount(t, fixture.registry, StageTimeToFirstToken)
		require.False(t, ok, "a window closed without a token must not be observed")
		require.Zero(t, ttftSampleCount(t, fixture.registry))
	})
}

func TestGenerateAssistantStreamStage(t *testing.T) {
	t.Parallel()

	generate := func(t *testing.T, fixture stageMetricsFixture, parts []fantasy.StreamPart) error {
		t.Helper()
		model := &chattest.FakeModel{
			ProviderName: "anthropic",
			ModelName:    "claude",
			StreamFn: func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts(parts), nil
			},
		}
		_, err := GenerateAssistant(t.Context(), GenerateAssistantOptions{
			Model:      model,
			Messages:   []fantasy.Message{},
			Clock:      quartz.NewMock(t),
			Metrics:    fixture.metrics,
			Stages:     fixture.tracer,
			StageModel: StageModel{Model: "claude"},
		})
		return err
	}

	t.Run("SuccessEndsStreamAfterFirstToken", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		})
		require.NoError(t, err)

		stream := endedSpan(t, fixture.spans, StageStream)
		ttft := endedSpan(t, fixture.spans, StageTimeToFirstToken)
		require.Equal(t, codes.Unset, stream.Status().Code)
		require.Equal(t, codes.Unset, ttft.Status().Code)
		require.Equal(t, stream.SpanContext().SpanID(), ttft.Parent().SpanID())
		require.False(t, ttft.EndTime().After(stream.EndTime()))
		count, ok := stageSampleCount(t, fixture.registry, StageStream)
		require.True(t, ok)
		require.Equal(t, uint64(1), count)
	})

	t.Run("StreamErrorEndsStreamWithError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		streamErr := xerrors.New("provider dropped the connection")
		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"},
			{Type: fantasy.StreamPartTypeError, Error: streamErr},
		})
		require.Error(t, err)

		stream := endedSpan(t, fixture.spans, StageStream)
		require.Equal(t, codes.Error, stream.Status().Code)
		require.Equal(t, streamErr.Error(), recordedErrorMessage(t, stream))
		count, ok := stageSampleCount(t, fixture.registry, StageStream)
		require.True(t, ok)
		require.Equal(t, uint64(1), count)
	})
}
