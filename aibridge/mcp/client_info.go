package mcp

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/coder/coder/v2/buildinfo"
)

// GetClientInfo returns the MCP client information to use when initializing MCP connections.
// This provides a consistent way for all proxy implementations to report client information.
func GetClientInfo() *mcp.Implementation {
	return &mcp.Implementation{
		Name:    "coder/aibridge",
		Version: buildinfo.Version(),
	}
}
