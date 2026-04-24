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

func TestHandleListTools_ReloadOnChange(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	t.Run("InitialRequestNoReload", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		// Boot the manager with the initial config.
		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		api := NewAPI(logger, m, func() []string {
			return []string{configPath}
		})

		// Request should return tools without triggering a reload.
		req := httptest.NewRequest(http.MethodGet, "/tools", nil)
		rec := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp workspacesdk.ListMCPToolsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Len(t, resp.Tools, 1)
		assert.Contains(t, resp.Tools[0].Name, "echo")
	})

	t.Run("ConfigChangeTriggersReload", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry1 := fakeMCPServerConfig(t, "srv1")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv1": entry1})

		m := NewManager(ctx, logger)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		api := NewAPI(logger, m, func() []string {
			return []string{configPath}
		})

		// Verify initial tools.
		req := httptest.NewRequest(http.MethodGet, "/tools", nil)
		rec := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)

		var resp1 workspacesdk.ListMCPToolsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp1))
		require.Len(t, resp1.Tools, 1)
		assert.Contains(t, resp1.Tools[0].Name, "srv1")

		// Mutate the config file.
		_, entry2 := fakeMCPServerConfig(t, "srv2")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv2": entry2})

		// Next request should trigger a reload and return new tools.
		req2 := httptest.NewRequest(http.MethodGet, "/tools", nil)
		rec2 := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec2, req2)
		require.Equal(t, http.StatusOK, rec2.Code)

		var resp2 workspacesdk.ListMCPToolsResponse
		require.NoError(t, json.NewDecoder(rec2.Body).Decode(&resp2))
		require.Len(t, resp2.Tools, 1)
		assert.Contains(t, resp2.Tools[0].Name, "srv2")
	})

	t.Run("ManagerClosedReturnsEmpty", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{})

		m := NewManager(ctx, logger)
		require.NoError(t, m.Close())

		api := NewAPI(logger, m, func() []string {
			return []string{configPath}
		})

		req := httptest.NewRequest(http.MethodGet, "/tools", nil)
		rec := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		// Should still return 200 with empty tools.
		require.Equal(t, http.StatusOK, rec.Code)
		var resp workspacesdk.ListMCPToolsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		assert.Empty(t, resp.Tools)
	})
}
