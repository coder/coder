package chatloop

import (
	"context"
	"testing"

	"charm.land/fantasy"
	"github.com/prometheus/client_golang/prometheus"
	promtestutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestGenerateAssistant_SilentNoOutputFinish(t *testing.T) {
	t.Parallel()

	generate := func(t *testing.T, parts []fantasy.StreamPart) (AssistantOutcome, *Metrics, error) {
		t.Helper()
		model := &chattest.FakeModel{
			ProviderName: "google",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts(parts), nil
			},
		}
		metrics := NewMetrics(prometheus.NewRegistry())
		outcome, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
			Model:   model,
			Metrics: metrics,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
		})
		return outcome, metrics, err
	}

	// Reasoning that never receives ReasoningEnd mirrors the observed
	// Gemini failure: deltas streamed but no content accumulated.
	unterminatedReasoning := func(finish fantasy.FinishReason) []fantasy.StreamPart {
		return []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeReasoningStart, ID: "reasoning-1"},
			{Type: fantasy.StreamPartTypeReasoningDelta, ID: "reasoning-1", Delta: "planning"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: finish},
		}
	}

	t.Run("UnknownFinishWithoutOutputErrors", func(t *testing.T) {
		t.Parallel()

		outcome, metrics, err := generate(t, unterminatedReasoning(fantasy.FinishReasonUnknown))
		require.ErrorIs(t, err, ErrNoModelOutput)
		require.Empty(t, outcome.Step.Content)

		classified := chaterror.Classify(err)
		require.True(t, classified.Retryable)
		require.Equal(t, codersdk.ChatErrorKindGeneric, classified.Kind)
		require.Equal(t, "google", classified.Provider)
		require.Equal(t, "The model ended its response without producing any output.", classified.Message)
		require.Equal(t, "finish reason: unknown", classified.Detail)

		retries := promtestutil.ToFloat64(metrics.StreamRetriesTotal.WithLabelValues(
			"google", "test-model", string(codersdk.ChatErrorKindGeneric),
		))
		require.Equal(t, float64(1), retries)
	})

	t.Run("ReasoningOnlyContentWithUnknownFinishErrors", func(t *testing.T) {
		t.Parallel()

		_, _, err := generate(t, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeReasoningStart, ID: "reasoning-1"},
			{Type: fantasy.StreamPartTypeReasoningDelta, ID: "reasoning-1", Delta: "planning"},
			{Type: fantasy.StreamPartTypeReasoningEnd, ID: "reasoning-1"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonUnknown},
		})
		require.ErrorIs(t, err, ErrNoModelOutput)
	})

	t.Run("ToolCallsFinishWithoutToolCallsErrors", func(t *testing.T) {
		t.Parallel()

		_, _, err := generate(t, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
		})
		require.ErrorIs(t, err, ErrNoModelOutput)
		require.Equal(t, "finish reason: tool-calls", chaterror.Classify(err).Detail)
	})

	t.Run("StopFinishWithoutOutputCompletes", func(t *testing.T) {
		t.Parallel()

		outcome, metrics, err := generate(t, unterminatedReasoning(fantasy.FinishReasonStop))
		require.NoError(t, err)
		require.Empty(t, outcome.Step.Content)
		require.True(t, outcome.ModelStopped)

		retries := promtestutil.ToFloat64(metrics.StreamRetriesTotal.WithLabelValues(
			"google", "test-model", string(codersdk.ChatErrorKindGeneric),
		))
		require.Equal(t, float64(0), retries)
	})

	t.Run("LengthFinishWithoutOutputCompletes", func(t *testing.T) {
		t.Parallel()

		_, _, err := generate(t, unterminatedReasoning(fantasy.FinishReasonLength))
		require.NoError(t, err)
	})

	t.Run("EmptyTextWithUnknownFinishErrors", func(t *testing.T) {
		t.Parallel()

		_, _, err := generate(t, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonUnknown},
		})
		require.ErrorIs(t, err, ErrNoModelOutput)
	})

	t.Run("WhitespaceTextWithUnknownFinishErrors", func(t *testing.T) {
		t.Parallel()

		_, _, err := generate(t, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "  \n"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonUnknown},
		})
		require.ErrorIs(t, err, ErrNoModelOutput)
	})

	t.Run("UnknownFinishWithTextCompletes", func(t *testing.T) {
		t.Parallel()

		outcome, _, err := generate(t, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
			{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "answer"},
			{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonUnknown},
		})
		require.NoError(t, err)
		require.NotEmpty(t, outcome.Step.Content)
	})

	t.Run("ToolCallsFinishWithToolCallCompletes", func(t *testing.T) {
		t.Parallel()

		outcome, _, err := generate(t, []fantasy.StreamPart{
			{Type: fantasy.StreamPartTypeToolCall, ID: "call-1", ToolCallName: "do_thing", ToolCallInput: "{}"},
			{Type: fantasy.StreamPartTypeFinish, FinishReason: fantasy.FinishReasonToolCalls},
		})
		require.NoError(t, err)
		require.Len(t, outcome.ToolCalls, 1)
	})
}
