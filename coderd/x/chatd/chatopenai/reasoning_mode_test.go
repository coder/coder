package chatopenai_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/codersdk"
)

func TestValidateReasoningMode(t *testing.T) {
	t.Parallel()
	proConfig := func(openAIConfig *codersdk.ChatModelOpenAIConfig) *codersdk.ChatModelCallConfig {
		return &codersdk.ChatModelCallConfig{
			OpenAIConfig:    openAIConfig,
			ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningMode: new("pro")}},
		}
	}
	t.Run("Unset", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, chatopenai.ValidateReasoningMode("openai", nil))
		require.NoError(t, chatopenai.ValidateReasoningMode("azure", &codersdk.ChatModelCallConfig{}))
		require.NoError(t, chatopenai.ValidateReasoningMode("anthropic", &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{}},
		}))
	})
	t.Run("Pro", func(t *testing.T) {
		t.Parallel()
		// Model support is left to OpenAI so unreleased models need no code change.
		require.NoError(t, chatopenai.ValidateReasoningMode("openai", proConfig(nil)))
		require.NoError(t, chatopenai.ValidateReasoningMode("openai", proConfig(&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: new(true), ReasoningModel: new(true)})))
	})
	for _, mode := range []string{"standard", "", "PRO", " pro ", "turbo"} {
		t.Run("Mode="+mode, func(t *testing.T) {
			t.Parallel()
			config := proConfig(nil)
			*config.ProviderOptions.OpenAI.ReasoningMode = mode
			require.ErrorContains(t, chatopenai.ValidateReasoningMode("openai", config), "must be pro")
		})
	}
	for _, provider := range []string{"azure", "anthropic", "openai-compat", "bedrock", ""} {
		t.Run("Provider="+provider, func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, chatopenai.ValidateReasoningMode(provider, proConfig(nil)), "requires an OpenAI provider")
		})
	}
	t.Run("ForcedSamplingModel", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, chatopenai.ValidateReasoningMode("openai", proConfig(&codersdk.ChatModelOpenAIConfig{ReasoningModel: new(false)})), "requires a reasoning model")
	})
	t.Run("ForcedChatCompletions", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, chatopenai.ValidateReasoningMode("openai", proConfig(&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: new(false)})), "requires the Responses API")
	})
}
