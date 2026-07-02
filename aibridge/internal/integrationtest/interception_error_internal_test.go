package integrationtest

import (
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/recorder"
)

// TestInterceptionUpstreamErrorRecorded verifies that a failed interception
// records a categorized upstream error on the ended record.
//
// The default test provider is centralized (backed by a single-key pool), so a
// 401 exhausts the pool. The pool masks permanent exhaustion as a 502 to the
// client. Blocking interceptors return the *keypool.Error, so the cause is
// recovered as "unauthorized". Streaming interceptors only observe the masked
// 502 (the pool response is re-parsed by the SDK), so the record reflects
// "server_error".
func TestInterceptionUpstreamErrorRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		provider  string
		fixture   []byte
		path      string
		streaming bool
		wantType  recorder.ErrorType
	}{
		{"anthropic_blocking", config.ProviderAnthropic, fixtures.AntSimple, pathAnthropicMessages, false, recorder.ErrorTypeUnauthorized},
		{"anthropic_streaming", config.ProviderAnthropic, fixtures.AntSimple, pathAnthropicMessages, true, recorder.ErrorTypeServerError},
		{"openai_blocking", config.ProviderOpenAI, fixtures.OaiChatSimple, pathOpenAIChatCompletions, false, recorder.ErrorTypeUnauthorized},
		{"openai_streaming", config.ProviderOpenAI, fixtures.OaiChatSimple, pathOpenAIChatCompletions, true, recorder.ErrorTypeServerError},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fix := fixtures.Parse(t, tc.fixture)
			upstream := testutil.NewMockUpstream(t.Context(), t,
				testutil.NewErrorResponse(http.StatusUnauthorized, ""),
			)
			upstream.AllowOverflow = true

			bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL, withProvider(tc.provider))

			reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, reqBody)
			require.NoError(t, err)
			_, _ = io.Copy(io.Discard, resp.Body)
			require.NoError(t, resp.Body.Close())

			intcs := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, intcs, 1)
			ended := bridgeServer.Recorder.RecordedInterceptionEnd(intcs[0].ID)
			require.NotNil(t, ended, "interception should be ended")
			require.Equal(t, tc.wantType, ended.ErrorType)
			require.NotEmpty(t, ended.ErrorMessage)
		})
	}
}
