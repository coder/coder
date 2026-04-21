package mcp

import (
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/coder/aibridge/buildinfo"
)

// GetClientInfo returns the MCP client information to use when initializing MCP connections.
// This provides a consistent way for all proxy implementations to report client information.
func GetClientInfo() mcp.Implementation {
	return mcp.Implementation{
		Name:    "coder/aibridge",
		Version: buildinfo.Version(),
	}
}
