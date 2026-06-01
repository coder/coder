package googleopenai_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/internal/googleopenai"
)

func TestShouldPatchGoogleUpstreamRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		baseURL string
		modelID string
		want    bool
	}{
		{
			name:    "gemini api openai endpoint",
			baseURL: "https://generativelanguage.googleapis.com/v1beta/openai/",
			modelID: "gemini-3.5-flash",
			want:    true,
		},
		{
			name:    "non gemini model",
			baseURL: "https://generativelanguage.googleapis.com/v1beta/openai/",
			modelID: "gpt-4o",
		},
		{
			name:    "openai endpoint",
			baseURL: "https://api.openai.com/v1/",
			modelID: "gemini-3.5-flash",
		},
		{
			name:    "vertex endpoint not enabled without fixture",
			baseURL: "https://us-central1-aiplatform.googleapis.com/v1/",
			modelID: "gemini-3.5-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, googleopenai.ShouldPatchGoogleUpstreamRequest(tt.baseURL, tt.modelID))
		})
	}
}

func TestAddThoughtSignaturesToLatestTurn(t *testing.T) {
	t.Parallel()

	payload := decodePayload(t, []byte(`{
		"messages":[
			{"role":"user","content":"previous turn"},
			{
				"role":"assistant",
				"tool_calls":[{"id":"old-call","type":"function","function":{"name":"old","arguments":"{}"}}]
			},
			{"role":"tool","tool_call_id":"old-call","content":"{}"},
			{"role":"user","content":"current turn"},
			{
				"role":"model",
				"tool_calls":[
					{"id":"call-1","type":"function","function":{"name":"list_templates","arguments":"{}"}},
					{"id":"call-2","type":"function","function":{"name":"read_template","arguments":"{}"}}
				]
			}
		]
	}`))

	require.True(t, googleopenai.AddThoughtSignaturesToLatestTurn(payload))
	require.Empty(t, thoughtSignature(t, payload, 1, 0), "previous turns should stay unchanged")
	require.Equal(t, googleopenai.DummyThoughtSignature, thoughtSignature(t, payload, 4, 0))
	require.Equal(t, googleopenai.DummyThoughtSignature, thoughtSignature(t, payload, 4, 1))
}

func TestAddThoughtSignaturesToLatestTurnPreservesRealSignature(t *testing.T) {
	t.Parallel()

	payload := decodePayload(t, []byte(`{
		"messages":[
			{"role":"user","content":"current turn"},
			{
				"role":"assistant",
				"tool_calls":[{
					"id":"call-1",
					"type":"function",
					"function":{"name":"list_templates","arguments":"{}"},
					"extra_content":{"google":{"thought_signature":"real-signature"}}
				}]
			}
		]
	}`))

	require.False(t, googleopenai.AddThoughtSignaturesToLatestTurn(payload))
	require.Equal(t, "real-signature", thoughtSignature(t, payload, 1, 0))
}

func decodePayload(t *testing.T, body []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	require.NoError(t, json.Unmarshal(body, &payload))
	return payload
}

func thoughtSignature(t *testing.T, payload map[string]any, messageIndex int, toolCallIndex int) string {
	t.Helper()

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
