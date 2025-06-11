package aibridged_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/serpent"
	"github.com/stretchr/testify/require"
	"storj.io/drpc"

	"github.com/coder/coder/v2/aibridged"
	"github.com/coder/coder/v2/aibridged/proto"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
	"github.com/coder/coder/v2/testutil"
)

func TestOpenAIStreaming(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subpath := strings.TrimPrefix(r.URL.Path, "/api/v2/aibridge")

		switch subpath {
		case "/v1/chat/completions":
			// Set SSE headers
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")

			w.WriteHeader(http.StatusOK)

			events := [][]byte{
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[{"index":0,"delta":{"role":"assistant","content":null,"tool_calls":[{"index":0,"id":"call_SWduKE0DLAgLShfQqkQzwqq2","type":"function","function":{"name":"get_weather","arguments":""}}],"refusal":null},"logprobs":null,"finish_reason":null}],"usage":null}`),
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\""}}]},"logprobs":null,"finish_reason":null}],"usage":null}`),
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"location"}}]},"logprobs":null,"finish_reason":null}],"usage":null}`),
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\":\""}}]},"logprobs":null,"finish_reason":null}],"usage":null}`),
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"Paris"}}]},"logprobs":null,"finish_reason":null}],"usage":null}`),
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"}"}}]},"logprobs":null,"finish_reason":null}],"usage":null}`),
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[{"index":0,"delta":{},"logprobs":null,"finish_reason":"tool_calls"}],"usage":null}`),
				[]byte(`{"id":"chatcmpl-Bh6ikAZkQqJaGnddPnWSJmmZkZzT6","object":"chat.completion.chunk","created":1749613638,"model":"gpt-4.1-2025-04-14","service_tier":"default","system_fingerprint":"fp_51e1070cf2","choices":[],"usage":{"prompt_tokens":53,"completion_tokens":14,"total_tokens":67,"prompt_tokens_details":{"cached_tokens":0,"audio_tokens":0},"completion_tokens_details":{"reasoning_tokens":0,"audio_tokens":0,"accepted_prediction_tokens":0,"rejected_prediction_tokens":0}}}`),
				[]byte(`[DONE]`),
			}

			// Get the flusher for streaming
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatal("streaming unsupported")
			}

			for _, event := range events {
				_, err := w.Write([]byte(fmt.Sprintf("data: %s\n\n", event)))
				require.NoError(t, err)

				// Flush immediately to send data
				flusher.Flush()

				time.Sleep(testutil.IntervalFast) // Shorter delay for testing
			}
		}
	}))
	defer ts.Close()

	client, _, api := coderdtest.NewWithAPI(t, &coderdtest.Options{
		DeploymentValues: coderdtest.DeploymentValues(t, func(values *codersdk.DeploymentValues) {
			values.AI.Value.BridgeConfig.OpenAIBaseURL = serpent.String(ts.URL)
		}),
	})
	log := testutil.Logger(t)

	fakeDialer := func(ctx context.Context) (proto.DRPCAIBridgeDaemonClient, error) {
		return &fakeBridgeDaemonClient{}, nil
	}
	bridgeSrv, err := aibridged.New(fakeDialer, "127.0.0.1:0", log, api.DeploymentValues.AI.Value.BridgeConfig)
	require.NoError(t, err)
	api.AIBridgeDaemons = []*aibridged.Server{bridgeSrv}

	// TODO: improve approach.
	require.Eventually(t, func() bool {
		return len(api.AIBridgeDaemons) > 0
	}, testutil.WaitMedium, testutil.IntervalFast)

	body := []byte(`
{
    "model": "gpt-4.1",
    "stream": true,
    "messages": [
        {
            "role": "user",
            "content": "What is the weather like in Cape Town today?"
        }
    ]
}`)

	resp, err := client.Request(t.Context(), http.MethodPost, "/api/v2/aibridge/v1/chat/completions", body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode)
}

type fakeBridgeDaemonClient struct{}

func (f *fakeBridgeDaemonClient) DRPCConn() drpc.Conn {
	conn, _ := drpcsdk.MemTransportPipe()
	return conn
}
func (f *fakeBridgeDaemonClient) AuditPrompt(ctx context.Context, in *proto.AuditPromptRequest) (*proto.AuditPromptResponse, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeBridgeDaemonClient) TrackTokenUsage(ctx context.Context, in *proto.TrackTokenUsageRequest) (*proto.TrackTokenUsageResponse, error) {
	return nil, errors.New("not implemented")
}
func (f *fakeBridgeDaemonClient) TrackUserPrompts(ctx context.Context, in *proto.TrackUserPromptsRequest) (*proto.TrackUserPromptsResponse, error) {
	return nil, errors.New("not implemented")
}
