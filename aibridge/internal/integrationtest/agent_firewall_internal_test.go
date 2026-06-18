package integrationtest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
)

func TestAgentFirewallHeaders(t *testing.T) {
	t.Parallel()

	t.Run("valid headers are recorded and stripped", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		fix := fixtures.Parse(t, fixtures.OaiChatSimple)
		upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))

		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withProvider(config.ProviderOpenAI))

		reqBody, err := sjson.SetBytes(fix.Request(), "stream", false)
		require.NoError(t, err)

		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody, http.Header{
			agplaibridge.HeaderAgentFirewallSessionID:      {"e5f6a7b8-1234-5678-9abc-def012345678"},
			agplaibridge.HeaderAgentFirewallSequenceNumber: {"42"},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Drain body so asyncRecorder.Wait() flushes pending recordings.
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify firewall headers were recorded in the interception.
		interceptions := bridgeServer.Recorder.RecordedInterceptions()
		require.Len(t, interceptions, 1)
		require.NotNil(t, interceptions[0].AgentFirewallSessionID)
		assert.Equal(t, "e5f6a7b8-1234-5678-9abc-def012345678", *interceptions[0].AgentFirewallSessionID)
		require.NotNil(t, interceptions[0].AgentFirewallSequenceNumber)
		assert.Equal(t, int32(42), *interceptions[0].AgentFirewallSequenceNumber)

		// Verify firewall headers were stripped before reaching upstream.
		received := upstream.ReceivedRequests()
		require.Len(t, received, 1)
		assert.Empty(t, received[0].Header.Get(agplaibridge.HeaderAgentFirewallSessionID))
		assert.Empty(t, received[0].Header.Get(agplaibridge.HeaderAgentFirewallSequenceNumber))

		bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
	})

	t.Run("no headers records nil correlation", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		fix := fixtures.Parse(t, fixtures.OaiChatSimple)
		upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))

		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withProvider(config.ProviderOpenAI))

		reqBody, err := sjson.SetBytes(fix.Request(), "stream", false)
		require.NoError(t, err)

		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		interceptions := bridgeServer.Recorder.RecordedInterceptions()
		require.Len(t, interceptions, 1)
		assert.Nil(t, interceptions[0].AgentFirewallSessionID)
		assert.Nil(t, interceptions[0].AgentFirewallSequenceNumber)

		bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
	})

	t.Run("partial headers return 400", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name    string
			headers http.Header
		}{
			{
				name: "only session ID",
				headers: http.Header{
					agplaibridge.HeaderAgentFirewallSessionID: {"e5f6a7b8-1234-5678-9abc-def012345678"},
				},
			},
			{
				name: "only sequence number",
				headers: http.Header{
					agplaibridge.HeaderAgentFirewallSequenceNumber: {"7"},
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, fixtures.OaiChatSimple)
				// Use a plain upstream that fails the test if called,
				// since we expect the request to be rejected before
				// reaching the provider.
				upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("upstream should not have been called")
				}))
				t.Cleanup(upstream.Close)

				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withProvider(config.ProviderOpenAI))

				reqBody, err := sjson.SetBytes(fix.Request(), "stream", false)
				require.NoError(t, err)

				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody, tc.headers)
				require.NoError(t, err)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)

				// No interception should have been recorded.
				interceptions := bridgeServer.Recorder.RecordedInterceptions()
				assert.Empty(t, interceptions)
			})
		}
	})

	t.Run("invalid sequence number returns 400", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name  string
			value string
		}{
			{name: "non-numeric", value: "not-a-number"},
			{name: "int32 overflow", value: "2147483648"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, fixtures.OaiChatSimple)
				upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("upstream should not have been called")
				}))
				t.Cleanup(upstream.Close)

				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withProvider(config.ProviderOpenAI))

				reqBody, err := sjson.SetBytes(fix.Request(), "stream", false)
				require.NoError(t, err)

				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody, http.Header{
					agplaibridge.HeaderAgentFirewallSessionID:      {"e5f6a7b8-1234-5678-9abc-def012345678"},
					agplaibridge.HeaderAgentFirewallSequenceNumber: {tc.value},
				})
				require.NoError(t, err)
				defer resp.Body.Close()
				assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)

				interceptions := bridgeServer.Recorder.RecordedInterceptions()
				assert.Empty(t, interceptions)
			})
		}
	})
}
