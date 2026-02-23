package codersdk_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestChatModelProviderOptions_MarshalJSON_UsesPlainProviderPayload(t *testing.T) {
	t.Parallel()

	sendReasoning := true
	effort := "high"

	raw, err := json.Marshal(codersdk.ChatModelProviderOptions{
		Anthropic: &codersdk.ChatModelAnthropicProviderOptions{
			SendReasoning: &sendReasoning,
			Effort:        &effort,
		},
	})
	require.NoError(t, err)
	require.NotContains(t, string(raw), `"type":"anthropic.options"`)
	require.NotContains(t, string(raw), `"data":`)
	require.Contains(t, string(raw), `"send_reasoning":true`)
	require.Contains(t, string(raw), `"effort":"high"`)
}

func TestChatModelProviderOptions_UnmarshalJSON_ParsesPlainProviderPayloads(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"anthropic": {
			"send_reasoning": true,
			"effort": "high"
		}
	}`)

	var decoded codersdk.ChatModelProviderOptions
	err := json.Unmarshal(raw, &decoded)
	require.NoError(t, err)
	require.NotNil(t, decoded.Anthropic)
	require.NotNil(t, decoded.Anthropic.SendReasoning)
	require.True(t, *decoded.Anthropic.SendReasoning)
	require.NotNil(t, decoded.Anthropic.Effort)
	require.Equal(
		t,
		"high",
		*decoded.Anthropic.Effort,
	)
}
