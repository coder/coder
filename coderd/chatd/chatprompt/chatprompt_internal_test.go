package chatprompt

import (
	"testing"

	"charm.land/fantasy"
	fantasyanthropic "charm.land/fantasy/providers/anthropic"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
)

func TestSanitizeOpenAIReplayMetadata(t *testing.T) {
	t.Parallel()

	t.Run("ReasoningMetadata", func(t *testing.T) {
		t.Parallel()

		encryptedContent := "encrypted"
		metadata := &fantasyopenai.ResponsesReasoningMetadata{
			ItemID:           "rs_123",
			EncryptedContent: &encryptedContent,
			Summary:          []string{"step one", "step two"},
		}
		opts := sanitizeOpenAIReplayMetadata(fantasy.ProviderOptions{
			fantasyopenai.Name: metadata,
		})

		reasoning, ok := opts[fantasyopenai.Name].(*fantasyopenai.ResponsesReasoningMetadata)
		require.True(t, ok)
		require.Empty(t, reasoning.ItemID)
		require.Equal(t, []string{"step one", "step two"}, reasoning.Summary)
		require.NotNil(t, reasoning.EncryptedContent)
		require.Equal(t, encryptedContent, *reasoning.EncryptedContent)
	})

	t.Run("WebSearchMetadata", func(t *testing.T) {
		t.Parallel()

		action := &fantasyopenai.WebSearchAction{Type: "search", Query: "coder"}
		metadata := &fantasyopenai.WebSearchCallMetadata{
			ItemID: "ws_123",
			Action: action,
		}
		opts := sanitizeOpenAIReplayMetadata(fantasy.ProviderOptions{
			fantasyopenai.Name: metadata,
		})

		webSearch, ok := opts[fantasyopenai.Name].(*fantasyopenai.WebSearchCallMetadata)
		require.True(t, ok)
		require.Empty(t, webSearch.ItemID)
		require.Same(t, action, webSearch.Action)
		require.Equal(t, "search", webSearch.Action.Type)
		require.Equal(t, "coder", webSearch.Action.Query)
	})

	t.Run("NonOpenAIMetadataUntouched", func(t *testing.T) {
		t.Parallel()

		cacheControl := &fantasyanthropic.ProviderCacheControlOptions{
			CacheControl: fantasyanthropic.CacheControl{Type: "ephemeral"},
		}
		opts := sanitizeOpenAIReplayMetadata(fantasy.ProviderOptions{
			"anthropic": cacheControl,
		})

		anthropicMetadata, ok := opts["anthropic"].(*fantasyanthropic.ProviderCacheControlOptions)
		require.True(t, ok)
		require.Equal(t, "ephemeral", anthropicMetadata.CacheControl.Type)
	})

	t.Run("ProviderMetadataToOptionsRoundTrip", func(t *testing.T) {
		t.Parallel()

		encryptedContent := "round-trip"
		raw := marshalProviderMetadata(fantasy.ProviderMetadata{
			fantasyopenai.Name: &fantasyopenai.ResponsesReasoningMetadata{
				ItemID:           "rs_roundtrip",
				EncryptedContent: &encryptedContent,
				Summary:          []string{"summary"},
			},
		})
		require.NotNil(t, raw)

		opts := providerMetadataToOptions(slogtest.Make(t, nil), raw)
		require.NotNil(t, opts)

		reasoning, ok := opts[fantasyopenai.Name].(*fantasyopenai.ResponsesReasoningMetadata)
		require.True(t, ok)
		require.Empty(t, reasoning.ItemID)
		require.Equal(t, []string{"summary"}, reasoning.Summary)
		require.NotNil(t, reasoning.EncryptedContent)
		require.Equal(t, encryptedContent, *reasoning.EncryptedContent)
	})
}
