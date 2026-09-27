package chatd

import (
	"encoding/json"
	"testing"

	"charm.land/fantasy"
	"charm.land/fantasy/object"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// maxOutputTokensProbeObject is the structured-output shape for the
// object-call body test. The fake server echoes responseText as the
// Responses output text, so it must be valid JSON for this schema.
type maxOutputTokensProbeObject struct {
	Title string `json:"title"`
}

// maxOutputTokensTestServer starts a fake OpenAI Responses backend that
// captures each request body, and resolves a chatd model against it. The
// returned route carries routeBaseURL as the backend identity the
// max_output_tokens capability decides on, while actual HTTP traffic
// goes to the fake server.
func maxOutputTokensTestServer(
	t *testing.T,
	routeBaseURL string,
	responseText string,
) (chatprovider.Model, resolvedModelCall, func() map[string]any) {
	t.Helper()

	seen := make(chan []byte, 1)
	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		seen <- req.RawBody
		return chattest.OpenAINonStreamingResponse(responseText)
	})
	model, err := chatprovider.ModelFromConfig(
		fantasyopenai.Name,
		"gpt-4o",
		chatprovider.ProviderAPIKeys{
			ByProvider:        map[string]string{fantasyopenai.Name: "test-key"},
			BaseURLByProvider: map[string]string{fantasyopenai.Name: serverURL},
		},
		chatprovider.UserAgent(),
		nil,
		nil,
		&codersdk.ChatModelCallConfig{OpenAIConfig: &codersdk.ChatModelOpenAIConfig{UseResponsesAPI: ptr.Ref(true)}},
	)
	require.NoError(t, err)
	require.True(t, model.Transport().UsesResponses())

	provider := aibridgeTestAIProvider(uuid.New(), "test-openai", database.AIProviderTypeOpenai)
	provider.BaseUrl = routeBaseURL
	resolved := resolvedModelCall{
		model:            model,
		callConfig:       codersdk.ChatModelCallConfig{MaxOutputTokens: ptr.Ref(int64(32000))},
		route:            aibridgeTestRoute(provider),
		resolvedProvider: fantasyopenai.Name,
	}
	readBody := func() map[string]any {
		t.Helper()
		var body map[string]any
		require.NoError(t, json.Unmarshal(<-seen, &body))
		return body
	}
	return model, resolved, readBody
}

func maxOutputTokensPrompt() []fantasy.Message {
	return []fantasy.Message{{
		Role:    fantasy.MessageRoleUser,
		Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
	}}
}

// TestResolvedModelCallMaxOutputTokensRequestBody proves the reported
// ChatGPT subscription failure is fixed at the HTTP layer: a chat call
// routed to the ChatGPT backend omits max_output_tokens, while a call
// routed anywhere else still sends it.
func TestResolvedModelCallMaxOutputTokensRequestBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		routeBaseURL      string
		wantMaxOutputCaps bool
	}{
		{
			name:              "chatgpt omits",
			routeBaseURL:      "https://chatgpt.com/backend-api/codex",
			wantMaxOutputCaps: false,
		},
		{
			name:              "openai keeps",
			routeBaseURL:      "https://api.openai.com/v1",
			wantMaxOutputCaps: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			model, resolved, readBody := maxOutputTokensTestServer(t, tc.routeBaseURL, "ok")

			call := resolved.newCall()
			call.Prompt = maxOutputTokensPrompt()
			_, err := model.LanguageModel().Generate(ctx, call)
			require.NoError(t, err)

			body := readBody()
			if tc.wantMaxOutputCaps {
				require.Equal(t, float64(32000), body["max_output_tokens"])
				return
			}
			require.NotContains(t, body, "max_output_tokens")
		})
	}
}

// TestResolvedModelCallMaxOutputTokensObjectRequestBody covers the same
// capability for structured-output calls, which share the Responses
// params builder and would fail on the ChatGPT backend for the same
// reason.
func TestResolvedModelCallMaxOutputTokensObjectRequestBody(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		routeBaseURL      string
		wantMaxOutputCaps bool
	}{
		{
			name:              "chatgpt omits",
			routeBaseURL:      "https://chatgpt.com/backend-api/codex",
			wantMaxOutputCaps: false,
		},
		{
			name:              "openai keeps",
			routeBaseURL:      "https://api.openai.com/v1",
			wantMaxOutputCaps: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitShort)
			model, resolved, readBody := maxOutputTokensTestServer(t, tc.routeBaseURL, `{"title":"hello"}`)

			call := resolved.newObjectCall("propose_title", "Propose a short chat title.", 256)
			call.Prompt = maxOutputTokensPrompt()
			_, err := object.Generate[maxOutputTokensProbeObject](ctx, model.LanguageModel(), call)
			require.NoError(t, err)

			body := readBody()
			if tc.wantMaxOutputCaps {
				require.Equal(t, float64(256), body["max_output_tokens"])
				return
			}
			require.NotContains(t, body, "max_output_tokens")
		})
	}
}
