package agentmcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
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
			logger := testutil.Logger(t).Leveled(slog.LevelDebug)
			dir := t.TempDir()

			configPath := writeMCPConfig(t, dir, tc.entries(t))

			m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
			if tc.closeManager {
				require.NoError(t, m.Close())
			} else {
				m.MarkStartupSettled()
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
		logger := testutil.Logger(t).Leveled(slog.LevelDebug)
		dir := t.TempDir()

		_, entry1 := fakeMCPServerConfig(t, "srv1")
		configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv1": entry1})

		m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
		m.MarkStartupSettled()
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

// TestHandleListTools_ReloadsAfterStartupSettled exercises the
// cold-start path end-to-end against a real *Manager. Startup has
// settled, so the handler may drive the first safe reload.
func TestHandleListTools_ReloadsAfterStartupSettled(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t).Leveled(slog.LevelDebug)
	dir := t.TempDir()

	_, entry := fakeMCPServerConfig(t, "srv")
	configPath := writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	m.MarkStartupSettled()
	t.Cleanup(func() { _ = m.Close() })

	// No prior m.Reload: snapshot empty and tools unset.
	require.Empty(t, m.cachedTools(), "manager should start with no tools")

	api := NewAPI(logger, m, func() []string {
		return []string{configPath}
	})

	req := httptest.NewRequest(http.MethodGet, "/tools", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	api.Routes().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp workspacesdk.ListMCPToolsResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Tools, 1)
	assert.Contains(t, resp.Tools[0].Name, "echo")
}

func TestHandleListTools_WaitsForStartupSettled(t *testing.T) {
	t.Parallel()

	if os.Getenv("TEST_MCP_FAKE_SERVER") == "1" {
		runFakeMCPServer()
		return
	}

	ctx := testutil.Context(t, testutil.WaitLong)
	logger := testutil.Logger(t).Leveled(slog.LevelDebug)
	dir := t.TempDir()
	configPath := filepath.Join(dir, ".mcp.json")

	m := NewManager(ctx, logger, agentexec.DefaultExecer, nil)
	t.Cleanup(func() { _ = m.Close() })

	pathsRequested := make(chan struct{})
	var pathsOnce sync.Once
	api := NewAPI(logger, m, func() []string {
		pathsOnce.Do(func() { close(pathsRequested) })
		return []string{configPath}
	})

	req := httptest.NewRequest(http.MethodGet, "/tools", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		api.Routes().ServeHTTP(rec, req)
		close(done)
	}()

	select {
	case <-pathsRequested:
	case <-ctx.Done():
		t.Fatalf("handler did not request paths: %v", ctx.Err())
	}

	select {
	case <-done:
		t.Fatal("handler returned before startup settled")
	default:
	}

	_, entry := fakeMCPServerConfig(t, "srv")
	writeMCPConfig(t, dir, map[string]mcpServerEntry{"srv": entry})
	m.MarkStartupSettled()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatalf("handler did not return after startup settled: %v", ctx.Err())
	}

	require.Equal(t, http.StatusOK, rec.Code)
	var resp workspacesdk.ListMCPToolsResponse
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Tools, 1)
	assert.Contains(t, resp.Tools[0].Name, "echo")
}

func TestHandleListTools_LogsListErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name         string
		ctx          func() context.Context
		closeManager bool
		message      string
	}{
		{
			name: "Canceled",
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			},
			message: "mcp tool list canceled by caller",
		},
		{
			name: "DeadlineExceeded",
			ctx: func() context.Context {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				cancel()
				return ctx
			},
			message: "mcp tool list timed out",
		},
		{
			name:         "ManagerClosed",
			ctx:          context.Background,
			closeManager: true,
			message:      "mcp tool list failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := tc.ctx()
			sink := testutil.NewFakeSink(t)
			logger := sink.Logger(slog.LevelDebug)
			dir := t.TempDir()
			configPath := filepath.Join(dir, ".mcp.json")

			m := NewManager(context.Background(), logger, agentexec.DefaultExecer, nil)
			m.MarkStartupSettled()
			t.Cleanup(func() { _ = m.Close() })
			if tc.closeManager {
				require.NoError(t, m.Close())
			}

			api := NewAPI(logger, m, func() []string {
				return []string{configPath}
			})

			req := httptest.NewRequest(http.MethodGet, "/tools", nil).WithContext(ctx)
			rec := httptest.NewRecorder()
			api.Routes().ServeHTTP(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			entries := sink.Entries(func(e slog.SinkEntry) bool {
				return e.Message == tc.message
			})
			require.Len(t, entries, 1)
		})
	}
}
