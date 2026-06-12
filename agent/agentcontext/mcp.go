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
	//   - Set Name to the server name (matches Source today;
	//     reserved for the case where a future provider scheme
	//     decouples them).
	//   - Populate ContentHash over a canonical encoding of the
	//     server name plus the tool list (proto Tools field)
	//     so any tool-set change flips the dirty bit.
	//   - Carry a Description summarizing the server.
	//   - Populate Tools with the structured tool list; Payload
	//     is unused for this kind and should be left empty.
	//
	// Implementations should never block; the resolver calls
	// this on every re-resolve.
	MCPResources() []Resource
}
