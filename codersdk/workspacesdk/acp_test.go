package workspacesdk_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestACPTransport(t *testing.T) {
	t.Parallel()
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	clientID, agentID := uuid.New(), uuid.New()
	client, _ := newTailnetConn(t, derpMap, clientID, "client")
	agent, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{clientID: client, agentID: agent})
	router := http.NewServeMux()
	router.HandleFunc("/api/v0/acp/harnesses", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Empty(t, r.Header.Get(workspacesdk.CoderChatIDHeader))
		require.Empty(t, r.Header.Get(workspacesdk.CoderToolCallIDHeader))
		require.NoError(t, json.NewEncoder(w).Encode([]workspacesdk.ACPHarness{{Slug: "fake"}}))
	})
	serveTailnetHTTP(t, agent, router)
	ctx := testutil.Context(t, testutil.WaitLong)
	require.True(t, client.AwaitReachable(ctx, agentIP))
	conn := workspacesdk.NewAgentConn(client, workspacesdk.AgentConnOptions{AgentID: agentID, Logger: testutil.Logger(t)})
	catalog, err := conn.ListACPHarnesses(ctx)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
}
