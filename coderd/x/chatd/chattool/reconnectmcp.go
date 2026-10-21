package chattool

import (
	"context"
	"fmt"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// ReconnectMCPServers returns an AgentTool that retries workspace MCP
// servers that failed to connect.
func ReconnectMCPServers(options ProcessToolOptions) fantasy.AgentTool {
	return fantasy.NewAgentTool(
		"reconnect_mcp_servers",
		"Reconnect workspace MCP servers that failed to connect, for example a server started after the workspace came up. "+
			"Connected servers are left alone. Returns the status of each workspace MCP server. "+
			"Tools from reconnected servers become available on your next step.",
		func(ctx context.Context, _ struct{}, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
			if options.GetWorkspaceConn == nil {
				return fantasy.NewTextErrorResponse("workspace connection resolver is not configured"), nil
			}
			conn, err := options.GetWorkspaceConn(ctx)
			if err != nil {
				return fantasy.NewTextErrorResponse(err.Error()), nil
			}
			resp, err := conn.ResyncContext(ctx)
			if err != nil {
				return errorResult(fmt.Sprintf("reconnect MCP servers: %v", err)), nil
			}
			servers := []workspacesdk.ContextSnapshotResource{}
			for _, r := range resp.Resources {
				if r.Kind == "mcp_server" {
					servers = append(servers, r)
				}
			}
			return toolResponse(map[string]any{"servers": servers}), nil
		},
	)
}
