package chatcompletions

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
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
		require.Equal(t, int64(7), gjson.GetBytes(out, "usage.completion_tokens").Int())
	})
}

// The blocking path sends typed params unless the body is overridden, so
// preserved cache_control markers must force the raw-body path for
// every upstream, not just Google.
func TestBlockingSendsPreservedCacheControlUpstream(t *testing.T) {
	t.Parallel()

	received := make(chan []byte, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		received <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	t.Cleanup(upstream.Close)

	var req ChatCompletionNewParamsWrapper
	require.NoError(t, json.Unmarshal([]byte(strings.Replace(cacheControlRequest, `"stream":true`, `"stream":false`, 1)), &req))

	httpReq := httptest.NewRequest(http.MethodPost, "/chat/completions", nil)
	interceptor := NewBlockingInterceptor(
		uuid.New(),
		&req,
		intercept.Config{BaseURL: upstream.URL},
		intercept.BYOK{Secret: "test-key", Header: intercept.AuthHeaderAuthorization},
		httpReq.Header,
		otel.Tracer("test"),
	)
	interceptor.Setup(slogtest.Make(t, nil), &testutil.MockRecorder{}, nil)

	w := httptest.NewRecorder()
	require.NoError(t, interceptor.ProcessRequest(w, httpReq))

	body := <-received
	require.Equal(t, "ephemeral", gjson.GetBytes(body, "messages.0.content.0.cache_control.type").String())
	require.Equal(t, "ephemeral", gjson.GetBytes(body, "messages.1.cache_control.type").String())
	require.Equal(t, "ephemeral", gjson.GetBytes(body, "messages.3.content.0.cache_control.type").String())
	require.Equal(t, "anthropic/claude-haiku-4.5", gjson.GetBytes(body, "model").String())
}
