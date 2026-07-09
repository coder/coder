package chatloop

import (
	"context"
	"errors"
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func refusalProviderMetadataForTest(category, explanation string) fantasy.ProviderMetadata {
	return fantasy.ProviderMetadata{
		fantasyanthropic.Name: &fantasyanthropic.RefusalMetadata{
			Category:    category,
			Explanation: explanation,
		},
	}
}

func TestContentFilterError(t *testing.T) {
	t.Parallel()

	t.Run("WithRefusalMetadata", func(t *testing.T) {
		t.Parallel()

		err := contentFilterError("anthropic", refusalProviderMetadataForTest(
			"harmful_content", "The response was blocked. See https://example.com for help.",
		))
		require.ErrorIs(t, err, ErrContentFiltered)

		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindContentFilter, classified.Kind)
		require.Equal(t, "anthropic", classified.Provider)
		require.False(t, classified.Retryable)
		require.Equal(t, "Anthropic blocked this response under its content policy (harmful_content).", classified.Message)
		require.Equal(t, "The response was blocked. See https://example.com for help.", classified.Detail)
	})

	t.Run("WithoutMetadataFallsBackToDefaultMessage", func(t *testing.T) {
		t.Parallel()

		err := contentFilterError("anthropic", nil)
		require.ErrorIs(t, err, ErrContentFiltered)

		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindContentFilter, classified.Kind)
		require.False(t, classified.Retryable)
		require.Equal(t, "Anthropic blocked this response under its content policy.", classified.Message)
		require.Empty(t, classified.Detail)

		payload := chaterror.TerminalErrorPayload(classified)
		require.NotNil(t, payload)
		require.Equal(t, codersdk.ChatErrorKindContentFilter, payload.Kind)
	})
}

func TestGenerateAssistant_ContentFilterRefusal(t *testing.T) {
	t.Parallel()

	t.Run("EmptyContentSurfacesTerminalError", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "anthropic",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{{
					Type:         fantasy.StreamPartTypeFinish,
					FinishReason: fantasy.FinishReasonContentFilter,
					ProviderMetadata: refusalProviderMetadataForTest(
						"harmful_content", "The response was blocked.",
					),
				}}), nil
			},
		}

		outcome, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
		})
		require.ErrorIs(t, err, ErrContentFiltered)
		require.Empty(t, outcome.Step.Content)

		classified := chaterror.Classify(err)
		require.Equal(t, codersdk.ChatErrorKindContentFilter, classified.Kind)
		require.Equal(t, "anthropic", classified.Provider)
		require.False(t, classified.Retryable)
		require.Equal(t, "Anthropic blocked this response under its content policy (harmful_content).", classified.Message)
		require.Equal(t, "The response was blocked.", classified.Detail)
	})

	t.Run("PartialContentIsPersistedNotErrored", func(t *testing.T) {
		t.Parallel()

		model := &chattest.FakeModel{
			ProviderName: "anthropic",
			ModelName:    "test-model",
			StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
				return streamFromParts([]fantasy.StreamPart{
					{Type: fantasy.StreamPartTypeTextStart, ID: "text-1"},
					{Type: fantasy.StreamPartTypeTextDelta, ID: "text-1", Delta: "partial"},
					{Type: fantasy.StreamPartTypeTextEnd, ID: "text-1"},
					{
						Type:         fantasy.StreamPartTypeFinish,
						FinishReason: fantasy.FinishReasonContentFilter,
					},
				}), nil
			},
		}

		outcome, err := GenerateAssistant(context.Background(), GenerateAssistantOptions{
			Model: model,
			Messages: []fantasy.Message{
				textMessage(fantasy.MessageRoleUser, "hello"),
			},
		})
		require.NoError(t, err)
		require.False(t, errors.Is(err, ErrContentFiltered))
		require.NotEmpty(t, outcome.Step.Content)
	})
}
