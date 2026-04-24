package agentmcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

// TestEndToEnd_EditFileSeeUpdate exercises the full path from
// config change to tool list update, as described in Stage 4:
//
//  1. Create a Manager, run an initial Reload with a fake server.
//  2. Call the API handler, assert the initial tool list.
//  3. Rewrite .mcp.json with a different server set.
//  4. Call the handler again, assert the new tool list.
//
// This proves stat-on-request detected the diff, invoked reload,
// and returned fresh tools in a single request.
func TestEndToEnd_EditFileSeeUpdate(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
	dir := t.TempDir()

	// Step 1: initial config with server "alpha".
	_, alphaEntry := fakeMCPServerConfig(t, "alpha")
	configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{
		"alpha": alphaEntry,
	})

	m := NewManager(ctx, logger)
	t.Cleanup(func() { _ = m.Close() })

	err := m.Reload(ctx, []string{configPath})
	require.NoError(t, err)

	api := NewAPI(logger, m, func() []string {
		return []string{configPath}
	})

	// Step 2: first request sees "alpha" tools.
	req1 := httptest.NewRequest(http.MethodGet, "/tools", nil)
	rec1 := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec1, req1)
	require.Equal(t, http.StatusOK, rec1.Code)

	var resp1 workspacesdk.ListMCPToolsResponse
	require.NoError(t, json.NewDecoder(rec1.Body).Decode(&resp1))
	require.Len(t, resp1.Tools, 1)
	assert.Contains(t, resp1.Tools[0].Name, "alpha")

	// Step 3: rewrite config with server "beta".
	_, betaEntry := fakeMCPServerConfig(t, "beta")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{
		"beta": betaEntry,
	})

	// Step 4: next request triggers reload and sees "beta" tools.
	req2 := httptest.NewRequest(http.MethodGet, "/tools", nil)
	rec2 := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)

	var resp2 workspacesdk.ListMCPToolsResponse
	require.NoError(t, json.NewDecoder(rec2.Body).Decode(&resp2))
	require.Len(t, resp2.Tools, 1)
	assert.Contains(t, resp2.Tools[0].Name, "beta")
	assert.NotContains(t, resp2.Tools[0].Name, "alpha",
		"old server should not appear after config change")
}
