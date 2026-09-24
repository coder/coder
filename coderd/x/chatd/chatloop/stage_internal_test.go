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
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// familyMetrics returns the series of the named family, or nil when it
// has none.
func familyMetrics(t *testing.T, registry *prometheus.Registry, name string) []*dto.Metric {
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

// stageSeries reports false when stage has no series in the family.
func stageSeries(t *testing.T, registry *prometheus.Registry, family string, stage Stage) (*dto.Metric, bool) {
	t.Helper()
	for _, metric := range familyMetrics(t, registry, family) {
		if metricLabel(metric, "stage") == string(stage) {
			return metric, true
		}
	}
	return nil, false
}

// stageSampleCount reports false when stage has no series.
func stageSampleCount(t *testing.T, registry *prometheus.Registry, stage Stage) (uint64, bool) {
	t.Helper()
	metric, ok := stageSeries(t, registry, "coderd_chatd_stage_duration_seconds", stage)
	return metric.GetHistogram().GetSampleCount(), ok
}

// histogramTotals sums the sample count and sum across every series of
// the named histogram family.
func histogramTotals(t *testing.T, registry *prometheus.Registry, name string) (uint64, float64) {
	t.Helper()
	var (
		count uint64
		sum   float64
	)
	for _, metric := range familyMetrics(t, registry, name) {
		count += metric.GetHistogram().GetSampleCount()
		sum += metric.GetHistogram().GetSampleSum()
	}
	return count, sum
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

// requireEndedBefore asserts that the first span ended before the
// second. The recorder lists spans in end order, which a mock clock's
// equal timestamps cannot show.
func requireEndedBefore(t *testing.T, spans *tracetest.SpanRecorder, first, second Stage) {
	t.Helper()
	var names []string
	for _, span := range spans.Ended() {
		if span.Name() == string(first) || span.Name() == string(second) {
			names = append(names, span.Name())
		}
	}
	require.Equal(t, []string{string(first), string(second)}, names)
}

// recordedErrorMessage returns the message of the span's first
// recorded error event.
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
	clock    *quartz.Mock
}

func newStageMetricsFixture(t *testing.T) stageMetricsFixture {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() {
		require.NoError(t, provider.Shutdown(context.Background()))
	})
	registry := prometheus.NewRegistry()
	metrics := NewMetricsWithOptions(registry, MetricsOptions{StageMetrics: true})
	clock := quartz.NewMock(t)
	return stageMetricsFixture{
		tracer:   NewStageTracer(provider, metrics, WithClock(clock)),
		spans:    recorder,
		registry: registry,
		metrics:  metrics,
		clock:    clock,
	}
}

// guardedStream opens an anthropic/claude attempt on the fixture's
// clock, tracer and metrics.
func (f stageMetricsFixture) guardedStream(
	ctx context.Context,
	model StageModel,
	open func(context.Context) (fantasy.StreamResponse, error),
) (guardedAttempt, error) {
	return guardedStream(ctx, "anthropic", "claude", f.clock, time.Minute, open, f.metrics, f.tracer, model)
}

// requireUnobservedTTFT asserts that the time_to_first_token span
// ended with wantErr and that neither TTFT histogram observed it.
func (f stageMetricsFixture) requireUnobservedTTFT(t *testing.T, wantErr string) {
	t.Helper()
	span := endedSpan(t, f.spans, StageTimeToFirstToken)
	require.Equal(t, codes.Error, span.Status().Code)
	require.Equal(t, wantErr, recordedErrorMessage(t, span))
	_, ok := stageSampleCount(t, f.registry, StageTimeToFirstToken)
	require.False(t, ok, "a window closed without an output part must not be observed")
	count, _ := histogramTotals(t, f.registry, "coderd_chatd_ttft_seconds")
	require.Zero(t, count)
}

// requireObservedTTFT asserts one successful time_to_first_token window
// of want on both TTFT histograms, which observe the same elapsed time.
func (f stageMetricsFixture) requireObservedTTFT(t *testing.T, want time.Duration) {
	t.Helper()
	require.Equal(t, codes.Unset, endedSpan(t, f.spans, StageTimeToFirstToken).Status().Code)
	stage, ok := stageSeries(t, f.registry, "coderd_chatd_stage_duration_seconds", StageTimeToFirstToken)
	require.True(t, ok)
	require.Equal(t, uint64(1), stage.GetHistogram().GetSampleCount())
	require.InDelta(t, want.Seconds(), stage.GetHistogram().GetSampleSum(), 1e-9)
	count, sum := histogramTotals(t, f.registry, "coderd_chatd_ttft_seconds")
	require.Equal(t, uint64(1), count)
	require.Equal(t, stage.GetHistogram().GetSampleSum(), sum)
}

// drainStream returns the number of parts consumed.
func drainStream(stream fantasy.StreamResponse) int {
	parts := 0
	for range stream {
		parts++
	}
	return parts
}

func TestGuardedStreamTTFTStage(t *testing.T) {
	t.Parallel()

	t.Run("FirstPartObservesStage", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		attempt, err := fixture.guardedStream(ContextWithScope(t.Context(), ScopeTurn),
			StageModel{Provider: "anthropic", ProviderType: "bedrock", Model: "claude", Effort: "high"},
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					fixture.clock.Advance(250 * time.Millisecond)
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"})
				}), nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, 1, drainStream(attempt.stream))
		attempt.release()

		fixture.requireObservedTTFT(t, 250*time.Millisecond)
		// Per-model time to first token is ttft_seconds only.
		_, ok := stageSeries(t, fixture.registry, "coderd_chatd_model_stage_duration_seconds", StageTimeToFirstToken)
		require.False(t, ok)
		// provider is the wire protocol and provider_type the configured
		// AI provider type.
		providers := map[attribute.Key][]string{}
		for _, attr := range endedSpan(t, fixture.spans, StageTimeToFirstToken).Attributes() {
			if attr.Key == AttrProvider || attr.Key == AttrProviderType {
				providers[attr.Key] = append(providers[attr.Key], attr.Value.AsString())
			}
		}
		require.Equal(t, map[attribute.Key][]string{
			AttrProvider:     {"anthropic"},
			AttrProviderType: {"bedrock"},
		}, providers)
	})

	t.Run("NilTracerObservesTTFTSeconds", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		attempt, err := guardedStream(t.Context(), "anthropic", "claude", fixture.clock, time.Minute,
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					fixture.clock.Advance(250 * time.Millisecond)
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"})
				}), nil
			},
			fixture.metrics, nil, StageModel{},
		)
		require.NoError(t, err)
		drainStream(attempt.stream)
		attempt.release()

		count, sum := histogramTotals(t, fixture.registry, "coderd_chatd_ttft_seconds")
		require.Equal(t, uint64(1), count)
		require.InDelta(t, 0.25, sum, 1e-9)
		require.Empty(t, fixture.spans.Ended())
	})

	t.Run("OpenFailureEndsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		openErr := xerrors.New("provider refused the request")
		_, err := fixture.guardedStream(t.Context(), StageModel{Model: "claude"},
			func(context.Context) (fantasy.StreamResponse, error) {
				return nil, openErr
			},
		)
		require.ErrorIs(t, err, openErr)

		fixture.requireUnobservedTTFT(t, openErr.Error())
	})

	t.Run("WarningsThenErrorEndsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		streamErr := xerrors.New("provider returned 429")
		attempt, err := fixture.guardedStream(t.Context(), StageModel{Model: "claude"},
			func(context.Context) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeWarnings},
					{Type: fantasy.StreamPartTypeError, Error: streamErr},
				}), nil
			},
		)
		require.NoError(t, err)
		require.Equal(t, 2, drainStream(attempt.stream))
		attempt.release()

		fixture.requireUnobservedTTFT(t, streamErr.Error())
	})

	t.Run("WarningsThenContentObservesOnce", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		attempt, err := fixture.guardedStream(t.Context(), StageModel{Model: "claude"},
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeWarnings}) {
						return
					}
					fixture.clock.Advance(time.Second)
					if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"}) {
						return
					}
					fixture.clock.Advance(time.Second)
					if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeTextDelta, Delta: " there"}) {
						return
					}
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop})
				}), nil
			},
		)
		require.NoError(t, err)
		drainStream(attempt.stream)
		attempt.release()

		// The window closes at the first text part, after the warnings
		// part, and the second text part does not observe again.
		fixture.requireObservedTTFT(t, time.Second)
	})

	t.Run("StartMarkerThenErrorObservesOnStartMarker", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		streamErr := xerrors.New("provider returned 500")
		attempt, err := fixture.guardedStream(t.Context(), StageModel{Model: "claude"},
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					fixture.clock.Advance(time.Second)
					if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeReasoningStart, ID: "r1"}) {
						return
					}
					fixture.clock.Advance(time.Second)
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: streamErr})
				}), nil
			},
		)
		require.NoError(t, err)
		drainStream(attempt.stream)
		attempt.release()

		// A start marker is a streamed output part, so the window
		// closes on it and the later error does not reopen it.
		fixture.requireObservedTTFT(t, time.Second)
	})

	t.Run("SilenceTimeoutClassifiesSpanError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		ctx := testutil.Context(t, testutil.WaitShort)

		attempt, err := fixture.guardedStream(ctx, StageModel{Model: "claude"},
			func(attemptCtx context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					fixture.clock.Advance(time.Minute).MustWait(ctx)
					yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeError, Error: attemptCtx.Err()})
				}), nil
			},
		)
		require.NoError(t, err)
		drainStream(attempt.stream)
		finishErr := attempt.finish(nil)
		attempt.release()
		require.ErrorIs(t, finishErr, errStreamSilenceTimeout)

		fixture.requireUnobservedTTFT(t, errStreamSilenceTimeout.Error())
	})

	t.Run("FinishWithoutContentEndsSpanWithoutObservation", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		attempt, err := fixture.guardedStream(t.Context(), StageModel{Model: "claude"},
			func(context.Context) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
				}), nil
			},
		)
		require.NoError(t, err)
		drainStream(attempt.stream)
		attempt.release()

		fixture.requireUnobservedTTFT(t, errNoFirstToken.Error())
	})
}

func TestGenerateAssistantStreamStage(t *testing.T) {
	t.Parallel()

	stageModel := StageModel{Provider: "anthropic", ProviderType: "bedrock", Model: "claude", Effort: "high"}
	generateWith := func(t *testing.T, fixture stageMetricsFixture, streamFn func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)) error {
		t.Helper()
		model := &chattest.FakeModel{
			ProviderName: "anthropic",
			ModelName:    "claude",
			StreamFn:     streamFn,
		}
		_, err := GenerateAssistant(t.Context(), GenerateAssistantOptions{
			Model:      model,
			Messages:   []fantasy.Message{},
			Clock:      fixture.clock,
			Metrics:    fixture.metrics,
			Stages:     fixture.tracer,
			StageModel: stageModel,
		})
		return err
	}

	generate := func(t *testing.T, fixture stageMetricsFixture, parts []fantasy.StreamPart) error {
		t.Helper()
		return generateWith(t, fixture, func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts(parts), nil
		})
	}

	requireStreamObserved := func(t *testing.T, fixture stageMetricsFixture) {
		t.Helper()
		count, ok := stageSampleCount(t, fixture.registry, StageStream)
		require.True(t, ok)
		require.Equal(t, uint64(1), count)
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
		requireEndedBefore(t, fixture.spans, StageTimeToFirstToken, StageStream)
		require.Subset(t, stream.Attributes(), []attribute.KeyValue{
			attribute.String(AttrProvider, "anthropic"),
			attribute.String(AttrProviderType, stageModel.ProviderType),
			attribute.String(AttrModel, stageModel.Model),
			attribute.String(AttrReasoningEffort, stageModel.Effort),
		})
		requireStreamObserved(t, fixture)
	})

	t.Run("OpenFailureEndsStreamWithError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		openErr := xerrors.New("provider refused the request")
		err := generateWith(t, fixture, func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			return nil, openErr
		})
		require.ErrorContains(t, err, openErr.Error())

		require.Equal(t, codes.Error, endedSpan(t, fixture.spans, StageStream).Status().Code)
		require.Equal(t, codes.Error, endedSpan(t, fixture.spans, StageTimeToFirstToken).Status().Code)
		requireEndedBefore(t, fixture.spans, StageTimeToFirstToken, StageStream)
		requireStreamObserved(t, fixture)
	})

	t.Run("EmptyStreamClosesTTFTBeforeStream", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		_ = generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		})

		// Only the attempt release closes this window, and it must do so
		// before the stream stage ends.
		requireEndedBefore(t, fixture.spans, StageTimeToFirstToken, StageStream)
		requireStreamObserved(t, fixture)
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
		requireStreamObserved(t, fixture)
	})
}

func TestExecuteLocalToolsToolCallStage(t *testing.T) {
	t.Parallel()

	stageModel := StageModel{ProviderType: "openai", Model: "gpt-test", Effort: "high"}
	execute := func(t *testing.T, fixture stageMetricsFixture, tool fantasy.AgentTool) PersistedStep {
		t.Helper()
		ctx := ContextWithScope(t.Context(), ScopeTurn)
		ctx, step := fixture.tracer.Start(ctx, StageGenerationStep)
		defer step.End(nil)
		result, err := ExecuteLocalTools(ctx, ExecuteLocalToolsOptions{
			Tools:         []fantasy.AgentTool{tool},
			ActiveTools:   []string{tool.Info().Name},
			ToolCalls:     []fantasy.ToolCallContent{{ToolCallID: "call-1", ToolName: tool.Info().Name, Input: "{}"}},
			ModelProvider: "openai",
			ModelName:     "gpt-test",
			Stages:        fixture.tracer,
			StageModel:    stageModel,
			Clock:         fixture.clock,
		})
		require.NoError(t, err)
		return result
	}

	t.Run("RunsToolOnStageContext", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		var toolSpanID string
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
				toolSpanID = trace.SpanContextFromContext(ctx).SpanID().String()
				return fantasy.NewTextResponse("ok"), nil
			})

		execute(t, fixture, tool)

		toolCall := endedSpan(t, fixture.spans, StageToolCall)
		step := endedSpan(t, fixture.spans, StageGenerationStep)
		require.Equal(t, step.SpanContext().SpanID(), toolCall.Parent().SpanID())
		require.Equal(t, toolCall.SpanContext().SpanID().String(), toolSpanID)
		require.Equal(t, codes.Unset, toolCall.Status().Code)
		require.Contains(t, toolCall.Attributes(), attribute.String(AttrToolName, "read_file"))
		require.Contains(t, toolCall.Attributes(), attribute.String(AttrProvider, "openai"))
		require.Contains(t, toolCall.Attributes(), attribute.String(AttrModel, "gpt-test"))
		require.Contains(t, toolCall.Attributes(), attribute.String(AttrScope, string(ScopeTurn)))
		count, ok := stageSampleCount(t, fixture.registry, StageToolCall)
		require.True(t, ok)
		require.Equal(t, uint64(1), count)
	})

	t.Run("ErrorResultEndsWithoutError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextErrorResponse("file not found"), nil
			})

		result := execute(t, fixture, tool)

		require.Len(t, result.Content, 1)
		toolResult, ok := result.Content[0].(fantasy.ToolResultContent)
		require.True(t, ok)
		require.IsType(t, fantasy.ToolResultOutputContentError{}, toolResult.Result)
		require.Equal(t, codes.Unset, endedSpan(t, fixture.spans, StageToolCall).Status().Code)
	})

	t.Run("ExecutionFailureEndsWithError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		runErr := xerrors.New("workspace agent unreachable")
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{}, runErr
			})

		execute(t, fixture, tool)

		toolCall := endedSpan(t, fixture.spans, StageToolCall)
		require.Equal(t, codes.Error, toolCall.Status().Code)
		require.Equal(t, runErr.Error(), recordedErrorMessage(t, toolCall))
	})

	t.Run("PanicEndsWithError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				panic("boom")
			})

		execute(t, fixture, tool)

		toolCall := endedSpan(t, fixture.spans, StageToolCall)
		require.Equal(t, codes.Error, toolCall.Status().Code)
		require.Contains(t, recordedErrorMessage(t, toolCall), "boom")
	})
}
