package agentcontext_test

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

// TestManager_MCPReportSurfacesResources verifies the injected MCP report
// is surfaced as KindMCPServer resources, and that a report change picked
// up on the next Trigger re-resolves the snapshot. In production the
// shared MCP engine wires SetOnReload to the Manager's Trigger so a reload
// re-publishes the updated tools.
func TestManager_MCPReportSurfacesResources(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var mu sync.Mutex
	report := agentcontext.MCPReport{Servers: []agentcontext.MCPServerStatus{{
		Name:      "srv",
		Connected: true,
		Tools:     []agentcontext.MCPTool{{Name: "echo", Description: "echoes input"}},
	}}}

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
		MCPReport: func() agentcontext.MCPReport {
			mu.Lock()
			defer mu.Unlock()
			return report
		},
	})

	// The eager first snapshot already reflects the injected report.
	got := findMCPServerResource(m.Snapshot(), "srv")
	require.NotNil(t, got)
	require.Equal(t, agentcontext.StatusOK, got.Status)
	require.Len(t, got.Tools, 1)
	require.Equal(t, "echo", got.Tools[0].Name)

	ctx := testutil.Context(t, testutil.WaitLong)
	go func() { _ = m.Run(ctx) }()

	// A report change re-resolves on the next Trigger.
	mu.Lock()
	report = agentcontext.MCPReport{Servers: []agentcontext.MCPServerStatus{{
		Name:      "srv",
		Connected: true,
		Warning:   "reconnect failed",
		Tools: []agentcontext.MCPTool{
			{Name: "echo"},
			{Name: "ping"},
		},
	}}}
	mu.Unlock()
	m.Trigger()

	require.Eventually(t, func() bool {
		got := findMCPServerResource(m.Snapshot(), "srv")
		return got != nil && len(got.Tools) == 2 && got.Error == "reconnect failed"
	}, testutil.WaitShort, testutil.IntervalMedium,
		"report change should re-resolve into the snapshot")
}

// TestManager_MCPReportConfigErrorOverlay verifies that a config error the
// engine attributes to a .mcp.json the filesystem pass found structurally
// valid flips that KindMCPConfig resource to StatusInvalid, and that the
// match survives a symlinked config file.
func TestManager_MCPReportConfigErrorOverlay(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		symlink bool
	}{
		{name: "DirectFile"},
		{name: "SymlinkedFile", symlink: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			configPath := filepath.Join(dir, ".mcp.json")
			// Structurally valid JSON; the server entry has neither
			// command nor url, which only the engine rejects.
			content := []byte(`{"mcpServers":{"broken":{"args":["x"]}}}`)
			reportedPath := configPath
			if tc.symlink {
				// The target stays inside the scan root; the resolver
				// rejects symlinks that escape it.
				target := filepath.Join(dir, "configs", "real.mcp.json")
				require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
				require.NoError(t, os.WriteFile(target, content, 0o600))
				require.NoError(t, os.Symlink(target, configPath))
				reportedPath = target
			} else {
				require.NoError(t, os.WriteFile(configPath, content, 0o600))
			}

			m := newTestManager(t, agentcontext.ManagerOptions{
				WorkingDir: func() string { return dir },
				MCPReport: func() agentcontext.MCPReport {
					return agentcontext.MCPReport{ConfigErrors: []agentcontext.MCPConfigError{{
						Path: reportedPath,
						Err:  "server \"broken\" has no command or url",
					}}}
				},
			})

			var cfg *agentcontext.Resource
			for i := range m.Snapshot().Resources {
				if r := &m.Snapshot().Resources[i]; r.Kind == agentcontext.KindMCPConfig {
					cfg = r
				}
			}
			require.NotNil(t, cfg, "the .mcp.json must still be discovered")
			require.Equal(t, configPath, cfg.Source, "the resource keeps the walked path")
			require.Equal(t, agentcontext.StatusInvalid, cfg.Status)
			require.Equal(t, "server \"broken\" has no command or url", cfg.Error)
			require.Nil(t, findMCPServerResource(m.Snapshot(), "broken"), "no server row is fabricated")
		})
	}
}

// findMCPServerResource returns the KindMCPServer resource for the named
// server, or nil if absent.
func findMCPServerResource(snap agentcontext.Snapshot, name string) *agentcontext.Resource {
	for i := range snap.Resources {
		if r := snap.Resources[i]; r.Kind == agentcontext.KindMCPServer && r.Source == name {
			return &snap.Resources[i]
		}
	}
	return nil
}
