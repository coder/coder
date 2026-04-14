//go:build !slim

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/aibridge"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func TestBuildProviders(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfig", func(t *testing.T) {
		t.Parallel()
		providers, err := buildProviders(codersdk.AIBridgeConfig{})
		require.NoError(t, err)
		assert.Empty(t, providers)
	})

	t.Run("LegacyOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := buildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		assert.Len(t, names, 2)
	})

	t.Run("IndexedOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-zdr", Key: "sk-zdr"},
				{Type: aibridge.ProviderOpenAI, Name: "openai-azure", Key: "sk-azure", BaseURL: "https://azure.openai.com"},
			},
		}

		providers, err := buildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assert.Equal(t, []string{"anthropic-zdr", "openai-azure"}, names)
	})

	t.Run("LegacyOpenAIConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Key: "sk-indexed"},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-legacy")

		_, err := buildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with indexed provider")
	})

	t.Run("LegacyAnthropicConflictsWithIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: aibridge.ProviderAnthropic, Key: "sk-indexed"},
			},
		}
		cfg.LegacyAnthropic.Key = serpent.String("sk-legacy")

		_, err := buildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "conflicts with indexed provider")
	})

	t.Run("MixedLegacyAndIndexed", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-zdr", Key: "sk-zdr"},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := buildProviders(cfg)
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

		providers, err := buildProviders(cfg)
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

		providers, err := buildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 1)

		p := providers[0]
		assert.Equal(t, aibridge.ProviderAnthropic, p.Type())
		assert.Equal(t, aibridge.ProviderAnthropic, p.Name())
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: "gemini", Name: "gemini-pro"},
			},
		}

		_, err := buildProviders(cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown provider type")
	})

	t.Run("CopilotVariants", func(t *testing.T) {
		t.Parallel()
		// Copilot providers can target any of the three GitHub
		// Copilot API hosts via an explicit BASE_URL.
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderCopilot, Name: aibridge.ProviderCopilot},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotBusiness, BaseURL: "https://" + agplaibridge.HostCopilotBusiness},
				{Type: aibridge.ProviderCopilot, Name: agplaibridge.ProviderCopilotEnterprise, BaseURL: "https://" + agplaibridge.HostCopilotEnterprise},
			},
		}

		providers, err := buildProviders(cfg)
		require.NoError(t, err)
		require.Len(t, providers, 3)

		assert.Equal(t, aibridge.ProviderCopilot, providers[0].Name())
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
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: agplaibridge.ProviderChatGPT, BaseURL: agplaibridge.BaseURLChatGPT},
			},
		}

		providers, err := buildProviders(cfg)
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

func TestDomainsFromProviders(t *testing.T) {
	t.Parallel()

	t.Run("ExtractsHostnames", func(t *testing.T) {
		t.Parallel()

		providers, err := buildProviders(codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: "openai", Key: "k"},
				{Type: aibridge.ProviderAnthropic, Name: "anthropic", Key: "k"},
				{Type: aibridge.ProviderOpenAI, Name: "custom", Key: "k", BaseURL: "https://custom-llm.example.com:8443/api"},
			},
		})
		require.NoError(t, err)

		domains, mapping := domainsFromProviders(providers)

		assert.Contains(t, domains, "api.openai.com")
		assert.Contains(t, domains, "api.anthropic.com")
		assert.Contains(t, domains, "custom-llm.example.com")

		assert.Equal(t, "openai", mapping("api.openai.com"))
		assert.Equal(t, "anthropic", mapping("api.anthropic.com"))
		assert.Equal(t, "custom", mapping("custom-llm.example.com"))
		assert.Empty(t, mapping("unknown.com"))
	})

	t.Run("DeduplicatesSameHost", func(t *testing.T) {
		t.Parallel()

		providers, err := buildProviders(codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: "first", Key: "k", BaseURL: "https://api.example.com/v1"},
				{Type: aibridge.ProviderOpenAI, Name: "second", Key: "k", BaseURL: "https://api.example.com/v2"},
			},
		})
		require.NoError(t, err)

		domains, mapping := domainsFromProviders(providers)

		// Count occurrences of api.example.com.
		count := 0
		for _, d := range domains {
			if d == "api.example.com" {
				count++
			}
		}
		assert.Equal(t, 1, count)
		// First provider wins.
		assert.Equal(t, "first", mapping("api.example.com"))
	})

	t.Run("CaseInsensitive", func(t *testing.T) {
		t.Parallel()

		providers, err := buildProviders(codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: "provider", Key: "k", BaseURL: "https://API.Example.COM/v1"},
			},
		})
		require.NoError(t, err)

		domains, mapping := domainsFromProviders(providers)

		assert.Contains(t, domains, "api.example.com")
		assert.Equal(t, "provider", mapping("API.Example.COM"))
		assert.Equal(t, "provider", mapping("api.example.com"))
	})
}
