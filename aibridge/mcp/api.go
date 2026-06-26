package mcp

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

// ServerProxier provides an abstraction to communicate with MCP Servers regardless of their transport.
// The ServerProxier is expected to, at least, fetch any available MCP tools.
type ServerProxier interface {
	// Init initializes the proxier, establishing a connection with the upstream server and fetching resources.
	Init(context.Context) error
	// Gracefully shut down connections to the MCP server. Session management will vary per transport.
	// See https://modelcontextprotocol.io/specification/2025-06-18/basic/transports#session-management.
	Shutdown(ctx context.Context) error

	// ListTools lists all known tools. These MUST be sorted in a stable order.
	ListTools() []*Tool
	// GetTool returns a given tool, if known, or returns nil.
	GetTool(id string) *Tool
	// CallTool invokes an injected MCP tool
	CallTool(ctx context.Context, name string, input any) (*mcp.CallToolResult, error)
}

// TODO: support HTTP+SSE.
