package coderd

import (
	"fmt"
	"net/http"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
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
		// Create MCP server instance for each request
		mcpServer, err := mcp.NewServer(api.Logger.Named("mcp"))
		if err != nil {
			api.Logger.Error(r.Context(), "failed to create MCP server", slog.Error(err))
			httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
				Message: "MCP server initialization failed",
			})
			return
		}
		// Extract the original session token from the request
		authenticatedClient := codersdk.New(api.AccessURL,
			codersdk.WithSessionToken(httpmw.APITokenFromRequest(r)))
		toolset := MCPToolset(r.URL.Query().Get("toolset"))
		// Default to standard toolset if no toolset is specified.
		if toolset == "" {
			toolset = MCPToolsetStandard
		}

		switch toolset {
		case MCPToolsetStandard:
			if err := mcpServer.RegisterTools(authenticatedClient); err != nil {
				api.Logger.Warn(r.Context(), "failed to register MCP tools", slog.Error(err))
			}
		case MCPToolsetChatGPT:
			if err := mcpServer.RegisterChatGPTTools(authenticatedClient); err != nil {
				api.Logger.Warn(r.Context(), "failed to register MCP tools", slog.Error(err))
			}
		default:
			httpapi.Write(r.Context(), w, http.StatusBadRequest, codersdk.Response{
				Message: fmt.Sprintf("Invalid toolset: %s", toolset),
			})
			return
		}

		// Handle the MCP request
		mcpServer.ServeHTTP(w, r)
	})
}
