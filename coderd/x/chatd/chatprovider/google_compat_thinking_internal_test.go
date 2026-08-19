package chatprovider

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriteGoogleCompatThinkingConfig(t *testing.T) {
	t.Parallel()

	thinkingConfig := func(payload map[string]any) map[string]any {
		extraBody, _ := payload["extra_body"].(map[string]any)
		google, _ := extraBody["google"].(map[string]any)
		config, _ := google["thinking_config"].(map[string]any)
		return config
	}

	t.Run("Gemini3EffortBecomesThinkingLevel", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{"model": "gemini-3-flash-preview", "reasoning_effort": "medium"}
		require.True(t, rewriteGoogleCompatThinkingConfig(payload))
		require.NotContains(t, payload, "reasoning_effort")
		require.Equal(t, map[string]any{"include_thoughts": true, "thinking_level": "medium"}, thinkingConfig(payload))
	})

	t.Run("Gemini3ProClampsUnsupportedLevel", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{"model": "gemini-3.0-pro", "reasoning_effort": "minimal"}
		require.True(t, rewriteGoogleCompatThinkingConfig(payload))
		require.Equal(t, map[string]any{"include_thoughts": true, "thinking_level": "low"}, thinkingConfig(payload))
	})

	t.Run("PreGemini3EffortBecomesThinkingBudget", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{"model": "gemini-2.5-flash", "reasoning_effort": "medium"}
		require.True(t, rewriteGoogleCompatThinkingConfig(payload))
		require.NotContains(t, payload, "reasoning_effort")
		require.Equal(t, map[string]any{"include_thoughts": true, "thinking_budget": 8192}, thinkingConfig(payload))
	})

	t.Run("NoEffortStillIncludesThoughts", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{"model": "gemini-2.5-flash"}
		require.True(t, rewriteGoogleCompatThinkingConfig(payload))
		require.Equal(t, map[string]any{"include_thoughts": true}, thinkingConfig(payload))
	})

	t.Run("ProviderPrefixedModelID", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{"model": "google/gemini-3.1-pro-preview", "reasoning_effort": "high"}
		require.True(t, rewriteGoogleCompatThinkingConfig(payload))
		require.Equal(t, map[string]any{"include_thoughts": true, "thinking_level": "high"}, thinkingConfig(payload))
	})

	t.Run("ExplicitThinkingConfigWins", func(t *testing.T) {
		t.Parallel()
		pinned := map[string]any{"thinking_budget": float64(128)}
		payload := map[string]any{
			"model":            "gemini-2.5-pro",
			"reasoning_effort": "high",
			"extra_body":       map[string]any{"google": map[string]any{"thinking_config": pinned}},
		}
		require.True(t, rewriteGoogleCompatThinkingConfig(payload))
		require.NotContains(t, payload, "reasoning_effort")
		require.Equal(t, pinned, thinkingConfig(payload))
	})

	t.Run("NonGeminiUntouched", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{"model": "gpt-4o", "reasoning_effort": "high"}
		require.False(t, rewriteGoogleCompatThinkingConfig(payload))
		require.Equal(t, map[string]any{"model": "gpt-4o", "reasoning_effort": "high"}, payload)
	})

	t.Run("UnknownEffortUntouched", func(t *testing.T) {
		t.Parallel()
		payload := map[string]any{"model": "gemini-2.5-flash", "reasoning_effort": "turbo"}
		require.False(t, rewriteGoogleCompatThinkingConfig(payload))
		require.Contains(t, payload, "reasoning_effort")
	})
}
