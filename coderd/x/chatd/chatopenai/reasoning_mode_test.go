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
		require.NoError(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeOpenAI, nil))
		require.NoError(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeAzure, &codersdk.ChatModelCallConfig{}))
		require.NoError(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeAnthropic, &codersdk.ChatModelCallConfig{
			ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{}},
		}))
	})
	t.Run("Pro", func(t *testing.T) {
		t.Parallel()
		// Model support is left to OpenAI so unreleased models need no code change.
		require.NoError(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeOpenAI, proConfig(nil)))
		require.NoError(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeOpenAI, proConfig(&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: new(true), ReasoningModel: new(true)})))
	})
	for _, mode := range []string{"standard", "", "PRO", " pro ", "turbo"} {
		t.Run("Mode="+mode, func(t *testing.T) {
			t.Parallel()
			config := proConfig(nil)
			*config.ProviderOptions.OpenAI.ReasoningMode = mode
			require.ErrorContains(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeOpenAI, config), "must be pro")
		})
	}
	for _, providerType := range []codersdk.AIProviderType{codersdk.AIProviderTypeAzure, codersdk.AIProviderTypeAnthropic, codersdk.AIProviderTypeOpenAICompat, codersdk.AIProviderTypeBedrock, ""} {
		t.Run("Provider="+string(providerType), func(t *testing.T) {
			t.Parallel()
			require.ErrorContains(t, chatopenai.ValidateReasoningMode(providerType, proConfig(nil)), "requires an OpenAI provider")
		})
	}
	t.Run("ForcedSamplingModel", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeOpenAI, proConfig(&codersdk.ChatModelOpenAIConfig{ReasoningModel: new(false)})), "requires a reasoning model")
	})
	t.Run("ForcedChatCompletions", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, chatopenai.ValidateReasoningMode(codersdk.AIProviderTypeOpenAI, proConfig(&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: new(false)})), "requires the Responses API")
	})
}
