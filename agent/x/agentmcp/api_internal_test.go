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
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
	"github.com/coder/coder/v2/testutil"
)

func TestHandleListTools_ReloadOnChange(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	// Cases that share the single-request-and-check pattern.
	type singleRequestCase struct {
		name             string
		entries          func(t *testing.T) map[string]mcpServerEntry
		reloadManager    bool
		closeManager     bool
		expectedTools    int
		toolNameContains string
	}

	cases := []singleRequestCase{
		{
			name: "InitialRequestNoReload",
			entries: func(t *testing.T) map[string]mcpServerEntry {
				t.Helper()
				_, entry := fakeMCPServerConfig(t, "srv")
				return map[string]mcpServerEntry{"srv": entry}
			},
			reloadManager:    true,
			expectedTools:    1,
			toolNameContains: "echo",
		},
		{
			name: "ManagerClosedReturnsEmpty",
			entries: func(_ *testing.T) map[string]mcpServerEntry {
				return map[string]mcpServerEntry{}
			},
			closeManager:  true,
			expectedTools: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ctx := testutil.Context(t, testutil.WaitLong)
			logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
			dir := t.TempDir()

			configPath := writeMCPConfig(t, dir, tc.entries(t))

			m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
			if tc.closeManager {
				require.NoError(t, m.Close())
			} else {
				t.Cleanup(func() { _ = m.Close() })
			}

			if tc.reloadManager {
				err := m.Reload(ctx, []string{configPath})
				require.NoError(t, err)
			}

			api := NewAPI(logger, m, func() []string {
				return []string{configPath}
			})

			req := httptest.NewRequest(http.MethodGet, "/tools", nil)
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			var resp workspacesdk.ListMCPToolsResponse
			require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
			require.Len(t, resp.Tools, tc.expectedTools)
			if tc.toolNameContains != "" {
				assert.Contains(t, resp.Tools[0].Name, tc.toolNameContains)
			}
		})
	}

	// ConfigChangeTriggersReload has a mutate-then-re-request flow
	// that does not fit the single-request table pattern.
	t.Run("ConfigChangeTriggersReload", func(t *testing.T) {
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry1 := fakeMCPServerConfig(t, "srv1")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv1": entry1})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
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
}

func TestHandleListTools_RefreshParam(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	t.Run("RefreshTrueUnchangedSnapshot", func(t *testing.T) {
		// Exercises the ?refresh=true code path when the config
		// snapshot is unchanged. Verifies the endpoint returns
		// tools without error.
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry := fakeMCPServerConfig(t, "srv")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		api := NewAPI(logger, m, func() []string {
			return []string{configPath}
		})

		req := httptest.NewRequest(http.MethodGet, "/tools?refresh=true", nil)
		rec := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp workspacesdk.ListMCPToolsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		// Tool should still be present after refresh.
		require.Len(t, resp.Tools, 1)
		assert.Contains(t, resp.Tools[0].Name, "echo")
	})

	t.Run("RefreshTrueWithChangedConfig", func(t *testing.T) {
		// Exercises the ?refresh=true code path when the config
		// has also changed. The reload path already calls
		// RefreshTools, so the handler skips the redundant call.
		// This test covers the branch; it cannot observe the
		// skip without a mock.
		t.Parallel()
		ctx := testutil.Context(t, testutil.WaitLong)
		logger := slogtest.Make(t, nil).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry1 := fakeMCPServerConfig(t, "srv1")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv1": entry1})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		t.Cleanup(func() { _ = m.Close() })

		err := m.Reload(ctx, []string{configPath})
		require.NoError(t, err)

		api := NewAPI(logger, m, func() []string {
			return []string{configPath}
		})

		// Mutate config.
		_, entry2 := fakeMCPServerConfig(t, "srv2")
		writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv2": entry2})

		req := httptest.NewRequest(http.MethodGet, "/tools?refresh=true", nil)
		rec := httptest.NewRecorder()
		api.Routes().ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		var resp workspacesdk.ListMCPToolsResponse
		require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
		require.Len(t, resp.Tools, 1)
		assert.Contains(t, resp.Tools[0].Name, "srv2")
	})
}
