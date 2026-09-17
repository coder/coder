package agentcontext

import (
	"os"
	"path/filepath"
	"runtime"
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

	t.Run("ConnectedWithoutToolsEmitted", func(t *testing.T) {
		t.Parallel()
		// A connected server with zero tools is still an OK resource,
		// so an empty inventory is distinguishable from a server that
		// has not been discovered yet.
		got := buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true},
		})
		require.Len(t, got, 1)
		require.Equal(t, StatusOK, got[0].Status)
		require.Empty(t, got[0].Tools)
		require.Empty(t, got[0].Error)
	})

	t.Run("WarningKeptOnOKRow", func(t *testing.T) {
		t.Parallel()
		got := buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true, Warning: "reconnect failed", Tools: []MCPTool{{Name: "read"}}},
		})
		require.Len(t, got, 1)
		require.Equal(t, StatusOK, got[0].Status)
		require.Equal(t, "reconnect failed", got[0].Error)
		require.Len(t, got[0].Tools, 1)
		// The warning does not participate in the content hash; the
		// live-sync compares error text separately.
		require.Equal(t, got[0].ContentHash, buildMCPServerResources([]MCPServerStatus{
			{Name: "fs", Connected: true, Tools: []MCPTool{{Name: "read"}}},
		})[0].ContentHash)
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

func TestApplyMCPConfigErrors(t *testing.T) {
	t.Parallel()

	okConfig := func(path string) Resource {
		return Resource{ID: resourceID(KindMCPConfig, path), Kind: KindMCPConfig, Source: path, Status: StatusOK}
	}

	t.Run("ExactPathMarksInvalid", func(t *testing.T) {
		t.Parallel()
		resources := []Resource{
			okConfig("/w/.mcp.json"),
			{ID: "instruction_file:/w/AGENTS.md", Kind: KindInstructionFile, Source: "/w/AGENTS.md", Status: StatusOK},
		}
		resources = applyMCPConfigErrors(resources, []MCPConfigError{{Path: "/w/.mcp.json", Err: "server \"a\" has no command or url"}})
		require.Equal(t, StatusInvalid, resources[0].Status)
		require.Equal(t, "server \"a\" has no command or url", resources[0].Error)
		require.Equal(t, StatusOK, resources[1].Status)
	})

	t.Run("KeepsFilesystemDiagnosis", func(t *testing.T) {
		t.Parallel()
		resources := []Resource{{
			ID: resourceID(KindMCPConfig, "/w/.mcp.json"), Kind: KindMCPConfig, Source: "/w/.mcp.json",
			Status: StatusOversize, Error: "too big",
		}}
		resources = applyMCPConfigErrors(resources, []MCPConfigError{{Path: "/w/.mcp.json", Err: "parse"}})
		require.Equal(t, StatusOversize, resources[0].Status)
		require.Equal(t, "too big", resources[0].Error)
	})

	t.Run("UnmatchedPathSynthesizesInvalidRow", func(t *testing.T) {
		t.Parallel()
		// A configured file the resolver does not recognize by name
		// (CODER_AGENT_EXP_MCP_CONFIG_FILES=/opt/custom.json) has no
		// row of its own, so its diagnostic gets one.
		resources := applyMCPConfigErrors([]Resource{okConfig("/w/.mcp.json")}, []MCPConfigError{{Path: "/opt/custom.json", Err: "parse"}})
		require.Len(t, resources, 2)
		require.Equal(t, StatusOK, resources[0].Status)
		require.Equal(t, Resource{
			ID: resourceID(KindMCPConfig, "/opt/custom.json"), Kind: KindMCPConfig, Source: "/opt/custom.json",
			Status: StatusInvalid, Error: "parse",
		}, resources[1])
	})

	t.Run("UnmatchedPathSharedWithAnotherKindIsDropped", func(t *testing.T) {
		t.Parallel()
		// A custom config entry pointing at a file already emitted under
		// another kind gets no second row: coderd rejects duplicate
		// sources regardless of kind, which would fail the whole push.
		resources := applyMCPConfigErrors([]Resource{
			{ID: "instruction_file:/w/AGENTS.md", Kind: KindInstructionFile, Source: "/w/AGENTS.md", Status: StatusOK},
		}, []MCPConfigError{{Path: "/w/AGENTS.md", Err: "parse"}})
		require.Len(t, resources, 1)
		require.Equal(t, StatusOK, resources[0].Status)
	})

	t.Run("SymlinkedConfigMatchesTarget", func(t *testing.T) {
		t.Parallel()
		if runtime.GOOS == "windows" {
			t.Skip("symlinks require admin privileges on Windows runners")
		}
		dir := t.TempDir()
		target := filepath.Join(dir, "real", ".mcp.json")
		require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o755))
		require.NoError(t, os.WriteFile(target, []byte("{}"), 0o600))
		link := filepath.Join(dir, ".mcp.json")
		require.NoError(t, os.Symlink(target, link))

		// The resolver walked the symlink; the engine reported the target.
		resources := []Resource{okConfig(link)}
		resources = applyMCPConfigErrors(resources, []MCPConfigError{{Path: target, Err: "semantic"}})
		require.Equal(t, StatusInvalid, resources[0].Status)
		require.Equal(t, "semantic", resources[0].Error)
	})
}
