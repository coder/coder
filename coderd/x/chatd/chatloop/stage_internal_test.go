package chatloop

import (
	"context"
	"testing"
	"time"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/xerrors"

	"github.com/coder/quartz"
)

// ttftStageCount returns the observation count of the
// time_to_first_token series on stage_duration_seconds, and whether the
// series exists at all.
func ttftStageCount(t *testing.T, registry *prometheus.Registry) (uint64, bool) {
	t.Helper()
	families, err := registry.Gather()
	require.NoError(t, err)
	for _, family := range families {
		if family.GetName() != "coderd_chatd_stage_duration_seconds" {
			continue
		}
		for _, metric := range family.GetMetric() {
			if stageLabel(metric) == StageTimeToFirstToken {
				return metric.GetHistogram().GetSampleCount(), true
			}
		}
	}
	return 0, false
}

func stageLabel(metric *dto.Metric) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == "stage" {
			return label.GetValue()
		}
	}
	return ""
}

func ttftSpanStatus(t *testing.T, spans *tracetest.SpanRecorder) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans.Ended() {
		if span.Name() == StageTimeToFirstToken {
			return span
		}
	}
	t.Fatalf("no %s span was recorded", StageTimeToFirstToken)
	return nil
}

func TestGuardedStreamTTFTStage(t *testing.T) {
	t.Parallel()

	newFixture := func(t *testing.T) (*StageTracer, *tracetest.SpanRecorder, *prometheus.Registry, *Metrics) {
		t.Helper()
		recorder := tracetest.NewSpanRecorder()
		provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
		t.Cleanup(func() {
			require.NoError(t, provider.Shutdown(context.Background()))
		})
		registry := prometheus.NewRegistry()
		metrics := NewMetrics(registry)
		return NewStageTracer(provider, metrics), recorder, registry, metrics
	}

	t.Run("FirstPartObservesStage", func(t *testing.T) {
		t.Parallel()
		stages, spans, registry, metrics := newFixture(t)

		attempt, err := guardedStream(
			t.Context(), "anthropic", "claude", quartz.NewMock(t), time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"})
				}), nil
			},
			metrics, stages, StageModel{Model: "claude", Effort: "high"},
		)
		require.NoError(t, err)
		parts := 0
		for range attempt.stream {
			parts++
		}
		require.Equal(t, 1, parts)
		attempt.release()

		count, ok := ttftStageCount(t, registry)
		require.True(t, ok)
		require.Equal(t, uint64(1), count)
		require.Equal(t, codes.Unset, ttftSpanStatus(t, spans).Status().Code)
	})

	t.Run("OpenFailureRecordsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		stages, spans, registry, metrics := newFixture(t)

		openErr := xerrors.New("provider refused the request")
		_, err := guardedStream(
			t.Context(), "anthropic", "claude", quartz.NewMock(t), time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return nil, openErr
			},
			metrics, stages, StageModel{Model: "claude"},
		)
		require.ErrorIs(t, err, openErr)

		require.Equal(t, codes.Error, ttftSpanStatus(t, spans).Status().Code)
		_, ok := ttftStageCount(t, registry)
		require.False(t, ok, "a failed window must not be observed")
	})

	t.Run("ReleaseWithoutPartRecordsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		stages, spans, registry, metrics := newFixture(t)

		attempt, err := guardedStream(
			t.Context(), "anthropic", "claude", quartz.NewMock(t), time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(func(fantasy.StreamPart) bool) {}), nil
			},
			metrics, stages, StageModel{Model: "claude"},
		)
		require.NoError(t, err)
		attempt.release()

		require.Equal(t, codes.Error, ttftSpanStatus(t, spans).Status().Code)
		_, ok := ttftStageCount(t, registry)
		require.False(t, ok, "a window closed without a token must not be observed")
	})
}
