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

// builtinProviderNames are the providers that buildProviders always registers
// unless explicitly overridden.
var builtinProviderNames = []string{
	aibridge.ProviderCopilot,
	agplaibridge.ProviderCopilotBusiness,
	agplaibridge.ProviderCopilotEnterprise,
	agplaibridge.ProviderChatGPT,
}

func assertHasBuiltins(t *testing.T, names []string) {
	t.Helper()
	for _, b := range builtinProviderNames {
		assert.Contains(t, names, b)
	}
}

func TestBuildProviders(t *testing.T) {
	t.Parallel()

	t.Run("EmptyConfig", func(t *testing.T) {
		t.Parallel()
		providers, err := buildProviders(codersdk.AIBridgeConfig{})
		require.NoError(t, err)

		names := providerNames(providers)
		assertHasBuiltins(t, names)
		assert.Len(t, names, len(builtinProviderNames))
	})

	t.Run("LegacyOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := buildProviders(cfg)
		require.NoError(t, err)

		names := providerNames(providers)
		assertHasBuiltins(t, names)
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
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
		assertHasBuiltins(t, names)
		assert.Contains(t, names, "anthropic-zdr")
		assert.Contains(t, names, "openai-azure")
		assert.NotContains(t, names, aibridge.ProviderOpenAI)
		assert.NotContains(t, names, aibridge.ProviderAnthropic)
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

	t.Run("IndexedOverridesBuiltin", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderCopilot, Name: aibridge.ProviderCopilot, BaseURL: "https://custom.copilot.com"},
			},
		}

		providers, err := buildProviders(cfg)
		require.NoError(t, err)

		for _, p := range providers {
			if p.Name() == aibridge.ProviderCopilot {
				assert.Equal(t, "https://custom.copilot.com", p.BaseURL())
				break
			}
		}
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
		assert.Contains(t, names, aibridge.ProviderAnthropic)
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
