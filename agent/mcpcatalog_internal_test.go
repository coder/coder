package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/agent/x/agentmcp"
)

func TestMCPCatalogMetadata(t *testing.T) {
	t.Parallel()
	meta := map[string]any{"ui": map[string]any{"resourceUri": "ui://view"}}
	catalog := mcpCatalogToContext([]agentmcp.ServerStatus{{Name: "apps", Connected: true, Tools: []agentmcp.ToolInfo{{Name: "view", Meta: meta}}}})
	require.Len(t, catalog, 1)
	require.Len(t, catalog[0].Tools, 1)
	require.Equal(t, meta, catalog[0].Tools[0].Meta)
}
