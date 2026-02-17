package chatd

import (
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
		},
		[]ConfiguredProvider{
			{Provider: "openai", APIKey: "   "},
			{Provider: "anthropic", APIKey: " provider-anthropic "},
		},
	)

	require.Equal(t, "deployment-openai", merged.OpenAI)
	require.Equal(t, "provider-anthropic", merged.Anthropic)
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
