package coderd

import (
	"net/http"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

// mcpHTTPHandler creates the MCP HTTP transport handler
func (api *API) mcpHTTPHandler(tools []toolsdk.GenericTool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Create MCP server instance for each request
		mcpServer, err := mcp.NewServer(api.Logger.Named("mcp"))
		if err != nil {
			api.Logger.Error(r.Context(), "failed to create MCP server", slog.Error(err))
			httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
				Message: "MCP server initialization failed",
			})
			return
		}

		authenticatedClient := codersdk.New(api.AccessURL)
		// Extract the original session token from the request
		authenticatedClient.SetSessionToken(httpmw.APITokenFromRequest(r))

		// Register tools with authenticated client
		if err := mcpServer.RegisterTools(authenticatedClient, tools); err != nil {
			api.Logger.Warn(r.Context(), "failed to register MCP tools", slog.Error(err))
		}

		// Handle the MCP request
		mcpServer.ServeHTTP(w, r)
	})
}

// standardMCPHTTPHandler sets up the MCP HTTP transport handler for the standard tools.
// Standard tools are all tools except for the report task, ChatGPT search, and ChatGPT fetch tools.
func (api *API) standardMCPHTTPHandler() http.Handler {
	mcpTools := []toolsdk.GenericTool{}
	// Register all available tools, but exclude:
	// - ReportTask - which requires dependencies not available in the remote MCP context
	// - ChatGPT search and fetch tools, which are redundant with the standard tools.
	for _, tool := range toolsdk.All {
		if tool.Name == toolsdk.ToolNameReportTask ||
			tool.Name == toolsdk.ToolNameChatGPTSearch || tool.Name == toolsdk.ToolNameChatGPTFetch {
			continue
		}
		mcpTools = append(mcpTools, tool)
	}
	return api.mcpHTTPHandler(mcpTools)
}

// chatgptMCPHTTPHandler sets up the MCP HTTP transport handler for the ChatGPT tools.
// ChatGPT tools are the search and fetch tools as defined in https://platform.openai.com/docs/mcp.
// We do not expose any extra ones because ChatGPT has an undocumented "Safety Scan" feature.
// In my experiments, if I included extra tools in the MCP server, ChatGPT would refuse
// to add Coder as a connector.
func (api *API) chatgptMCPHTTPHandler() http.Handler {
	mcpTools := []toolsdk.GenericTool{}
	// Register only the ChatGPT search and fetch tools.
	for _, tool := range toolsdk.All {
		if !(tool.Name == toolsdk.ToolNameChatGPTSearch || tool.Name == toolsdk.ToolNameChatGPTFetch) {
			continue
		}
		mcpTools = append(mcpTools, tool)
	}
	return api.mcpHTTPHandler(mcpTools)
}
