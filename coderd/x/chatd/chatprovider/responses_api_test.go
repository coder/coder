package chatprovider_test

import (
	"context"
	"encoding/json"
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
		{"DefaultGPT6Astra", "gpt-6-astra", nil, "/responses"},
		{"ForceCompletionsOnGPT6Astra", "gpt-6-astra", &forceCompletions, "/chat/completions"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var mu sync.Mutex
			var gotPath string
			serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				mu.Lock()
				gotPath = req.URL.Path
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
				gotPath = req.URL.Path
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

func TestModelFromConfig_OpenAIReasoningModelOverride(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		model         string
		override      *bool
		wantReasoning bool
	}{
		{"ForceReasoningOnUnknownModel", "brand-new-model", new(true), true},
		{"ForceSamplingOnReasoningModel", "gpt-5", new(false), false},
		{"UnsetReasoningModel", "gpt-5", nil, true},
		{"UnsetUnknownModel", "brand-new-model", nil, false},
		{"UnsetGPT6Astra", "gpt-6-astra", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			bodies := make(chan []byte, 1)
			serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				bodies <- req.RawBody
				return chattest.OpenAINonStreamingResponse("ok")
			})
			model, err := chatprovider.ModelFromConfig(
				fantasyopenai.Name, tc.model,
				chatprovider.ProviderAPIKeys{
					ByProvider:        map[string]string{fantasyopenai.Name: "test-key"},
					BaseURLByProvider: map[string]string{fantasyopenai.Name: serverURL},
				},
				chatprovider.UserAgent(), nil, nil,
				&codersdk.ChatModelOpenAIConfig{UseResponsesAPI: new(true), ReasoningModel: tc.override},
			)
			require.NoError(t, err)
			// Client construction must snapshot the override, not retain its pointer.
			if tc.override != nil {
				*tc.override = !*tc.override
			}
			_, err = model.LanguageModel().Generate(t.Context(), fantasy.Call{
				Prompt:      []fantasy.Message{{Role: fantasy.MessageRoleUser, Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}}}},
				Temperature: new(0.7), TopP: new(0.9),
				ProviderOptions: chatprovider.ProviderOptionsForCall(model, codersdk.ChatModelCallConfig{
					ReasoningEffort: &codersdk.ChatModelReasoningEffortConfig{Default: new(codersdk.ChatModelReasoningEffortHigh), Max: new(codersdk.ChatModelReasoningEffortHigh)},
					ProviderOptions: &codersdk.ChatModelProviderOptions{OpenAI: &codersdk.ChatModelOpenAIProviderOptions{ReasoningSummary: new("auto")}},
				}, nil),
			})
			require.NoError(t, err)
			var body map[string]any
			require.NoError(t, json.Unmarshal(<-bodies, &body))
			if tc.wantReasoning {
				require.Equal(t, map[string]any{"effort": "high", "summary": "auto"}, body["reasoning"])
				require.NotContains(t, body, "temperature")
				require.NotContains(t, body, "top_p")
			} else {
				require.NotContains(t, body, "reasoning")
				require.Equal(t, 0.7, body["temperature"])
				require.Equal(t, 0.9, body["top_p"])
			}
		})
	}
}
