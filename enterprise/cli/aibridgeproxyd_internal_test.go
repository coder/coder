//go:build !slim

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/codersdk"
)

func TestDomainsFromProviders(t *testing.T) {
	t.Parallel()

	t.Run("ExtractsHostnames", func(t *testing.T) {
		t.Parallel()

		providers, err := agplcli.BuildProviders(codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: "openai", Keys: []string{"k"}},
				{Type: aibridge.ProviderAnthropic, Name: "anthropic", Keys: []string{"k"}},
				{Type: aibridge.ProviderOpenAI, Name: "custom", Keys: []string{"k"}, BaseURL: "https://custom-llm.example.com:8443/api"},
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

		providers, err := agplcli.BuildProviders(codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: "first", Keys: []string{"k"}, BaseURL: "https://api.example.com/v1"},
				{Type: aibridge.ProviderOpenAI, Name: "second", Keys: []string{"k"}, BaseURL: "https://api.example.com/v2"},
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

		providers, err := agplcli.BuildProviders(codersdk.AIBridgeConfig{
			Providers: []codersdk.AIProviderConfig{
				{Type: aibridge.ProviderOpenAI, Name: "provider", Keys: []string{"k"}, BaseURL: "https://API.Example.COM/v1"},
			},
		})
		require.NoError(t, err)

		domains, mapping := domainsFromProviders(providers)

		assert.Contains(t, domains, "api.example.com")
		assert.Equal(t, "provider", mapping("API.Example.COM"))
		assert.Equal(t, "provider", mapping("api.example.com"))
	})
}
