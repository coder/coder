package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/internal/googleopenai"
)

func TestGoogleOpenAICompatThoughtSignaturePatchSurvivesParamRoundTrip(t *testing.T) {
	t.Parallel()

	const originalSignature = "SIG123"
	raw := []byte(`{
		"model":"gemini-3.5-flash",
		"stream":true,
		"messages":[
			{"role":"user","content":"write a file"},
			{
				"role":"assistant",
				"content":"I'll search for available workspace templates.",
				"tool_calls":[
					{
						"id":"pbk491lp",
						"function":{"arguments":"{}","name":"list_templates"},
						"type":"function",
						"extra_content":{"google":{"thought_signature":"` + originalSignature + `"}}
					}
				]
			},
			{"role":"tool","tool_call_id":"pbk491lp","content":"{}"}
		]
	}`)

	var req ChatCompletionNewParamsWrapper
	require.NoError(t, json.Unmarshal(raw, &req))

	roundTripped, err := json.Marshal(req.ChatCompletionNewParams)
	require.NoError(t, err)
	require.Empty(t, googleThoughtSignatureFromBody(t, roundTripped, 1, 0),
		"openai-go drops extra_content during the typed param round-trip")

	body, err := (&interceptionBase{
		req: &req,
		cfg: intercept.Config{BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/"},
	}).chatCompletionRequestBody()
	require.NoError(t, err)
	require.Equal(t, googleopenai.DummyThoughtSignature, googleThoughtSignatureFromBody(t, body, 1, 0))
}

func TestGoogleOpenAICompatChatCompletionRequestOptions(t *testing.T) {
	t.Parallel()

	var req ChatCompletionNewParamsWrapper
	require.NoError(t, json.Unmarshal([]byte(`{
		"model":"gemini-3.5-flash",
		"messages":[
			{"role":"user","content":"current turn"},
			{
				"role":"assistant",
				"tool_calls":[{"id":"call-1","function":{"arguments":"{}","name":"list_templates"},"type":"function"}]
			}
		]
	}`), &req))

	opts := make([]option.RequestOption, 1)
	updated, overrideBody, err := (&interceptionBase{
		req: &req,
		cfg: intercept.Config{BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/"},
	}).chatCompletionRequestOptions(opts)
	require.NoError(t, err)
	require.True(t, overrideBody)
	require.Len(t, opts, 1)
	require.Len(t, updated, 2)
}

func googleThoughtSignatureFromBody(t *testing.T, body []byte, messageIndex int, toolCallIndex int) string {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	messages, ok := payload["messages"].([]any)
	require.True(t, ok)
	require.Greater(t, len(messages), messageIndex)
	message, ok := messages[messageIndex].(map[string]any)
	require.True(t, ok)
	toolCalls, ok := message["tool_calls"].([]any)
	require.True(t, ok)
	require.Greater(t, len(toolCalls), toolCallIndex)
	toolCall, ok := toolCalls[toolCallIndex].(map[string]any)
	require.True(t, ok)
	extraContent, _ := toolCall["extra_content"].(map[string]any)
	google, _ := extraContent["google"].(map[string]any)
	signature, _ := google["thought_signature"].(string)
	return signature
}
