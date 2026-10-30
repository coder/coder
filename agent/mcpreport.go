package agent

import (
	"github.com/coder/coder/v2/agent/agentcontext"
	"github.com/coder/coder/v2/agent/x/agentmcp"
)

// mcpReportToContext adapts the shared MCP engine's discovery report into
// the agentcontext mirror the resolver turns into KindMCPServer resources
// and KindMCPConfig error overlays. The two types are kept separate so
// agentcontext does not import agent/x/agentmcp.
func mcpReportToContext(report agentmcp.Report) agentcontext.MCPReport {
	out := agentcontext.MCPReport{}
	switch report.Phase {
	case agentmcp.DiscoveryPending:
		out.Phase = agentcontext.MCPDiscoveryPending
	case agentmcp.DiscoveryComplete:
		out.Phase = agentcontext.MCPDiscoveryComplete
	}
	if len(report.Servers) > 0 {
		out.Servers = make([]agentcontext.MCPServerStatus, 0, len(report.Servers))
		for _, s := range report.Servers {
			cs := agentcontext.MCPServerStatus{
				Name:      s.Name,
				Connected: s.Connected,
				Err:       s.Err,
				Warning:   s.Warning,
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
			out.Servers = append(out.Servers, cs)
		}
	}
	if len(report.ConfigErrors) > 0 {
		out.ConfigErrors = make([]agentcontext.MCPConfigError, 0, len(report.ConfigErrors))
		for _, e := range report.ConfigErrors {
			out.ConfigErrors = append(out.ConfigErrors, agentcontext.MCPConfigError{Path: e.Path, Err: e.Err})
		}
	}
	return out
}
