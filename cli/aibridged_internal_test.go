//go:build !slim

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestBuildProviders(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfig", func(t *testing.T) {
		t.Parallel()
		providers, err := BuildProviders(codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("LegacyOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		assert.Len(t, names, 2)
	})

	t.Run("IndexedOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{
					Type:    aibridge.ProviderAnthropic,
					Name:    "anthropic-zdr",
					Keys:    []string{"sk-zdr"},
					DumpDir: "/tmp/anthropic-dump",
				},
				{
					Type:    aibridge.ProviderOpenAI,
					Name:    "openai-azure",
					Keys:    []string{"sk-azure"},
					BaseURL: "https://azure.openai.com",
					DumpDir: "/tmp/openai-dump",
				},
			},
		}

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Equal(t, []string{"anthropic-zdr", "openai-azure"}, names)
		assert.Equal(t, "/tmp/anthropic-dump", providers[0].APIDumpDir())
		assert.Equal(t, "/tmp/openai-dump", providers[1].APIDumpDir())
	})

	t.Run("LegacyOpenAIConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Keys: []string{"sk-indexed"}},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-legacy")

		_, err := BuildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with indexed provider")
	})

	t.Run("LegacyAnthropicConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: aibridge.ProviderAnthropic, Keys: []string{"sk-indexed"}},
			},
		}
		cfg.LegacyAnthropic.Key = serpent.String("sk-legacy")

		_, err := BuildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with indexed provider")
	})

	t.Run("MixedLegacyAndIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-zdr", Keys: []string{"sk-zdr"}},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		assert.Contains(t, names, "anthropic-zdr")
	})

	t.Run("LegacyAnthropicWithBedrock", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")
		cfg.LegacyBedrock.Region = serpent.String("us-west-2")
		cfg.LegacyBedrock.AccessKey = serpent.String("AKID")
		cfg.LegacyBedrock.AccessKeySecret = serpent.String("secret")

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Equal(t, []string{aibridge.ProviderAnthropic}, names)
	})

	t.Run("LegacyBedrockWithoutAnthropicKey", func(t *testing.T) {
		t.Parallel()
		// Bedrock credentials alone should be enough to create an
		// Anthropic provider — no CODER_AIBRIDGE_ANTHROPIC_KEY needed.
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyBedrock.Region = serpent.String("us-west-2")
		cfg.LegacyBedrock.AccessKey = serpent.String("AKID")
		cfg.LegacyBedrock.AccessKeySecret = serpent.String("secret")

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 1)

		p := providers[0]
		assert.Equal(t, aibridge.ProviderAnthropic, p.Type())
		assert.Equal(t, aibridge.ProviderAnthropic, p.Name())
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: "gemini", Name: "gemini-pro"},
			},
		}

		_, err := BuildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown provider type")
	})

	t.Run("CopilotVariants", func(t *testing.T) {
		t.Parallel()
		// Copilot providers can target any of the three GitHub
		// Copilot API hosts via an explicit BASE_URL.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderCopilot, Name: aibridge.ProviderCopilot, DumpDir: "/tmp/copilot-dump"},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotBusiness, BaseURL: "https://" + agplaibridge.HostCopilotBusiness},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotEnterprise, BaseURL: "https://" + agplaibridge.HostCopilotEnterprise},
			},
		}

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 3)

		assert.Equal(t, aibridge.ProviderCopilot, providers[0].Name())
		assert.Equal(t, "/tmp/copilot-dump", providers[0].APIDumpDir())
		assert.Equal(t, agplaibridge.ProviderCopilotBusiness, providers[1].Name())
		assert.Equal(t, "https://"+agplaibridge.HostCopilotBusiness, providers[1].BaseURL())
		assert.Equal(t, agplaibridge.ProviderCopilotEnterprise, providers[2].Name())
		assert.Equal(t, "https://"+agplaibridge.HostCopilotEnterprise, providers[2].BaseURL())
	})

	t.Run("ChatGPTProvider", func(t *testing.T) {
		t.Parallel()
		// ChatGPT is an OpenAI-compatible provider with a custom
		// base URL. Admins configure it as an indexed openai provider.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: agplaibridge.ProviderChatGPT, BaseURL: agplaibridge.BaseURLChatGPT},
			},
		}

		providers, err := BuildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 1)

		assert.Equal(t, agplaibridge.ProviderChatGPT, providers[0].Name())
		assert.Equal(t, agplaibridge.BaseURLChatGPT, providers[0].BaseURL())
	})
}

func providerNames(providers []aibridge.Provider) []string {
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name()
	}
	return names
}
