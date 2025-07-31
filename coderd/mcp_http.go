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

type MCPToolset string

const (
	MCPToolsetStandard MCPToolset = "standard"
	MCPToolsetChatGPT  MCPToolset = "chatgpt"
)

// mcpHTTPHandler creates the MCP HTTP transport handler
// It supports a "toolset" query parameter to select the set of tools to register.
func (api *API) mcpHTTPHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		toolset := MCPToolset(r.URL.Query().Get("toolset"))
		// Default to standard toolset if no toolset is specified.
		if toolset == "" {
			toolset = MCPToolsetStandard
		}

		mcpTools := []toolsdk.GenericTool{}
		switch toolset {
		case MCPToolsetStandard:
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
		case MCPToolsetChatGPT:
			// ChatGPT tools are the search and fetch tools as defined in https://platform.openai.com/docs/mcp.
			// We do not expose any extra ones because ChatGPT has an undocumented "Safety Scan" feature.
			// In my experiments, if I included extra tools in the MCP server, ChatGPT would often - but not always -
			// refuse to add Coder as a connector.
			for _, tool := range toolsdk.All {
				if tool.Name == toolsdk.ToolNameChatGPTSearch || tool.Name == toolsdk.ToolNameChatGPTFetch {
					mcpTools = append(mcpTools, tool)
				}
			}
		default:
			httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
				Message: "Invalid toolset",
			})
			return
		}

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
		if err := mcpServer.RegisterTools(authenticatedClient, mcpTools); err != nil {
			api.Logger.Warn(r.Context(), "failed to register MCP tools", slog.Error(err))
		}

		// Handle the MCP request
		mcpServer.ServeHTTP(w, r)
	})
}
