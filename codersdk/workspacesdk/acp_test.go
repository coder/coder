package workspacesdk_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

func TestACPTransport(t *testing.T) {
	t.Parallel()
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	clientID, agentID := uuid.New(), uuid.New()
	client, _ := newTailnetConn(t, derpMap, clientID, "client")
	agent, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{clientID: client, agentID: agent})
	id := workspacesdk.ACPSessionID{SessionID: "native/id?x=1 &雪", HarnessSlug: "fake", WorkingDirectory: "/explicit dir/雪"}
	cursor := workspacesdk.ACPCursor{Epoch: uuid.New(), Seq: 9}
	info := workspacesdk.ACPSession{ID: id, Status: workspacesdk.ACPSessionStatusIdle, Cursor: cursor}
	router := http.NewServeMux()
	router.HandleFunc("/api/v0/acp/", func(w http.ResponseWriter, r *http.Request) {
		require.Empty(t, r.Header.Get(workspacesdk.CoderChatIDHeader))
		// ACP uses its own UUID deduplication and does not attach context tool-call IDs.
		require.Empty(t, r.Header.Get(workspacesdk.CoderToolCallIDHeader))
		var result any
		if r.URL.Path != "/api/v0/acp/harnesses" && r.URL.Path != "/api/v0/acp/sessions" {
			if r.URL.Query().Get("session_id") == "unknown" {
				w.WriteHeader(http.StatusNotFound)
				_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "ACP session not found"})
				return
			}
			require.Equal(t, id.SessionID, r.URL.Query().Get("session_id"))
			require.Equal(t, id.HarnessSlug, r.URL.Query().Get("harness_slug"))
			require.Equal(t, id.WorkingDirectory, r.URL.Query().Get("working_directory"))
		}
		switch r.URL.Path {
		case "/api/v0/acp/harnesses":
			result = []workspacesdk.ACPHarness{{Slug: "fake"}}
		case "/api/v0/acp/sessions":
			if r.Method == http.MethodPost {
				var req workspacesdk.ACPCreateSessionRequest
				require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
				require.Equal(t, id.WorkingDirectory, req.WorkingDirectory)
				result = info
			} else {
				result = []workspacesdk.ACPSession{info}
			}
		case "/api/v0/acp/sessions/session/interrupt":
			result = info
		case "/api/v0/acp/sessions/session/messages":
			result = workspacesdk.ACPMessageResponse{Outcome: workspacesdk.ACPMessageOutcomeInjected}
		case "/api/v0/acp/sessions/session/stream":
			require.Equal(t, cursor.Epoch.String(), r.URL.Query().Get("epoch"))
			require.Equal(t, "9", r.URL.Query().Get("after"))
			conn, err := websocket.Accept(w, r, nil)
			require.NoError(t, err)
			defer conn.CloseNow()
			require.NoError(t, wsjson.Write(r.Context(), conn, workspacesdk.ACPEvent{Cursor: cursor, Kind: workspacesdk.ACPEventKindSnapshot, Session: &info}))
			_ = conn.Close(websocket.StatusNormalClosure, "")
			return
		case "/api/v0/acp/sessions/session":
			result = workspacesdk.ACPSessionResponse{Session: info}
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(codersdk.Response{Message: "ACP session not found"})
			return
		}
		require.NoError(t, json.NewEncoder(w).Encode(result))
	})
	serveTailnetHTTP(t, agent, router)
	ctx := testutil.Context(t, testutil.WaitLong)
	require.True(t, client.AwaitReachable(ctx, agentIP))
	conn := workspacesdk.NewAgentConn(client, workspacesdk.AgentConnOptions{AgentID: agentID, Logger: testutil.Logger(t)})
	ctx = workspacesdk.WithToolCallID(ctx, uuid.New())
	catalog, err := conn.ListACPHarnesses(ctx)
	require.NoError(t, err)
	require.Len(t, catalog, 1)
	created, err := conn.CreateACPSession(ctx, workspacesdk.ACPCreateSessionRequest{RequestID: uuid.New(), HarnessSlug: id.HarnessSlug, WorkingDirectory: id.WorkingDirectory})
	require.NoError(t, err)
	require.Equal(t, info.ID, created.ID)

	listed, err := conn.ListACPSessions(ctx)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	_, err = conn.ReadACPSession(ctx, id, workspacesdk.ACPReadOptions{})
	require.NoError(t, err)
	message, err := conn.SendACPMessage(ctx, id, workspacesdk.ACPMessageRequest{ID: uuid.New(), Text: "hello"})
	require.NoError(t, err)
	require.Equal(t, workspacesdk.ACPMessageOutcomeInjected, message.Outcome)
	_, err = conn.InterruptACPSession(ctx, id)
	require.NoError(t, err)
	events, closer, err := conn.WatchACPSession(ctx, testutil.Logger(t), id, workspacesdk.ACPReadOptions{After: &cursor})
	require.NoError(t, err)
	defer closer.Close()
	select {
	case event := <-events:
		require.Equal(t, cursor, event.Cursor)
		require.Equal(t, workspacesdk.ACPEventKindSnapshot, event.Kind)
		require.NotNil(t, event.Session)
		require.Equal(t, workspacesdk.ACPSessionStatusIdle, event.Session.Status)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	_, err = conn.ReadACPSession(ctx, workspacesdk.ACPSessionID{HarnessSlug: "unknown", WorkingDirectory: "/explicit", SessionID: "unknown"}, workspacesdk.ACPReadOptions{})
	var sdkErr *codersdk.Error
	require.ErrorAs(t, err, &sdkErr)
	require.Equal(t, http.StatusNotFound, sdkErr.StatusCode())
}
