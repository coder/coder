package agent_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agenttest"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/coder/v2/testutil"
)

// TestAgent_APIClientSessionID verifies that a client_session_id set as baggage
// on the agent connection reaches the agent's request logs. workspacesdk builds
// a separate HTTP client per agent API request, so the CLI's baggage transport
// wrapper does not apply; the ID rides along via the connection's extra headers
// instead. The agent's tracing middleware reads client_session_id from the
// baggage header and adds it to the request log context.
func TestAgent_APIClientSessionID(t *testing.T) {
	t.Parallel()

	const sessionID = "0123456789abcdef0123456789abcdef"
	ctx := testutil.Context(t, testutil.WaitLong)

	sink := testutil.NewFakeSink(t)

	//nolint:dogsled
	conn, _, _, _, _ := setupAgent(t, agentsdk.Manifest{}, 0, func(_ *agenttest.Client, o *agent.Options) {
		o.Logger = sink.Logger().Named("agent")
	})
	require.True(t, conn.AwaitReachable(ctx))

	conn.SetExtraHeaders(http.Header{
		"baggage": []string{tracing.SessionIDBaggageKey + "=" + sessionID},
	})

	// Any agent HTTP API call exercises the per-request client Asher flagged;
	// ListeningPorts hits /api/v0/listening-ports, which the agent middleware
	// tracks.
	_, err := conn.ListeningPorts(ctx)
	require.NoError(t, err)

	// The middleware should have added client_session_id to the request log
	// context, so at least one agent log entry carries it. The request log is
	// written after the handler returns, so the client can see the response
	// before the entry exists.
	testutil.Eventually(ctx, t, func(context.Context) bool {
		return len(sink.Entries(func(e slog.SinkEntry) bool {
			for _, f := range e.Fields {
				if f.Name == "client_session_id" {
					return f.Value == sessionID
				}
			}
			return false
		})) > 0
	}, testutil.IntervalFast, "client_session_id should reach the agent request logs")
}
