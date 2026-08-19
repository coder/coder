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
		for _, modelID := range []string{
			"gemini-2.5-flash",
			"gemini-2.5-flash-lite",
			"gemini-2.5-flash-preview-09-2025",
			"gemini-2.5-pro-preview-06-05",
		} {
			payload := map[string]any{"model": modelID, "reasoning_effort": "medium"}
			require.True(t, rewriteGoogleCompatThinkingConfig(payload), modelID)
			require.NotContains(t, payload, "reasoning_effort", modelID)
			require.Equal(t, map[string]any{"include_thoughts": true, "thinking_budget": 8192}, thinkingConfig(payload), modelID)
		}
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

func TestRewriteGoogleCompatThinkingConfig_NonThinkingModelsUntouched(t *testing.T) {
	t.Parallel()

	// Models without known thinking support must keep their previous request
	// shape, whether or not an effort was configured.
	for _, modelID := range []string{
		"gemini-1.5-flash",
		"gemini-2.0-flash",
		"gemini-exp-1206",
		// Specialized 2.5 variants reject thinking_config ("Thinking is not
		// enabled for this model") even though their version matches the
		// thinking-capable 2.5 chat families.
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
		"gemini-2.5-flash-preview-tts",
		"gemini-2.5-flash-native-audio-preview-09-2025",
		"gemini-2.5-computer-use-preview-10-2025",
	} {
		t.Run(modelID, func(t *testing.T) {
			t.Parallel()

			plain := map[string]any{"model": modelID}
			require.False(t, rewriteGoogleCompatThinkingConfig(plain))
			require.NotContains(t, plain, "extra_body")

			withEffort := map[string]any{"model": modelID, "reasoning_effort": "low"}
			require.False(t, rewriteGoogleCompatThinkingConfig(withEffort))
			require.Equal(t, "low", withEffort["reasoning_effort"])
			require.NotContains(t, withEffort, "extra_body")
		})
	}
}
