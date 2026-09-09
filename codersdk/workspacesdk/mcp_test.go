package workspacesdk_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/coder/v2/tailnet/tailnettest"
	"github.com/coder/coder/v2/testutil"
)

func TestReadMCPResource(t *testing.T) {
	t.Parallel()
	ctx := testutil.Context(t, testutil.WaitLong)
	derpMap, _ := tailnettest.RunDERPAndSTUN(t)
	clientID, agentID := uuid.New(), uuid.New()
	client, _ := newTailnetConn(t, derpMap, clientID, "client")
	agent, agentIP := newTailnetConn(t, derpMap, agentID, "agent")
	stitchTailnet(t, map[uuid.UUID]*tailnet.Conn{clientID: client, agentID: agent})
	router := http.NewServeMux()
	router.HandleFunc("POST /api/v0/mcp/read-resource", func(w http.ResponseWriter, r *http.Request) {
		var req workspacesdk.ReadMCPResourceRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		assert.Equal(t, "apps", req.ServerName)
		w.Header().Set("Content-Type", "application/json")
		if req.URI == "ui://missing" {
			w.WriteHeader(http.StatusNotFound)
			assert.NoError(t, json.NewEncoder(w).Encode(codersdk.Response{Message: "not found"}))
			return
		}
		assert.Equal(t, "ui://view?name=a&b", req.URI)
		assert.NoError(t, json.NewEncoder(w).Encode(workspacesdk.ReadMCPResourceResponse{URI: req.URI, MimeType: "text/html;profile=mcp-app", Text: "<html>view</html>", Meta: json.RawMessage(`{"ui":{"prefersBorder":true}}`)}))
	})
	serveTailnetHTTP(t, agent, router)
	require.True(t, client.AwaitReachable(ctx, agentIP))
	conn := workspacesdk.NewAgentConn(client, workspacesdk.AgentConnOptions{AgentID: agentID})
	got, err := conn.ReadMCPResource(ctx, workspacesdk.ReadMCPResourceRequest{ServerName: "apps", URI: "ui://view?name=a&b"})
	require.NoError(t, err)
	require.Equal(t, "ui://view?name=a&b", got.URI)
	require.Equal(t, "text/html;profile=mcp-app", got.MimeType)
	require.Equal(t, "<html>view</html>", got.Text)
	require.JSONEq(t, `{"ui":{"prefersBorder":true}}`, string(got.Meta))
	_, err = conn.ReadMCPResource(ctx, workspacesdk.ReadMCPResourceRequest{ServerName: "apps", URI: "ui://missing"})
	var apiErr *codersdk.Error
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusNotFound, apiErr.StatusCode())
}
