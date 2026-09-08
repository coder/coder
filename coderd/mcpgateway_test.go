package coderd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

// newGatewayUpstream starts a fake upstream MCP server exposing one "echo" tool.
func newGatewayUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	srv := mcp.NewServer(&mcp.Implementation{Name: "upstream", Version: "1.0.0"}, nil)
	srv.AddTool(&mcp.Tool{
		Name:        "echo",
		Description: "Echoes the input",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"input": map[string]any{"type": "string"}},
		},
	}, func(_ context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args map[string]any
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return nil, err
		}
		input, _ := args["input"].(string)
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + input}}}, nil
	})
	ts := httptest.NewServer(mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true},
	))
	t.Cleanup(ts.Close)
	return ts
}

// gatewayPost sends a raw JSON-RPC request to the gateway route as a "cURL"
// client would, and returns the status and the JSON-RPC payload.
func gatewayPost(ctx context.Context, t *testing.T, baseURL, token string, orgID uuid.UUID, body string) (int, string) {
	t.Helper()
	url := baseURL + "/api/experimental/organizations/" + orgID.String() + "/mcp-gateway"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBufferString(body))
	require.NoError(t, err)
	req.Header.Set(codersdk.SessionTokenHeader, token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")

	//nolint:bodyclose // Closed below.
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	return res.StatusCode, extractJSONRPC(string(raw))
}

// extractJSONRPC returns the JSON-RPC payload from either a plain JSON body or
// an SSE stream.
func extractJSONRPC(body string) string {
	if !strings.Contains(body, "data:") {
		return body
	}
	for _, line := range strings.Split(body, "\n") {
		if after, ok := strings.CutPrefix(strings.TrimSpace(line), "data:"); ok {
			return strings.TrimSpace(after)
		}
	}
	return body
}

const initializeBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}`

func TestMCPGateway(t *testing.T) {
	t.Parallel()

	t.Run("CURL", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentMCPGateway)}
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
		first := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitLong)

		upstream := newGatewayUpstream(t)
		cfg := dbgen.MCPServerConfig(t, db, database.MCPServerConfig{
			OrganizationID: first.OrganizationID,
			Slug:           "up",
			Url:            upstream.URL,
			Enabled:        true,
		})

		status, _ := gatewayPost(ctx, t, client.URL.String(), client.SessionToken(), first.OrganizationID, initializeBody)
		require.Equal(t, http.StatusOK, status)

		status, body := gatewayPost(ctx, t, client.URL.String(), client.SessionToken(), first.OrganizationID,
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
		require.Equal(t, http.StatusOK, status)
		require.Contains(t, body, cfg.Slug+"__echo")
	})

	t.Run("ExperimentDisabled", func(t *testing.T) {
		t.Parallel()

		client := coderdtest.New(t, nil)
		first := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		status, _ := gatewayPost(ctx, t, client.URL.String(), client.SessionToken(), first.OrganizationID, initializeBody)
		require.Equal(t, http.StatusForbidden, status)
	})

	t.Run("NotOrgMember", func(t *testing.T) {
		t.Parallel()

		dv := coderdtest.DeploymentValues(t)
		dv.Experiments = []string{string(codersdk.ExperimentMCPGateway)}
		client, db := coderdtest.NewWithDatabase(t, &coderdtest.Options{DeploymentValues: dv})
		first := coderdtest.CreateFirstUser(t, client)
		ctx := testutil.Context(t, testutil.WaitShort)

		other := dbgen.Organization(t, db, database.Organization{})
		member, _ := coderdtest.CreateAnotherUser(t, client, first.OrganizationID)

		status, _ := gatewayPost(ctx, t, client.URL.String(), member.SessionToken(), other.ID, initializeBody)
		require.Contains(t, []int{http.StatusForbidden, http.StatusNotFound}, status)
	})
}
