package agentcontext

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/require"
)

func TestParseMCPConfig(t *testing.T) {
	t.Parallel()

	write := func(t *testing.T, body string) string {
		t.Helper()
		dir := t.TempDir()
		path := filepath.Join(dir, ".mcp.json")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		return path
	}

	t.Run("InfersTransportAndSorts", func(t *testing.T) {
		t.Parallel()
		path := write(t, `{"mcpServers": {
			"zebra": {"command": "zebra-bin", "args": ["--flag"]},
			"alpha": {"url": "https://example.com/mcp"}
		}}`)
		got, err := parseMCPConfig(path)
		require.NoError(t, err)
		require.Len(t, got, 2)
		// Sorted by name.
		require.Equal(t, "alpha", got[0].Name)
		require.Equal(t, "http", got[0].Transport)
		require.Equal(t, "https://example.com/mcp", got[0].URL)
		require.Equal(t, "zebra", got[1].Name)
		require.Equal(t, "stdio", got[1].Transport)
		require.Equal(t, "zebra-bin", got[1].Command)
		require.Equal(t, []string{"--flag"}, got[1].Args)
	})

	t.Run("ExplicitTypeWins", func(t *testing.T) {
		t.Parallel()
		path := write(t, `{"mcpServers": {"s": {"type": "sse", "url": "https://x"}}}`)
		got, err := parseMCPConfig(path)
		require.NoError(t, err)
		require.Len(t, got, 1)
		require.Equal(t, "sse", got[0].Transport)
	})

	t.Run("EmptyServers", func(t *testing.T) {
		t.Parallel()
		path := write(t, `{"mcpServers": {}}`)
		got, err := parseMCPConfig(path)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("RejectsServerWithoutCommandOrURL", func(t *testing.T) {
		t.Parallel()
		path := write(t, `{"mcpServers": {"s": {}}}`)
		_, err := parseMCPConfig(path)
		require.Error(t, err)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		t.Parallel()
		path := write(t, `{not json`)
		_, err := parseMCPConfig(path)
		require.Error(t, err)
	})
}

// TestParseMCPConfig_ExpandsEnv is a standalone (non-parallel) test
// because t.Setenv cannot be used under a parallel parent test.
func TestParseMCPConfig_ExpandsEnv(t *testing.T) {
	t.Setenv("AGENTCONTEXT_MCP_TEST_TOKEN", "secret")
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"mcpServers": {"s": {"command": "x", "env": {"TOKEN": "${AGENTCONTEXT_MCP_TEST_TOKEN}"}}}}`), 0o600))
	got, err := parseMCPConfig(path)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "secret", got[0].Env["TOKEN"])
}

func TestToolInputSchema(t *testing.T) {
	t.Parallel()

	t.Run("FullSchema", func(t *testing.T) {
		t.Parallel()
		got := toolInputSchema(mcp.ToolInputSchema{
			Type:       "object",
			Properties: map[string]any{"q": map[string]any{"type": "string"}},
			Required:   []string{"q"},
		})
		require.Equal(t, "object", got["type"])
		require.Equal(t, map[string]any{"q": map[string]any{"type": "string"}}, got["properties"])
		// Required is converted to []any so structpb.NewStruct accepts it.
		require.Equal(t, []any{"q"}, got["required"])
	})

	t.Run("EmptyYieldsNil", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, toolInputSchema(mcp.ToolInputSchema{}))
	})

	t.Run("TypeOnly", func(t *testing.T) {
		t.Parallel()
		got := toolInputSchema(mcp.ToolInputSchema{Type: "object"})
		require.Equal(t, map[string]any{"type": "object"}, got)
	})
}

func TestMCPConfigSet(t *testing.T) {
	t.Parallel()

	t.Run("Empty", func(t *testing.T) {
		t.Parallel()
		paths, key := mcpConfigSet(Snapshot{})
		require.Empty(t, paths)
		require.Empty(t, key)
	})

	t.Run("SortedAndKeyedByContentHash", func(t *testing.T) {
		t.Parallel()
		snap := Snapshot{Resources: []Resource{
			{Kind: KindMCPConfig, Source: "/b/.mcp.json", ContentHash: [32]byte{0x01}},
			{Kind: KindMCPConfig, Source: "/a/.mcp.json", ContentHash: [32]byte{0x02}},
			// Non-config and empty-source resources are ignored.
			{Kind: KindInstructionFile, Source: "/a/AGENTS.md"},
			{Kind: KindMCPServer, Source: "fs"},
			{Kind: KindMCPConfig, Source: ""},
		}}
		paths, key := mcpConfigSet(snap)
		require.Equal(t, []string{"/a/.mcp.json", "/b/.mcp.json"}, paths)
		require.NotEmpty(t, key)

		// An in-place content edit (same path, new hash) changes the key.
		snap2 := Snapshot{Resources: []Resource{
			{Kind: KindMCPConfig, Source: "/b/.mcp.json", ContentHash: [32]byte{0x01}},
			{Kind: KindMCPConfig, Source: "/a/.mcp.json", ContentHash: [32]byte{0x09}},
		}}
		_, key2 := mcpConfigSet(snap2)
		require.NotEqual(t, key, key2)
	})
}
