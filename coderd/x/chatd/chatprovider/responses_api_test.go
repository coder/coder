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
				&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: tc.override},
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

// The wire path the client actually uses, the provider option struct type, and
// file-part acceptance must all agree, because a mismatch is silent: the SDK
// type-asserts the concrete option struct, and Responses accepts only images
// and PDFs natively.
func TestModelTransportConsumersAgree(t *testing.T) {
	t.Parallel()

	// Taken from opposite sides of the provider SDK's known-model list.
	const responsesModel = "gpt-4o"
	const nonResponsesModel = "babbage-002"

	forceResponses := true
	forceCompletions := false
	serviceTier := "auto"

	cases := []struct {
		name           string
		modelID        string
		override       *bool
		wantPath       string
		wantOptions    fantasy.ProviderOptionsData
		wantAcceptText bool
	}{
		{
			name:        "ForceResponsesOnUnknownModel",
			modelID:     nonResponsesModel,
			override:    &forceResponses,
			wantPath:    "/responses",
			wantOptions: &fantasyopenai.ResponsesProviderOptions{},
		},
		{
			name:           "ForceCompletionsOnKnownModel",
			modelID:        responsesModel,
			override:       &forceCompletions,
			wantPath:       "/chat/completions",
			wantOptions:    &fantasyopenai.ProviderOptions{},
			wantAcceptText: true,
		},
		{
			name:        "UnsetFollowsKnownModelList",
			modelID:     responsesModel,
			wantPath:    "/responses",
			wantOptions: &fantasyopenai.ResponsesProviderOptions{},
		},
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
				tc.modelID,
				chatprovider.ProviderAPIKeys{
					ByProvider:        map[string]string{fantasyopenai.Name: "test-key"},
					BaseURLByProvider: map[string]string{fantasyopenai.Name: serverURL},
				},
				chatprovider.UserAgent(),
				nil,
				nil,
				&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: tc.override},
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
			require.Equal(t, tc.wantPath, gotPath)
			mu.Unlock()

			options := chatprovider.ProviderOptionsForCall(model, codersdk.ChatModelCallConfig{
				ProviderOptions: &codersdk.ChatModelProviderOptions{
					OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ServiceTier: &serviceTier},
				},
			}, nil)
			require.IsType(t, tc.wantOptions, options[fantasyopenai.Name])

			// Reasoning effort creates the option struct when the config has no
			// OpenAI options of its own.
			effortOptions := chatprovider.ProviderOptionsForCall(model, codersdk.ChatModelCallConfig{
				ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{
					Default: new(codersdk.ChatModelReasoningEffortHigh),
					Max:     new(codersdk.ChatModelReasoningEffortHigh),
				},
			}, nil)
			require.IsType(t, tc.wantOptions, effortOptions[fantasyopenai.Name])

			require.Equal(t, tc.wantAcceptText, model.AcceptsFilePartMediaType("text/plain"))
			require.True(t, model.AcceptsFilePartMediaType("image/png"))
		})
	}
}
