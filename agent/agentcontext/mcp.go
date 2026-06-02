package agentcontext

// MCPProvider supplies the live MCP server portion of a
// snapshot. Implementations typically wrap an existing MCP
// manager (e.g. agent/x/agentmcp.Manager) and translate each
// server's tool list into a KindMCPServer resource.
//
// The interface is intentionally minimal so the existing MCP
// lifecycle code can be reused without refactoring; a follow-up
// change absorbs the lifecycle into this package.
type MCPProvider interface {
	// MCPResources returns one Resource per MCP server known
	// to the provider. Each Resource must:
	//
	//   - Have Kind == KindMCPServer.
	//   - Use the server name as Source.
	//   - Populate ContentHash over the canonical encoding
	//     of the tool list so changes flip the dirty bit.
	//   - Carry a Description summarizing the server.
	//
	// Implementations should never block; the resolver calls
	// this on every re-resolve.
	MCPResources() []Resource
}
