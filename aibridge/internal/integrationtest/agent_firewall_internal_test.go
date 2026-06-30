package integrationtest

import (
	"context"
	"fmt"
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

		agentFirewallSessionID := "e5f6a7b8-1234-5678-9abc-def012345678"
		agentFirewallSequenceNumber := int32(42)
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody, http.Header{
			agplaibridge.HeaderAgentFirewallSessionID:      {agentFirewallSessionID},
			agplaibridge.HeaderAgentFirewallSequenceNumber: {fmt.Sprintf("%d", agentFirewallSequenceNumber)},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		// Read the full response body so that AI Gateway can record the interception.
		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		// Verify firewall headers were recorded in the interception.
		interceptions := bridgeServer.Recorder.RecordedInterceptions()
		require.Len(t, interceptions, 1)
		require.NotNil(t, interceptions[0].AgentFirewallSessionID)
		assert.Equal(t, agentFirewallSessionID, *interceptions[0].AgentFirewallSessionID)
		require.NotNil(t, interceptions[0].AgentFirewallSequenceNumber)
		assert.Equal(t, agentFirewallSequenceNumber, *interceptions[0].AgentFirewallSequenceNumber)

		// Verify firewall headers were stripped before reaching upstream.
		received := upstream.ReceivedRequests()
		require.Len(t, received, 1)
		assert.Empty(t, received[0].Header.Get(agplaibridge.HeaderAgentFirewallSessionID))
		assert.Empty(t, received[0].Header.Get(agplaibridge.HeaderAgentFirewallSequenceNumber))

		bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
	})

	t.Run("invalid headers are rejected before reaching upstream", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		fix := fixtures.Parse(t, fixtures.OaiChatSimple)
		// Use a plain upstream that fails the test if called, since the
		// request must be rejected before reaching the provider.
		upstream := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("upstream should not have been called")
		}))
		t.Cleanup(upstream.Close)

		bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withProvider(config.ProviderOpenAI))

		reqBody, err := sjson.SetBytes(fix.Request(), "stream", false)
		require.NoError(t, err)

		// Session ID without a sequence number is malformed; the rest of
		// the validation matrix itself is covered by the unit tests for
		// extractAgentFirewallHeaders.
		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody, http.Header{
			agplaibridge.HeaderAgentFirewallSessionID: {"e5f6a7b8-1234-5678-9abc-def012345678"},
		})
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		_, err = io.ReadAll(resp.Body)
		require.NoError(t, err)

		// The request must fail closed: no interception recorded.
		interceptions := bridgeServer.Recorder.RecordedInterceptions()
		assert.Empty(t, interceptions)
	})
}
