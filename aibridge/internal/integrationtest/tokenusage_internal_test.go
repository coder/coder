package integrationtest

import (
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

// TestResponsesStreamingRecordsTokenUsageWithoutMCP asserts that a streaming
// Responses interception records token usage regardless of whether an MCP
// server proxier is configured. Upstream reports usage independently of tool
// injection, and coderd/aibridged tolerates a nil proxier when construction
// fails, so usage must not depend on it.
func TestResponsesStreamingRecordsTokenUsageWithoutMCP(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		opts []bridgeOption
	}{
		{name: "without_mcp_proxy", opts: []bridgeOption{withoutMCP()}},
		{name: "with_noop_mcp_proxy", opts: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, fixtures.OaiResponsesStreamingSimple)
			upstream := testutil.NewMockUpstream(ctx, t, testutil.NewFixtureResponse(fix))
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, tc.opts...)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, fix.Request())
			require.NoError(t, err)
			defer resp.Body.Close()
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)

			usages := bridgeServer.Recorder.RecordedTokenUsages()
			require.Len(t, usages, 1, "exactly one token usage record per completed response")
			require.Positive(t, bridgeServer.Recorder.TotalInputTokens())
			require.Positive(t, bridgeServer.Recorder.TotalOutputTokens())
		})
	}
}

// TestResponsesStreamingRecordsTokenUsagePerAgenticIteration asserts that
// decoupling token recording from the MCP proxier does not double-count usage
// when the inner agentic loop iterates. The injected-tool fixture drives two
// upstream calls, so two records are expected.
func TestResponsesStreamingRecordsTokenUsagePerAgenticIteration(t *testing.T) {
	t.Parallel()

	bridgeServer, _, resp := setupInjectedToolTest(
		t,
		fixtures.OaiResponsesStreamingSingleInjectedTool,
		true,
		defaultTracer,
		pathOpenAIResponses,
		nil,
	)
	defer resp.Body.Close()

	usages := bridgeServer.Recorder.RecordedTokenUsages()
	require.Len(t, usages, 2, "one token usage record per agentic iteration")
}
