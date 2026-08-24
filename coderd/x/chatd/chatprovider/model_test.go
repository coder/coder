package chatprovider_test

import (
	"testing"

	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatopenai"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestModelResolvesTransportFromClient(t *testing.T) {
	t.Parallel()

	forceResponses := true

	// babbage-002 is absent from the provider SDK's known Responses model list.
	client := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "babbage-002"}

	tests := []struct {
		name   string
		config *codersdk.ChatModelOpenAIConfig
		want   chatopenai.Transport
	}{
		{"NoConfig", nil, chatopenai.TransportChatCompletions},
		{"ConfigWithoutOverride", &codersdk.ChatModelOpenAIConfig{}, chatopenai.TransportChatCompletions},
		{"Forced", &codersdk.ChatModelOpenAIConfig{UseResponsesAPI: &forceResponses}, chatopenai.TransportResponses},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			model := chatprovider.NewModel(client, tt.config)
			require.Equal(t, tt.want, model.Transport())
			require.Equal(t, fantasyopenai.Name, model.Provider())
			require.Equal(t, "babbage-002", model.ModelID())
			require.True(t, model.Valid())
		})
	}
}

func TestModelZeroValueFailsClosed(t *testing.T) {
	t.Parallel()

	var model chatprovider.Model
	require.False(t, model.Valid())
	require.Equal(t, chatopenai.TransportInvalid, model.Transport())
	require.Panics(t, func() {
		_ = model.Transport().UsesResponses()
	})
}

// A fantasy provider can return a nil client without an error; the
// constructor must yield the invalid zero value so newLanguageModel reports
// it instead of panicking on the nil dereference.
func TestNewModelNilClientIsInvalid(t *testing.T) {
	t.Parallel()

	model := chatprovider.NewModel(nil, nil)
	require.False(t, model.Valid())
	require.Equal(t, chatopenai.TransportInvalid, model.Transport())
}

func TestModelWithLanguageModelPreservesTransport(t *testing.T) {
	t.Parallel()

	forceResponses := true
	model := chatprovider.NewModel(
		&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "babbage-002"},
		&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: &forceResponses},
	)

	replacement := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "babbage-002"}
	wrapped := model.WithLanguageModel(replacement)

	require.Equal(t, model.Transport(), wrapped.Transport())
	require.Same(t, replacement, wrapped.LanguageModel())
}

// The kept transport is only correct for the identity it was resolved from,
// so replacing the client may not launder an invalid wrapper into a valid one
// or swap in a client with a different identity.
func TestModelWithLanguageModelRejectsIdentityChanges(t *testing.T) {
	t.Parallel()

	client := &chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "babbage-002"}
	model := chatprovider.NewModel(client, nil)

	require.Panics(t, func() {
		chatprovider.Model{}.WithLanguageModel(client)
	})
	require.Panics(t, func() {
		model.WithLanguageModel(nil)
	})
	require.Panics(t, func() {
		model.WithLanguageModel(
			&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4.1"},
		)
	})
}
