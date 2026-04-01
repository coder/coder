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

	t.Run("LegacyOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{}
		cfg.LegacyOpenAI.Key = serpent.String("sk-openai")
		cfg.LegacyAnthropic.Key = serpent.String("sk-anthropic")

		providers, err := buildProviders(cfg, nil)
		require.NoError(t, err)

		names := providerNames(providers)
		// Legacy + builtins.
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		assert.Contains(t, names, aibridge.ProviderCopilot)
		assert.Contains(t, names, agplaibridge.ProviderCopilotBusiness)
		assert.Contains(t, names, agplaibridge.ProviderCopilotEnterprise)
		assert.Contains(t, names, agplaibridge.ProviderChatGPT)
	})

	t.Run("IndexedOnly", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: aibridge.ProviderAnthropic, Name: "anthropic-zdr", Key: "sk-zdr"},
				{Type: aibridge.ProviderOpenAI, Name: "openai-azure", Key: "sk-azure", BaseURL: "https://azure.openai.com"},
			},
		}

		providers, err := buildProviders(cfg, nil)
		require.NoError(t, err)

		names := providerNames(providers)
		// Indexed + builtins (no legacy since keys are empty).
		assert.Contains(t, names, "anthropic-zdr")
		assert.Contains(t, names, "openai-azure")
		assert.Contains(t, names, aibridge.ProviderCopilot)
		assert.Contains(t, names, agplaibridge.ProviderChatGPT)
		// No default openai/anthropic since legacy keys are empty.
		assert.NotContains(t, names, aibridge.ProviderOpenAI)
		assert.NotContains(t, names, aibridge.ProviderAnthropic)
	})

	t.Run("IndexedOverridesLegacy", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				// Indexed provider uses the same default name as legacy.
				{Type: aibridge.ProviderOpenAI, Name: aibridge.ProviderOpenAI, Key: "sk-indexed"},
			},
		}
		cfg.LegacyOpenAI.Key = serpent.String("sk-legacy")

		providers, err := buildProviders(cfg, nil)
		require.NoError(t, err)

		// Should only have one "openai" provider (the indexed one).
		count := 0
		for _, p := range providers {
			if p.Name() == aibridge.ProviderOpenAI {
				count++
			}
		}
		assert.Equal(t, 1, count, "expected exactly one openai provider")
	})

	t.Run("IndexedOverridesBuiltin", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				// Override the built-in copilot provider.
				{Type: aibridge.ProviderCopilot, Name: aibridge.ProviderCopilot, BaseURL: "https://custom.copilot.com"},
			},
		}

		providers, err := buildProviders(cfg, nil)
		require.NoError(t, err)

		// Should have the indexed copilot, not the default one.
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

		providers, err := buildProviders(cfg, nil)
		require.NoError(t, err)

		names := providerNames(providers)
		// Legacy openai and anthropic should both be present since no name collision.
		assert.Contains(t, names, aibridge.ProviderOpenAI)
		assert.Contains(t, names, aibridge.ProviderAnthropic)
		// Indexed provider also present.
		assert.Contains(t, names, "anthropic-zdr")
	})

	t.Run("UnknownType", func(t *testing.T) {
		t.Parallel()
		cfg := codersdk.AIBridgeConfig{
			Providers: []codersdk.AIBridgeProviderConfig{
				{Type: "gemini", Name: "gemini-pro"},
			},
		}

		_, err := buildProviders(cfg, nil)
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
