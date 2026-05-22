//go:build !slim

package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
)

func TestDomainsFromProviders(t *testing.T) {
	t.Parallel()

	t.Run("ExtractsHostnames", func(t *testing.T) {
		t.Parallel()

		providers := []aibridge.Provider{
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "openai", Key: "k"}),
			aibridge.NewAnthropicProvider(config.Anthropic{Name: "anthropic", Key: "k"}, nil),
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "custom", Key: "k", BaseURL: "https://custom-llm.example.com:8443/api"}),
		}

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

		providers := []aibridge.Provider{
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "first", Key: "k", BaseURL: "https://api.example.com/v1"}),
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "second", Key: "k", BaseURL: "https://api.example.com/v2"}),
		}

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

		providers := []aibridge.Provider{
			aibridge.NewOpenAIProvider(config.OpenAI{Name: "provider", Key: "k", BaseURL: "https://API.Example.COM/v1"}),
		}

		domains, mapping := domainsFromProviders(providers)

		assert.Contains(t, domains, "api.example.com")
		assert.Equal(t, "provider", mapping("API.Example.COM"))
		assert.Equal(t, "provider", mapping("api.example.com"))
	})
}
