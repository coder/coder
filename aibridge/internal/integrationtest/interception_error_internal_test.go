package integrationtest

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
// 401 marks the key temporary and exhausts the pool. Both blocking and
// streaming interceptors preserve the *keypool.Error so the cause is
// categorized as "unauthorized", the auth-failure exhaustion outcome.
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
		{"anthropic_streaming", config.ProviderAnthropic, fixtures.AntSimple, pathAnthropicMessages, true, recorder.ErrorTypeUnauthorized},
		{"openai_blocking", config.ProviderOpenAI, fixtures.OaiChatSimple, pathOpenAIChatCompletions, false, recorder.ErrorTypeUnauthorized},
		{"openai_streaming", config.ProviderOpenAI, fixtures.OaiChatSimple, pathOpenAIChatCompletions, true, recorder.ErrorTypeUnauthorized},
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

// TestInterceptionClientAbortRecorded verifies that an interception the client
// abandons is recorded as aborted, not as an upstream failure or a success,
// and that what was recorded before the abort is kept.
func TestInterceptionClientAbortRecorded(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		provider  string
		fixture   []byte
		path      string
		streaming bool
		// wantUsage is whether usage arrives before the end of the stream, as
		// Anthropic reports input tokens on message_start.
		wantUsage bool
	}{
		{"anthropic_blocking", config.ProviderAnthropic, fixtures.AntSimple, pathAnthropicMessages, false, false},
		{"anthropic_streaming", config.ProviderAnthropic, fixtures.AntSimple, pathAnthropicMessages, true, true},
		{"openai_chat_blocking", config.ProviderOpenAI, fixtures.OaiChatSimple, pathOpenAIChatCompletions, false, false},
		{"openai_chat_streaming", config.ProviderOpenAI, fixtures.OaiChatSimple, pathOpenAIChatCompletions, true, false},
		{"openai_responses_streaming", config.ProviderOpenAI, fixtures.OaiResponsesStreamingSimple, pathOpenAIResponses, true, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fix := fixtures.Parse(t, tc.fixture)
			// Upstream relays the first half of the stream, or nothing for a
			// blocking request, then stalls until the gateway gives up on it.
			upstreamReached := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Drain the body so the server notices when the gateway drops
				// the connection and cancels r.Context().
				_, _ = io.Copy(io.Discard, r.Body)
				if tc.streaming {
					stream := fix.Streaming()
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = w.Write(stream[:len(stream)/2])
					w.(http.Flusher).Flush()
				}
				close(upstreamReached)
				<-r.Context().Done()
			}))
			t.Cleanup(upstream.Close)

			bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL, withProvider(tc.provider))

			reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)
			reqCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			respCh := make(chan *http.Response, 1)
			go func() {
				req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, bridgeServer.URL+tc.path, bytes.NewReader(reqBody))
				if !assert.NoError(t, err) {
					return
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := http.DefaultClient.Do(req)
				if err != nil {
					return // Expected once the request is aborted.
				}
				if !tc.streaming {
					_ = resp.Body.Close()
					return
				}
				respCh <- resp
			}()

			select {
			case <-upstreamReached:
			case <-time.After(testutil.WaitMedium):
				t.Fatal("upstream was not reached")
			}
			if tc.streaming {
				// Wait for relayed bytes so the abort lands mid-stream.
				var resp *http.Response
				select {
				case resp = <-respCh:
				case <-time.After(testutil.WaitMedium):
					t.Fatal("no streaming response")
				}
				defer resp.Body.Close()
				_, err := bufio.NewReader(resp.Body).ReadString('\n')
				require.NoError(t, err)
			}
			cancel()

			var ended *recorder.InterceptionRecordEnded
			require.Eventually(t, func() bool {
				intcs := bridgeServer.Recorder.RecordedInterceptions()
				if len(intcs) != 1 {
					return false
				}
				ended = bridgeServer.Recorder.RecordedInterceptionEnd(intcs[0].ID)
				return ended != nil
			}, testutil.WaitMedium, testutil.IntervalFast)
			require.Equal(t, recorder.ErrorTypeAborted, ended.ErrorType)
			require.NotEmpty(t, ended.ErrorMessage)

			if tc.streaming {
				require.Len(t, bridgeServer.Recorder.RecordedPromptUsages(), 1, "prompt of an aborted stream should be kept")
			}
			if tc.wantUsage {
				require.Positive(t, bridgeServer.Recorder.TotalInputTokens(), "usage recorded before the abort should be kept")
			}
		})
	}
}
