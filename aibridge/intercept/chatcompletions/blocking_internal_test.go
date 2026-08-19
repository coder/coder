package chatcompletions

import (
	"encoding/json"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/coder/coder/v2/aibridge/intercept"
)

// The typed openai.ChatCompletion round trip drops provider-specific fields,
// so Google blocking responses must be serialized from the raw upstream body
// or Gemini's thought metadata never reaches the client.
func TestBlockingMarshalCompletionPreservesGoogleExtraContent(t *testing.T) {
	t.Parallel()

	raw := `{"id":"upstream-id","object":"chat.completion","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"<thought>hidden</thought>answer","extra_content":{"google":{"thought":true,"thought_signature":"sig"}}}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`
	var completion openai.ChatCompletion
	require.NoError(t, json.Unmarshal([]byte(raw), &completion))
	completion.ID = "bridge-id"
	completion.Usage.CompletionTokens = 7

	t.Run("GoogleUpstreamKeepsRawFields", func(t *testing.T) {
		t.Parallel()

		out, err := (&BlockingInterception{interceptionBase: interceptionBase{
			cfg: intercept.Config{BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai/"},
		}}).marshalCompletion(&completion)
		require.NoError(t, err)

		require.True(t, gjson.GetBytes(out, "choices.0.message.extra_content.google.thought").Bool())
		require.Equal(t, "sig", gjson.GetBytes(out, "choices.0.message.extra_content.google.thought_signature").String())
		require.Equal(t, "bridge-id", gjson.GetBytes(out, "id").String())
		require.Equal(t, int64(7), gjson.GetBytes(out, "usage.completion_tokens").Int())
	})

	t.Run("OtherUpstreamsUseTypedMarshal", func(t *testing.T) {
		t.Parallel()

		out, err := (&BlockingInterception{interceptionBase: interceptionBase{
			cfg: intercept.Config{BaseURL: "https://api.openai.com/v1"},
		}}).marshalCompletion(&completion)
		require.NoError(t, err)

		require.False(t, gjson.GetBytes(out, "choices.0.message.extra_content").Exists())
		require.Equal(t, "bridge-id", gjson.GetBytes(out, "id").String())
	})
}
