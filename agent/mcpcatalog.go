package agent

import (
	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/x/agentmcp"
)

// mcpCatalogToContext adapts the shared MCP engine's catalog into the
// agentcontext per-server snapshot the resolver turns into KindMCPServer
// resources. The two types are kept separate so agentcontext does not
// import agent/x/agentmcp.
func mcpCatalogToContext(servers []agentmcp.ServerStatus) []agentcontext.MCPServerStatus {
	if len(servers) == 0 {
		return nil
	}
	out := make([]agentcontext.MCPServerStatus, 0, len(servers))
	for _, s := range servers {
		cs := agentcontext.MCPServerStatus{
			Name:      s.Name,
			Connected: s.Connected,
			Err:       s.Err,
		}
		if len(s.Tools) > 0 {
			cs.Tools = make([]agentcontext.MCPTool, 0, len(s.Tools))
			for _, t := range s.Tools {
				cs.Tools = append(cs.Tools, agentcontext.MCPTool{
					Name:        t.Name,
					Description: t.Description,
					InputSchema: t.InputSchema,
				})
			}
		}
		out = append(out, cs)
	}
	return out
}
