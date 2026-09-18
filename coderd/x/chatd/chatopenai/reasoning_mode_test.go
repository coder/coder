package chatopenai_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

func TestValidateReasoningMode(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"gpt-5.6", "gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-6-astra"} {
		for _, suffix := range []string{"", "-2026-08-12"} {
			t.Run(model+suffix, func(t *testing.T) {
				t.Parallel()
				for _, mode := range []string{"standard", "pro"} {
					config := &codersdk.ChatModelCallConfig{ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningMode: new(mode)}}}
					require.NoError(t, chatopenai.ValidateReasoningMode("openai", model+suffix, config))
				}
			})
		}
	}
	t.Run("gpt-daybreak-blue-latest", func(t *testing.T) {
		t.Parallel()
		// The SDK does not know the alias as a Responses model, so the
		// transport must be forced for the mode to be accepted.
		config := &codersdk.ChatModelCallConfig{ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningMode: new("pro")}}}
		require.ErrorContains(t, chatopenai.ValidateReasoningMode("openai", "gpt-daybreak-blue-latest", config), "requires the Responses API")
		config.OpenAIConfig = &codersdk.ChatModelOpenAIConfig{UseResponsesAPI: new(true)}
		require.NoError(t, chatopenai.ValidateReasoningMode("openai", "gpt-daybreak-blue-latest", config))
	})
	t.Run("ForcedSamplingModel", func(t *testing.T) {
		t.Parallel()
		config := &codersdk.ChatModelCallConfig{
			OpenAIConfig:    &codersdk.ChatModelOpenAIConfig{ReasoningModel: new(false)},
			ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningMode: new("pro")}},
		}
		require.ErrorContains(t, chatopenai.ValidateReasoningMode("openai", "gpt-5.6", config), "requires a reasoning model")
		*config.OpenAIConfig.ReasoningModel = true
		require.NoError(t, chatopenai.ValidateReasoningMode("openai", "gpt-5.6", config))
	})
	for _, model := range []string{"gpt-5.5", "gpt-5.6-pro", "gpt-5.6-sol-pro", "gpt-5.6-preview", "gpt-5.6-sol-preview", "gpt-6", "gpt-6-astra-next", "gpt-7", "openai/gpt-5.6", "GPT-5.6", "gpt-5.6-2026-8-12", "gpt-daybreak-red-latest", "gpt-daybreak-blue-latest-2026-08-12"} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			config := &codersdk.ChatModelCallConfig{ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningMode: new("pro")}}}
			require.Error(t, chatopenai.ValidateReasoningMode("openai", model, config))
			config.ProviderOptions.OpenAI.ReasoningMode = nil
			require.NoError(t, chatopenai.ValidateReasoningMode("openai", model, config))
			require.NoError(t, chatopenai.ValidateReasoningMode("azure", model, nil))
		})
	}
}
