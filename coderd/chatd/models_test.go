package chatd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestMergeProviderAPIKeys(t *testing.T) {
	t.Parallel()

	merged := MergeProviderAPIKeys(
		ProviderAPIKeys{
			OpenAI:    " deployment-openai ",
			Anthropic: "deployment-anthropic",
			ByProvider: map[string]string{
				"openrouter": " deployment-openrouter ",
			},
		},
		[]ConfiguredProvider{
			{Provider: "openai", APIKey: "   "},
			{Provider: "anthropic", APIKey: " provider-anthropic "},
			{Provider: "openrouter", APIKey: "provider-openrouter"},
		},
	)

	require.Equal(t, "deployment-openai", merged.OpenAI)
	require.Equal(t, "provider-anthropic", merged.Anthropic)
	require.Equal(t, "provider-openrouter", merged.APIKey("openrouter"))
}

func TestModelCatalogListConfiguredModels_UsesFallbackAPIKeys(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(
		testutil.Logger(t),
		nil,
		ProviderAPIKeys{
			OpenAI: "deployment-openai",
		},
		ModelCatalogConfig{},
	)

	response, ok := catalog.ListConfiguredModels(
		[]ConfiguredProvider{
			{Provider: "openai", APIKey: "   "},
		},
		[]ConfiguredModel{
			{
				Provider:    "openai",
				Model:       "gpt-5.2",
				DisplayName: "GPT 5.2",
			},
		},
	)
	require.True(t, ok)
	require.Len(t, response.Providers, 1)

	provider := response.Providers[0]
	require.Equal(t, "openai", provider.Provider)
	require.True(t, provider.Available)
	require.Empty(t, provider.UnavailableReason)
	require.Equal(
		t,
		[]codersdk.ChatModel{{
			ID:          "openai:gpt-5.2",
			Provider:    "openai",
			Model:       "gpt-5.2",
			DisplayName: "GPT 5.2",
		}},
		provider.Models,
	)
}

func TestModelCatalogListConfiguredModels_NoEnabledModels(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(
		testutil.Logger(t),
		nil,
		ProviderAPIKeys{
			OpenAI: "deployment-openai",
		},
		ModelCatalogConfig{},
	)

	response, ok := catalog.ListConfiguredModels(
		[]ConfiguredProvider{
			{Provider: "openai", APIKey: ""},
		},
		nil,
	)
	require.False(t, ok)
	require.Equal(t, codersdk.ChatModelsResponse{}, response)
}

func TestNormalizeProviderSupportsFantasyProviders(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{
		"anthropic",
		"azure",
		"bedrock",
		"google",
		"openai",
		"openai-compat",
		"openrouter",
		"vercel",
	}, SupportedProviders())

	for _, provider := range SupportedProviders() {
		require.Equal(t, provider, NormalizeProvider(provider))
		require.Equal(t, provider, NormalizeProvider(strings.ToUpper(provider)))
	}
}

func TestModelCatalogListConfiguredProviderAvailability_AllSupported(t *testing.T) {
	t.Parallel()

	catalog := NewModelCatalog(
		testutil.Logger(t),
		nil,
		ProviderAPIKeys{
			OpenAI: "deployment-openai",
		},
		ModelCatalogConfig{},
	)

	response := catalog.ListConfiguredProviderAvailability(
		[]ConfiguredProvider{
			{Provider: "openrouter", APIKey: "openrouter-key"},
		},
	)
	require.Len(t, response.Providers, len(SupportedProviders()))

	availability := make(map[string]codersdk.ChatModelProvider, len(response.Providers))
	for _, provider := range response.Providers {
		availability[provider.Provider] = provider
	}

	require.True(t, availability["openai"].Available)
	require.True(t, availability["openrouter"].Available)
	require.False(t, availability["anthropic"].Available)
}

func TestModelFromConfig_OpenRouter(t *testing.T) {
	t.Parallel()

	model, err := modelFromConfig(
		chatModelConfig{
			Provider: "openrouter",
			Model:    "gpt-4o-mini",
		},
		ProviderAPIKeys{
			ByProvider: map[string]string{
				"openrouter": "openrouter-key",
			},
		},
	)
	require.NoError(t, err)
	require.Equal(t, "openrouter", model.Provider())
	require.Equal(t, "gpt-4o-mini", model.Model())
}

func TestModelFromConfig_AzureRequiresBaseURL(t *testing.T) {
	t.Parallel()

	_, err := modelFromConfig(
		chatModelConfig{
			Provider: "azure",
			Model:    "gpt-4o-mini",
		},
		ProviderAPIKeys{
			ByProvider: map[string]string{
				"azure": "azure-key",
			},
		},
	)
	require.EqualError(
		t,
		err,
		"azure provider requires a base URL, but chat provider configs do not support base URLs yet",
	)
}
