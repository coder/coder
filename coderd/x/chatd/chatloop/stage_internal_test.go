package chatloop

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
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

func stageSeries(t *testing.T, registry *prometheus.Registry, family string, stage Stage) (*dto.Metric, bool) {
	t.Helper()
	for _, metric := range familyMetrics(t, registry, family) {
		if metricLabel(metric, "stage") == string(stage) {
			return metric, true
		}
	}
	return nil, false
}

func stageSampleCount(t *testing.T, registry *prometheus.Registry, stage Stage) (uint64, bool) {
	t.Helper()
	metric, ok := stageSeries(t, registry, "coderd_chatd_stage_duration_seconds", stage)
	return metric.GetHistogram().GetSampleCount(), ok
}

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

// spanStage returns the stage attribute, since GenAI spans are named after
// their operation.
func spanStage(span sdktrace.ReadOnlySpan) Stage {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == AttrStage {
			return Stage(attr.Value.AsString())
		}
	}
	return ""
}

func metricLabel(metric *dto.Metric, name string) string {
	for _, label := range metric.GetLabel() {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}

func endedSpan(t *testing.T, spans *tracetest.SpanRecorder, stage Stage) sdktrace.ReadOnlySpan {
	t.Helper()
	var found sdktrace.ReadOnlySpan
	for _, span := range spans.Ended() {
		if spanStage(span) != stage {
			continue
		}
		require.Nil(t, found, "more than one %s span was recorded", stage)
		found = span
	}
	require.NotNil(t, found, "no %s span was recorded", stage)
	return found
}

// requireEndedBefore checks the recorder's end order, since mock clock
// timestamps can be equal.
func requireEndedBefore(t *testing.T, spans *tracetest.SpanRecorder, first, second Stage) {
	t.Helper()
	var names []string
	for _, span := range spans.Ended() {
		if stage := spanStage(span); stage == first || stage == second {
			names = append(names, string(stage))
		}
	}
	require.Equal(t, []string{string(first), string(second)}, names)
}

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

func spanAttribute(t *testing.T, span sdktrace.ReadOnlySpan, key string) string {
	t.Helper()
	for _, attr := range span.Attributes() {
		if string(attr.Key) == key {
			return attr.Value.AsString()
		}
	}
	t.Fatalf("span %s has no %s attribute", span.Name(), key)
	return ""
}

func requireNoContentAttributes(t *testing.T, span sdktrace.ReadOnlySpan) {
	t.Helper()
	for _, attr := range span.Attributes() {
		switch string(attr.Key) {
		case "gen_ai.input.messages", "gen_ai.output.messages",
			"gen_ai.tool.call.arguments", "gen_ai.tool.call.result":
			t.Fatalf("span %s captured %s without content capture", span.Name(), attr.Key)
		}
	}
}

// recordingSampler records what the sampler sees when each span starts.
type recordingSampler struct {
	mu     sync.Mutex
	params []sdktrace.SamplingParameters
}

func (s *recordingSampler) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.Attributes = slices.Clone(p.Attributes)
	s.params = append(s.params, p)
	return sdktrace.SamplingResult{Decision: sdktrace.RecordAndSample}
}

func (*recordingSampler) Description() string { return "recording" }

func (s *recordingSampler) find(name string) (sdktrace.SamplingParameters, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, p := range s.params {
		if p.Name == name {
			return p, true
		}
	}
	return sdktrace.SamplingParameters{}, false
}

type stageMetricsFixture struct {
	tracer   *StageTracer
	spans    *tracetest.SpanRecorder
	registry *prometheus.Registry
	metrics  *Metrics
	clock    *quartz.Mock
}

func newStageMetricsFixture(t *testing.T, opts ...StageTracerOption) stageMetricsFixture {
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
		tracer:   NewStageTracer(provider, metrics, append([]StageTracerOption{WithClock(clock)}, opts...)...),
		spans:    recorder,
		registry: registry,
		metrics:  metrics,
		clock:    clock,
	}
}

func (f stageMetricsFixture) guardedStream(
	ctx context.Context,
	stageModel StageModel,
	open func(context.Context) (fantasy.StreamResponse, error),
) (guardedAttempt, error) {
	return guardedStream(ctx, "anthropic", "claude", f.clock, time.Minute, open, f.metrics, f.tracer, stageModel)
}

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
		_, ok := stageSeries(t, fixture.registry, "coderd_chatd_model_stage_duration_seconds", StageTimeToFirstToken)
		require.False(t, ok)
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
		_, ok := stageSeries(t, fixture.registry, "coderd_chatd_stage_duration_seconds", StageTimeToFirstToken)
		require.False(t, ok)
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

	t.Run("WarningsAndKeepalivesThenContentObservesOnce", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		attempt, err := fixture.guardedStream(t.Context(), StageModel{Model: "claude"},
			func(context.Context) (fantasy.StreamResponse, error) {
				return fantasy.StreamResponse(func(yield func(fantasy.StreamPart) bool) {
					if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeWarnings}) {
						return
					}
					if !yield(fantasy.StreamPart{Type: fantasy.StreamPartTypeKeepalive}) {
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

		// Warnings and keepalives do not close the window; the first text
		// part does.
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
	generateWithMessages := func(t *testing.T, fixture stageMetricsFixture, messages []fantasy.Message, streamFn func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)) error {
		t.Helper()
		model := &chattest.FakeModel{
			ProviderName: "anthropic",
			ModelName:    "claude",
			StreamFn:     streamFn,
		}
		_, err := GenerateAssistant(t.Context(), GenerateAssistantOptions{
			Model:      model,
			Messages:   messages,
			Clock:      fixture.clock,
			Metrics:    fixture.metrics,
			Stages:     fixture.tracer,
			StageModel: stageModel,
		})
		return err
	}
	generateWith := func(t *testing.T, fixture stageMetricsFixture, streamFn func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)) error {
		t.Helper()
		return generateWithMessages(t, fixture, []fantasy.Message{}, streamFn)
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
			{
				Type:         fantasy.StreamPartTypeFinish,
				ID:           "msg_1",
				FinishReason: fantasy.FinishReasonStop,
				Usage:        fantasy.Usage{InputTokens: 10, OutputTokens: 5, CacheReadTokens: 3, CacheCreationTokens: 2},
			},
		})
		require.NoError(t, err)

		stream := endedSpan(t, fixture.spans, StageStream)
		require.Equal(t, "chat claude", stream.Name())
		require.Equal(t, trace.SpanKindClient, stream.SpanKind())
		require.Subset(t, stream.Attributes(), []attribute.KeyValue{
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.provider.name", "aws.bedrock"),
			attribute.String("gen_ai.request.model", "claude"),
			attribute.Bool("gen_ai.request.stream", true),
			attribute.String("gen_ai.response.id", "msg_1"),
			attribute.StringSlice("gen_ai.response.finish_reasons", []string{"stop"}),
			attribute.Int("gen_ai.usage.input_tokens", 15),
			attribute.Int("gen_ai.usage.output_tokens", 5),
			attribute.Int("gen_ai.usage.cache_read.input_tokens", 3),
			attribute.Int("gen_ai.usage.cache_creation.input_tokens", 2),
		})
		requireNoContentAttributes(t, stream)
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

	t.Run("ContentCaptureRecordsOnlyTheStep", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))

		messages := []fantasy.Message{
			{Role: fantasy.MessageRoleSystem, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "system prompt"}}},
			{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "earlier prompt"}}},
			{Role: fantasy.MessageRoleAssistant, Content: []fantasy.MessagePart{fantasy.ToolCallPart{ToolCallID: "call-1", ToolName: "read_file", Input: `{"path":"a"}`}}},
			{Role: fantasy.MessageRoleTool, Content: []fantasy.MessagePart{fantasy.ToolResultPart{ToolCallID: "call-1", Output: fantasy.ToolResultOutputContentText{Text: "file body"}}}},
		}
		err := generateWithMessages(t, fixture, messages, func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		})
		require.NoError(t, err)

		stream := endedSpan(t, fixture.spans, StageStream)
		require.JSONEq(t,
			`[{"role":"tool","parts":[{"type":"tool_call_response","id":"call-1","response":"file body"}]}]`,
			spanAttribute(t, stream, "gen_ai.input.messages"))
		require.JSONEq(t,
			`[{"role":"assistant","parts":[{"type":"text","content":"done"}],"finish_reason":"stop"}]`,
			spanAttribute(t, stream, "gen_ai.output.messages"))
	})

	t.Run("ContentCaptureKeepsOversizedMessagesValidJSON", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))

		prompt := strings.Repeat(`"quoted" `, MaxContentAttributeBytes)
		err := generateWithMessages(t, fixture, []fantasy.Message{
			{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: prompt}}},
		}, func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{
				{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
				{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "done"},
				{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
				{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
			}), nil
		})
		require.NoError(t, err)

		input := spanAttribute(t, endedSpan(t, fixture.spans, StageStream), "gen_ai.input.messages")
		require.LessOrEqual(t, len(input), MaxContentAttributeBytes)
		var messages []genAIMessage
		require.NoError(t, json.Unmarshal([]byte(input), &messages))
		require.Len(t, messages, 1)
		require.Len(t, messages[0].Parts, 1)
		require.True(t, strings.HasPrefix(prompt, messages[0].Parts[0].Content))
	})

	t.Run("SamplerSeesModelAtCreation", func(t *testing.T) {
		t.Parallel()
		sampler := &recordingSampler{}
		provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler))
		t.Cleanup(func() {
			require.NoError(t, provider.Shutdown(context.Background()))
		})
		fixture := newStageMetricsFixture(t)
		fixture.tracer = NewStageTracer(provider, fixture.metrics, WithClock(fixture.clock))

		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextDelta, Delta: "hi"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		})
		require.NoError(t, err)

		params, ok := sampler.find("chat claude")
		require.True(t, ok, "no chat span was sampled under its final name")
		require.Equal(t, trace.SpanKindClient, params.Kind)
		require.Subset(t, params.Attributes, []attribute.KeyValue{
			attribute.String("gen_ai.operation.name", "chat"),
			attribute.String("gen_ai.provider.name", "aws.bedrock"),
			attribute.String("gen_ai.request.model", "claude"),
		})
	})

	t.Run("ContentCaptureKeepsRequiredEmptyFields", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))

		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "empty"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "empty"},
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "done"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		})
		require.NoError(t, err)

		require.JSONEq(t,
			`[{"role":"assistant","parts":[{"type":"text","content":""},{"type":"text","content":"done"}],"finish_reason":"stop"}]`,
			spanAttribute(t, endedSpan(t, fixture.spans, StageStream), "gen_ai.output.messages"))
	})

	t.Run("ContentCaptureAcceptsCleanCloseWithoutFinish", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))

		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "done"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
		})
		require.NoError(t, err)

		stream := endedSpan(t, fixture.spans, StageStream)
		require.Equal(t, codes.Unset, stream.Status().Code)
		require.JSONEq(t,
			`[{"role":"assistant","parts":[{"type":"text","content":"done"}],"finish_reason":"stop"}]`,
			spanAttribute(t, stream, "gen_ai.output.messages"))
	})

	t.Run("ContentCaptureOmitsProviderExecutedTools", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))

		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeToolCall, ID: "ws", ToolCallName: "web_search", ToolCallInput: `{}`, ProviderExecuted: true},
			{Type: fantasy.StreamPartTypeToolResult, ID: "ws", ToolCallName: "web_search", ProviderExecuted: true},
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "done"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonStop},
		})
		require.NoError(t, err)

		require.JSONEq(t,
			`[{"role":"assistant","parts":[{"type":"text","content":"done"}],"finish_reason":"stop"}]`,
			spanAttribute(t, endedSpan(t, fixture.spans, StageStream), "gen_ai.output.messages"))
	})

	t.Run("ContentCaptureRecordsPartialResponseFinishReason", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))

		streamErr := xerrors.New("provider dropped the connection")
		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "partial"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
			{Type: fantasy.StreamPartTypeError, Error: streamErr},
		})
		require.ErrorContains(t, err, streamErr.Error())

		require.JSONEq(t,
			`[{"role":"assistant","parts":[{"type":"text","content":"partial"}],"finish_reason":"error"}]`,
			spanAttribute(t, endedSpan(t, fixture.spans, StageStream), "gen_ai.output.messages"))
	})

	t.Run("ContentCaptureRecordsUnfinishedText", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))

		streamErr := xerrors.New("provider dropped the connection")
		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "partial"},
			{Type: fantasy.StreamPartTypeError, Error: streamErr},
		})
		require.ErrorContains(t, err, streamErr.Error())

		require.JSONEq(t,
			`[{"role":"assistant","parts":[{"type":"text","content":"partial"}],"finish_reason":"error"}]`,
			spanAttribute(t, endedSpan(t, fixture.spans, StageStream), "gen_ai.output.messages"))
	})

	t.Run("ContentCaptureMarksSilentTimeoutAsError", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitShort)
		fixture := newStageMetricsFixture(t, WithContentCapture(true))
		trap := fixture.clock.Trap().AfterFunc(streamSilenceGuardTimerTag)
		defer trap.Close()

		sent := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- generateWith(t, fixture, func(streamCtx context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return func(yield func(fantasy.StreamPart) bool) {
					for _, part := range []fantasy.StreamPart{
						{Type: fantasy.StreamPartTypeTextStart, ID: "t"},
						{Type: fantasy.StreamPartTypeTextDelta, ID: "t", Delta: "partial"},
						{Type: fantasy.StreamPartTypeTextEnd, ID: "t"},
					} {
						if !yield(part) {
							return
						}
					}
					close(sent)
					// A provider that notices the cancellation can return
					// without an error part.
					<-streamCtx.Done()
				}, nil
			})
		}()
		trap.MustWait(ctx).MustRelease(ctx)
		testutil.TryReceive(ctx, t, sent)
		fixture.clock.Advance(DefaultStreamSilenceTimeout).MustWait(ctx)
		require.Error(t, testutil.TryReceive(ctx, t, done))

		require.JSONEq(t,
			`[{"role":"assistant","parts":[{"type":"text","content":"partial"}],"finish_reason":"error"}]`,
			spanAttribute(t, endedSpan(t, fixture.spans, StageStream), "gen_ai.output.messages"))
	})

	t.Run("OpenFailureEndsStreamWithError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		openErr := xerrors.New("provider refused the request")
		err := generateWith(t, fixture, func(context.Context, fantasy.Call) (fantasy.StreamResponse, error) {
			return nil, openErr
		})
		require.ErrorContains(t, err, openErr.Error())

		stream := endedSpan(t, fixture.spans, StageStream)
		require.Equal(t, codes.Error, stream.Status().Code)
		require.Equal(t, err.Error(), recordedErrorMessage(t, stream))
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

		// With no output part, only the attempt release closes the TTFT
		// window.
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
		require.ErrorContains(t, err, streamErr.Error())

		stream := endedSpan(t, fixture.spans, StageStream)
		require.Equal(t, codes.Error, stream.Status().Code)
		require.Equal(t, err.Error(), recordedErrorMessage(t, stream))
		requireStreamObserved(t, fixture)
	})

	t.Run("ContentFilterEndsStreamWithError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonContentFilter},
		})
		require.ErrorIs(t, err, ErrContentFiltered)

		stream := endedSpan(t, fixture.spans, StageStream)
		require.Equal(t, codes.Error, stream.Status().Code)
		require.Equal(t, err.Error(), recordedErrorMessage(t, stream))
		requireEndedBefore(t, fixture.spans, StageTimeToFirstToken, StageStream)
		requireStreamObserved(t, fixture)
	})

	t.Run("NoOutputEndsStreamWithError", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)

		err := generate(t, fixture, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonUnknown},
		})
		require.ErrorIs(t, err, ErrNoModelOutput)

		stream := endedSpan(t, fixture.spans, StageStream)
		require.Equal(t, codes.Error, stream.Status().Code)
		require.Equal(t, err.Error(), recordedErrorMessage(t, stream))
		requireStreamObserved(t, fixture)
	})
}

func TestExecuteLocalToolsToolCallStage(t *testing.T) {
	t.Parallel()

	stageModel := StageModel{Provider: "openai", ProviderType: "openai", Model: "gpt-test", Effort: "high"}
	executeWithBuiltins := func(t *testing.T, fixture stageMetricsFixture, tool fantasy.AgentTool, builtins map[string]bool) PersistedStep {
		t.Helper()
		ctx := ContextWithScope(t.Context(), ScopeTurn)
		ctx, step := fixture.tracer.Start(ctx, StageGenerationStep)
		defer step.End(nil)
		result, err := ExecuteLocalTools(ctx, ExecuteLocalToolsOptions{
			Tools:            []fantasy.AgentTool{tool},
			ActiveTools:      []string{tool.Info().Name},
			ToolCalls:        []fantasy.ToolCallContent{{ToolCallID: "call-1", ToolName: tool.Info().Name, Input: "{}"}},
			BuiltinToolNames: builtins,
			ModelProvider:    "openai",
			ModelName:        "gpt-test",
			Stages:           fixture.tracer,
			StageModel:       stageModel,
			Clock:            fixture.clock,
		})
		require.NoError(t, err)
		return result
	}
	execute := func(t *testing.T, fixture stageMetricsFixture, tool fantasy.AgentTool) PersistedStep {
		t.Helper()
		return executeWithBuiltins(t, fixture, tool, nil)
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
		require.Equal(t, "execute_tool read_file", toolCall.Name())
		require.Subset(t, toolCall.Attributes(), []attribute.KeyValue{
			attribute.String("gen_ai.operation.name", "execute_tool"),
			attribute.String("gen_ai.tool.name", "read_file"),
			attribute.String("gen_ai.tool.call.id", "call-1"),
			attribute.String("gen_ai.tool.type", "extension"),
		})
		requireNoContentAttributes(t, toolCall)
		count, ok := stageSampleCount(t, fixture.registry, StageToolCall)
		require.True(t, ok)
		require.Equal(t, uint64(1), count)
	})

	t.Run("SamplerSeesToolAtCreation", func(t *testing.T) {
		t.Parallel()
		sampler := &recordingSampler{}
		provider := sdktrace.NewTracerProvider(sdktrace.WithSampler(sampler))
		t.Cleanup(func() {
			require.NoError(t, provider.Shutdown(context.Background()))
		})
		fixture := newStageMetricsFixture(t)
		fixture.tracer = NewStageTracer(provider, fixture.metrics, WithClock(fixture.clock))
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse("ok"), nil
			})

		executeWithBuiltins(t, fixture, tool, map[string]bool{"read_file": true})

		params, ok := sampler.find("execute_tool read_file")
		require.True(t, ok, "no tool span was sampled under its final name")
		require.Subset(t, params.Attributes, []attribute.KeyValue{
			attribute.String("gen_ai.operation.name", "execute_tool"),
			attribute.String("gen_ai.tool.name", "read_file"),
			attribute.String("gen_ai.tool.call.id", "call-1"),
			attribute.String("gen_ai.tool.type", "function"),
		})
	})

	t.Run("ContentCaptureRecordsPayloads", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse("file body"), nil
			})

		executeWithBuiltins(t, fixture, tool, map[string]bool{"read_file": true})

		toolCall := endedSpan(t, fixture.spans, StageToolCall)
		require.Subset(t, toolCall.Attributes(), []attribute.KeyValue{
			attribute.String("gen_ai.tool.type", "function"),
			attribute.String("gen_ai.tool.call.arguments", "{}"),
			attribute.String("gen_ai.tool.call.result", `"file body"`),
		})
	})

	t.Run("ContentCaptureKeepsOversizedArgumentsValidJSON", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t, WithContentCapture(true))
		tool := fantasy.NewAgentTool("write_file", "writes a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse(`{"body":"` + strings.Repeat("r", MaxContentAttributeBytes) + `"}`), nil
			})
		input := `{"content":"` + strings.Repeat(`\"a`, MaxContentAttributeBytes) + `"}`

		_, err := ExecuteLocalTools(ContextWithScope(t.Context(), ScopeTurn), ExecuteLocalToolsOptions{
			Tools:       []fantasy.AgentTool{tool},
			ActiveTools: []string{"write_file"},
			ToolCalls:   []fantasy.ToolCallContent{{ToolCallID: "call-1", ToolName: "write_file", Input: input}},
			// A large window keeps the result under the model's result
			// budget, so only the span limit applies.
			ContextLimit: 10_000_000,
			Stages:       fixture.tracer,
			Clock:        fixture.clock,
		})
		require.NoError(t, err)

		toolCall := endedSpan(t, fixture.spans, StageToolCall)
		for _, key := range []string{"gen_ai.tool.call.arguments", "gen_ai.tool.call.result"} {
			value := spanAttribute(t, toolCall, key)
			require.LessOrEqual(t, len(value), MaxContentAttributeBytes, key)
			require.True(t, json.Valid([]byte(value)), "%s is not valid JSON", key)
		}
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
		fixture := newStageMetricsFixture(t, WithContentCapture(true))
		runErr := xerrors.New("workspace agent unreachable")
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.ToolResponse{}, runErr
			})

		execute(t, fixture, tool)

		toolCall := endedSpan(t, fixture.spans, StageToolCall)
		require.Equal(t, codes.Error, toolCall.Status().Code)
		require.Equal(t, runErr.Error(), recordedErrorMessage(t, toolCall))
		require.NotEmpty(t, spanAttribute(t, toolCall, "error.type"))
		// The conventions define a result only for successful calls.
		for _, attr := range toolCall.Attributes() {
			require.NotEqual(t, "gen_ai.tool.call.result", string(attr.Key))
		}
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

	t.Run("RejectedCallsRecordNoStage", func(t *testing.T) {
		t.Parallel()
		fixture := newStageMetricsFixture(t)
		tool := fantasy.NewAgentTool("read_file", "reads a file",
			func(context.Context, struct{}, fantasy.ToolCall) (fantasy.ToolResponse, error) {
				return fantasy.NewTextResponse("ok"), nil
			})

		result, err := ExecuteLocalTools(ContextWithScope(t.Context(), ScopeTurn), ExecuteLocalToolsOptions{
			Tools:       []fantasy.AgentTool{tool},
			ActiveTools: []string{"missing"},
			ToolCalls: []fantasy.ToolCallContent{
				// read_file is inactive; missing has no tool.
				{ToolCallID: "call-1", ToolName: "read_file", Input: "{}"},
				{ToolCallID: "call-2", ToolName: "missing", Input: "{}"},
			},
			ModelProvider: "openai",
			ModelName:     "gpt-test",
			Stages:        fixture.tracer,
			StageModel:    stageModel,
			Clock:         fixture.clock,
		})
		require.NoError(t, err)

		require.Len(t, result.Content, 2)
		for _, content := range result.Content {
			toolResult, ok := content.(fantasy.ToolResultContent)
			require.True(t, ok)
			require.IsType(t, fantasy.ToolResultOutputContentError{}, toolResult.Result)
		}
		for _, span := range fixture.spans.Ended() {
			require.NotEqual(t, StageToolCall, spanStage(span))
		}
		_, ok := stageSampleCount(t, fixture.registry, StageToolCall)
		require.False(t, ok)
	})
}
