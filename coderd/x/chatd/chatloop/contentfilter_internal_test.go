package chatloop

import (
	"context"
	"testing"

	fantasy "charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chaterror"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestContentFilterError(t *testing.T) {
	t.Parallel()

	t.Run("WithRefusalMetadata", func(t *testing.T) {
		t.Parallel()
		meta := fantasy.ProviderMetadata{
			fantasyanthropic.Name: &fantasyanthropic.RefusalMetadata{
				Category:    "cyber",
				Explanation: "blocked under policy",
			},
		}
		err := contentFilterError("anthropic", meta)
		classified := chaterror.Classify(err)
		if classified.Kind != codersdk.ChatErrorKindContentFilter {
			t.Errorf("kind = %q, want content_filter", classified.Kind)
		}
		if classified.Message != "blocked under policy" {
			t.Errorf("message = %q, want %q", classified.Message, "blocked under policy")
		}
		if classified.Detail != "cyber" {
			t.Errorf("detail = %q, want %q", classified.Detail, "cyber")
		}
	})

	t.Run("WithoutMetadataUsesDefault", func(t *testing.T) {
		t.Parallel()
		err := contentFilterError("anthropic", nil)
		classified := chaterror.Classify(err)
		if classified.Kind != codersdk.ChatErrorKindContentFilter {
			t.Errorf("kind = %q, want content_filter", classified.Kind)
		}
		want := "Anthropic blocked this response under its content policy."
		if classified.Message != want {
			t.Errorf("message = %q, want %q", classified.Message, want)
		}
	})
}

// TestRun_ContentFilterEmptyTurn exercises the branch in Run that converts
// a content-filter finish with no content into a terminal classified error
// instead of a silent empty turn. Nothing is persisted for the blocked step.
func TestRun_ContentFilterEmptyTurn(t *testing.T) {
	t.Parallel()

	model := &chattest.FakeModel{
		ProviderName: fantasyanthropic.Name,
		StreamFn: func(_ context.Context, _ fantasy.Call) (fantasy.StreamResponse, error) {
			return streamFromParts([]fantasy.StreamPart{{
				Type:         fantasy.StreamPartTypeFinish,
				FinishReason: fantasy.FinishReasonContentFilter,
				ProviderMetadata: fantasy.ProviderMetadata{
					fantasyanthropic.Name: &fantasyanthropic.RefusalMetadata{
						Category:    "cyber",
						Explanation: "blocked under policy",
					},
				},
			}}), nil
		},
	}

	persisted := false
	err := Run(context.Background(), RunOptions{
		Model: model,
		Messages: []fantasy.Message{
			textMessage(fantasy.MessageRoleUser, "hello"),
		},
		MaxSteps:             1,
		ContextLimitFallback: 4096,
		PersistStep: func(_ context.Context, _ PersistedStep) error {
			persisted = true
			return nil
		},
	})
	require.ErrorIs(t, err, ErrContentFiltered)

	classified := chaterror.Classify(err)
	require.Equal(t, codersdk.ChatErrorKindContentFilter, classified.Kind)
	require.Equal(t, "blocked under policy", classified.Message)
	require.Equal(t, "cyber", classified.Detail)
	require.False(t, classified.Retryable)
	require.False(t, persisted, "blocked turn must not persist a step")
}
