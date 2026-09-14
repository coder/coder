package chatopenai_test

import (
	"testing"

	fantasyazure "charm.land/fantasy/providers/azure"
	fantasyopenai "charm.land/fantasy/providers/openai"
	fantasyopenaicompat "charm.land/fantasy/providers/openaicompat"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
)

func TestTransportFor(t *testing.T) {
	t.Parallel()

	// Taken from opposite sides of the provider SDK's known-model list.
	const responsesModel = "gpt-4o"
	const nonResponsesModel = "babbage-002"

	forceResponses := true
	forceCompletions := false

	tests := []struct {
		name     string
		provider string
		modelID  string
		override *bool
		want     chatopenai.Transport
	}{
		{"OpenAIKnownModel", fantasyopenai.Name, responsesModel, nil, chatopenai.TransportResponses},
		{"OpenAIUnknownModel", fantasyopenai.Name, nonResponsesModel, nil, chatopenai.TransportChatCompletions},
		{"OpenAIForceResponses", fantasyopenai.Name, nonResponsesModel, &forceResponses, chatopenai.TransportResponses},
		{"OpenAIForceCompletions", fantasyopenai.Name, responsesModel, &forceCompletions, chatopenai.TransportChatCompletions},
		// GPT-6 Astra postdates the SDK's known-model list.
		{"OpenAIGPT6AstraDefaultsToResponses", fantasyopenai.Name, "gpt-6-astra", nil, chatopenai.TransportResponses},
		{"OpenAIGPT6AstraSnapshotDefaultsToResponses", fantasyopenai.Name, "gpt-6-astra-2026-09-03", nil, chatopenai.TransportResponses},
		{"OpenAIGPT6AstraForceCompletions", fantasyopenai.Name, "gpt-6-astra", &forceCompletions, chatopenai.TransportChatCompletions},
		{"AzureIgnoresOverride", fantasyazure.Name, responsesModel, &forceCompletions, chatopenai.TransportResponses},
		{"AzureKnownModelList", fantasyazure.Name, nonResponsesModel, nil, chatopenai.TransportChatCompletions},
		{"OpenAICompat", fantasyopenaicompat.Name, responsesModel, &forceResponses, chatopenai.TransportChatCompletions},
		{"Anthropic", "anthropic", "claude-sonnet-4-5", &forceResponses, chatopenai.TransportNotApplicable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatopenai.TransportFor(tt.provider, tt.modelID, tt.override))
		})
	}
}

func TestIsGPT6Astra(t *testing.T) {
	t.Parallel()

	require.True(t, chatopenai.IsGPT6Astra("gpt-6-astra"))
	require.True(t, chatopenai.IsGPT6Astra("GPT-6-Astra-2026-09-03"))
	require.False(t, chatopenai.IsGPT6Astra("gpt-5.6-sol"))
	require.False(t, chatopenai.IsGPT6Astra("gpt-6"))
}

func TestTransportUsesResponses(t *testing.T) {
	t.Parallel()

	require.True(t, chatopenai.TransportResponses.UsesResponses())
	require.False(t, chatopenai.TransportChatCompletions.UsesResponses())
	require.False(t, chatopenai.TransportNotApplicable.UsesResponses())

	// The zero value must not silently mean Chat Completions.
	require.Panics(t, func() {
		_ = chatopenai.TransportInvalid.UsesResponses()
	})
}
