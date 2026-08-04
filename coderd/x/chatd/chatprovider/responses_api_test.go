package chatprovider_test

import (
	"context"
	"sync"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
)

func TestModelFromConfig_OpenAIResponsesAPIOverride(t *testing.T) {
	t.Parallel()

	// Taken from opposite sides of the provider SDK's known-model list.
	const responsesModel = "gpt-4o"
	const nonResponsesModel = "babbage-002"

	forceResponses := true
	forceCompletions := false

	cases := []struct {
		name     string
		model    string
		override *bool
		wantPath string
	}{
		{"DefaultKnownModel", responsesModel, nil, "/responses"},
		{"DefaultUnknownModel", nonResponsesModel, nil, "/chat/completions"},
		{"ForceResponsesOnUnknownModel", nonResponsesModel, &forceResponses, "/responses"},
		{"ForceCompletionsOnKnownModel", responsesModel, &forceCompletions, "/chat/completions"},
		{"ForceResponsesOnKnownModel", responsesModel, &forceResponses, "/responses"},
		{"ForceCompletionsOnUnknownModel", nonResponsesModel, &forceCompletions, "/chat/completions"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var mu sync.Mutex
			var gotPath string
			serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				mu.Lock()
				gotPath = req.Request.URL.Path
				mu.Unlock()
				return chattest.OpenAINonStreamingResponse("ok")
			})

			model, err := chatprovider.ModelFromConfig(
				fantasyopenai.Name,
				tc.model,
				chatprovider.ProviderAPIKeys{
					ByProvider:        map[string]string{fantasyopenai.Name: "test-key"},
					BaseURLByProvider: map[string]string{fantasyopenai.Name: serverURL},
				},
				chatprovider.UserAgent(),
				nil,
				nil,
				tc.override,
			)
			require.NoError(t, err)

			_, err = model.LanguageModel().Generate(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{{
					Role:    fantasy.MessageRoleUser,
					Content: []fantasy.MessagePart{fantasy.TextPart{Text: "Test message"}},
				}},
			})
			require.NoError(t, err)

			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, tc.wantPath, gotPath)
		})
	}
}

func TestOpenAIResponsesAPIOverride(t *testing.T) {
	t.Parallel()

	useResponsesAPI := true

	t.Run("NilConfig", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, chatprovider.OpenAIResponsesAPIOverride(nil))
	})

	t.Run("Unset", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, chatprovider.OpenAIResponsesAPIOverride(&codersdk.ChatModelOpenAIConfig{}))
	})

	t.Run("Set", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.OpenAIResponsesAPIOverride(&codersdk.ChatModelOpenAIConfig{
			UseResponsesAPI: &useResponsesAPI,
		})
		require.NotNil(t, got)
		require.True(t, *got)
	})
}

// When other OpenAI options exist, the override must select the struct type
// the chosen API reads.
func TestProviderOptionsFromChatModelConfig_ResponsesAPIOverride(t *testing.T) {
	t.Parallel()

	const responsesModel = "gpt-4o"
	const nonResponsesModel = "babbage-002"

	forceResponses := true
	forceCompletions := false
	serviceTier := "auto"

	t.Run("ForceResponsesUsesResponsesOptions", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.ProviderOptionsFromChatModelConfig(
			&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: nonResponsesModel},
			&codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ServiceTier: &serviceTier},
			},
			&forceResponses,
		)
		require.IsType(t, &fantasyopenai.ResponsesProviderOptions{}, got[fantasyopenai.Name])
	})

	t.Run("ForceCompletionsUsesCompletionsOptions", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.ProviderOptionsFromChatModelConfig(
			&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: responsesModel},
			&codersdk.ChatModelProviderOptions{
				OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ServiceTier: &serviceTier},
			},
			&forceCompletions,
		)
		require.IsType(t, &fantasyopenai.ProviderOptions{}, got[fantasyopenai.Name])
	})
}

// When the override is the only OpenAI option, ApplyReasoningEffort creates
// the provider options itself and must match the chosen API.
func TestApplyReasoningEffort_ResponsesAPIOverride(t *testing.T) {
	t.Parallel()

	forceResponses := true
	forceCompletions := false

	t.Run("ForceResponsesOnUnknownModel", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.ApplyReasoningEffort(
			&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "babbage-002"},
			nil,
			new(codersdk.ChatModelReasoningEffortHigh),
			&forceResponses,
		)
		require.IsType(t, &fantasyopenai.ResponsesProviderOptions{}, got[fantasyopenai.Name])
	})

	t.Run("ForceCompletionsOnKnownModel", func(t *testing.T) {
		t.Parallel()
		got := chatprovider.ApplyReasoningEffort(
			&chattest.FakeModel{ProviderName: fantasyopenai.Name, ModelName: "gpt-4o"},
			nil,
			new(codersdk.ChatModelReasoningEffortHigh),
			&forceCompletions,
		)
		require.IsType(t, &fantasyopenai.ProviderOptions{}, got[fantasyopenai.Name])
	})
}
