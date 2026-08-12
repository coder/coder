package agentcontext_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/testutil"
)

// TestManager_MCPCatalogSurfacesResources verifies the injected MCP
// catalog is surfaced as KindMCPServer resources, and that a catalog
// change picked up on the next Trigger re-resolves the snapshot. In
// production the shared MCP engine wires SetOnReload to the Manager's
// Trigger so a reload re-publishes the updated tools.
func TestManager_MCPCatalogSurfacesResources(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var mu sync.Mutex
	servers := []agentcontext.MCPServerStatus{{
		Name:      "srv",
		Connected: true,
		Tools:     []agentcontext.MCPTool{{Name: "echo", Description: "echoes input"}},
	}}

	m := newTestManager(t, agentcontext.ManagerOptions{
		WorkingDir: func() string { return dir },
		MCPCatalog: func() []agentcontext.MCPServerStatus {
			mu.Lock()
			defer mu.Unlock()
			return append([]agentcontext.MCPServerStatus(nil), servers...)
		},
	})

	// The eager first snapshot already reflects the injected catalog.
	got := findMCPServerResource(m.Snapshot(), "srv")
	require.NotNil(t, got)
	require.Equal(t, agentcontext.StatusOK, got.Status)
	require.Len(t, got.Tools, 1)
	require.Equal(t, "echo", got.Tools[0].Name)

	ctx := testutil.Context(t, testutil.WaitLong)
	go func() { _ = m.Run(ctx) }()

	// A catalog change re-resolves on the next Trigger.
	mu.Lock()
	servers = []agentcontext.MCPServerStatus{{
		Name:      "srv",
		Connected: true,
		Tools: []agentcontext.MCPTool{
			{Name: "echo"},
			{Name: "ping"},
		},
	}}
	mu.Unlock()
	m.Trigger()

	require.Eventually(t, func() bool {
		got := findMCPServerResource(m.Snapshot(), "srv")
		return got != nil && len(got.Tools) == 2
	}, testutil.WaitShort, testutil.IntervalMedium,
		"catalog change should re-resolve into the snapshot")
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
