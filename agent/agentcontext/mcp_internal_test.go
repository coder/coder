package agentcontext

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildMCPServerResources(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, buildMCPServerResources(nil))
		require.Nil(t, buildMCPServerResources([]MCPServerStatus{}))
	})

	t.Run("GroupsByServerSortedWithTools", func(t *testing.T) {
		t.Parallel()
		// Tool names are whatever the server reported; the runner no
		// longer prefixes them with the server name.
		servers := []MCPServerStatus{
			{Name: "github", Connected: true, Tools: []MCPTool{
				{Name: "search", Description: "Search"},
				{Name: "create", Description: "Create"},
			}},
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read", InputSchema: map[string]any{"type": "object"}},
			}},
			// Dropped: a server with no name cannot be addressed.
			{Name: "", Connected: true, Tools: []MCPTool{{Name: "orphan"}}},
		}
		got := buildMCPServerResources(servers)
		require.Len(t, got, 2)

		// Servers are emitted in name order: fs, then github.
		require.Equal(t, "fs", got[0].Source)
		require.Equal(t, "fs", got[0].Name)
		require.Equal(t, KindMCPServer, got[0].Kind)
		require.Equal(t, "mcp_server:fs", got[0].ID)
		require.Equal(t, StatusOK, got[0].Status)
		require.NotEqual(t, [32]byte{}, got[0].ContentHash)
		require.Len(t, got[0].Tools, 1)
		require.Equal(t, "read", got[0].Tools[0].Name)
		require.Equal(t, map[string]any{"type": "object"}, got[0].Tools[0].InputSchema)

		require.Equal(t, "github", got[1].Source)
		require.Len(t, got[1].Tools, 2)
		// Tools within a server are sorted by name: create, then search.
		require.Equal(t, "create", got[1].Tools[0].Name)
		require.Equal(t, "search", got[1].Tools[1].Name)
	})

	t.Run("ConnectedWithoutToolsSkipped", func(t *testing.T) {
		t.Parallel()
		// A connected server that has not yet reported any tools is
		// not surfaced; a later re-resolve picks it up once tools
		// arrive.
		require.Nil(t, buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true},
		}))
	})

	t.Run("FailedServerSurfacesAsIssue", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources([]MCPServerStatus{
			{Name: "broken", Connected: false, Err: "initialize \"broken\": exec: no such file"},
		})
		require.Len(t, got, 1)
		require.Equal(t, KindMCPServer, got[0].Kind)
		require.Equal(t, "broken", got[0].Source)
		require.Equal(t, "broken", got[0].Name)
		require.Equal(t, "mcp_server:broken", got[0].ID)
		require.Equal(t, StatusUnreadable, got[0].Status)
		require.Equal(t, "initialize \"broken\": exec: no such file", got[0].Error)
		require.Empty(t, got[0].Tools)
		require.NotEqual(t, [32]byte{}, got[0].ContentHash)
	})

	t.Run("FailedServerWithoutErrorGetsDefault", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources([]MCPServerStatus{
			{Name: "broken", Connected: false},
		})
		require.Len(t, got, 1)
		require.Equal(t, StatusUnreadable, got[0].Status)
		require.Equal(t, "failed to connect", got[0].Error)
	})

	t.Run("ContentHashStableAndToolSensitive", func(t *testing.T) {
		t.Parallel()
		base := []MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read"},
			}},
		}
		h1 := buildMCPServerResources(base)[0].ContentHash
		// Identical input is hashed identically.
		require.Equal(t, h1, buildMCPServerResources(base)[0].ContentHash)
		// A description change flips the hash.
		require.NotEqual(t, h1, buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read files"},
			}},
		})[0].ContentHash)
		// Adding a tool flips the hash.
		require.NotEqual(t, h1, buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read"},
				{Name: "write", Description: "Write"},
			}},
		})[0].ContentHash)
		// A schema change flips the hash.
		require.NotEqual(t, h1, buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "Read", InputSchema: map[string]any{"type": "object"}},
			}},
		})[0].ContentHash)
	})

	t.Run("FailedServerHashErrorSensitive", func(t *testing.T) {
		t.Parallel()
		h1 := buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: false, Err: "boom"},
		})[0].ContentHash
		// The error text participates in the hash so a changed error
		// is detectable.
		require.NotEqual(t, h1, buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: false, Err: "different"},
		})[0].ContentHash)
		// A failed server hashes differently from a connected one, so
		// the connected->failed transition is detectable.
		require.NotEqual(t, h1, buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{
				{Name: "read", Description: "boom"},
			}},
		})[0].ContentHash)
	})

	t.Run("MixedServersSortedByName", func(t *testing.T) {
		t.Parallel()
		// Failed and connected servers are emitted together in name
		// order: broken (failed) before fs (ok).
		got := buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{{Name: "read"}}},
			{Name: "broken", Connected: false, Err: "nope"},
		})
		require.Len(t, got, 2)
		require.Equal(t, "broken", got[0].Source)
		require.Equal(t, StatusUnreadable, got[0].Status)
		require.Equal(t, "fs", got[1].Source)
		require.Equal(t, StatusOK, got[1].Status)
	})
}
