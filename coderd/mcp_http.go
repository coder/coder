package coderd

import (
	"net/http"

	"cdr.dev/slog"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/codersdk"
)

// mcpHTTPHandler creates the MCP HTTP transport handler
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

		authenticatedClient := codersdk.New(api.AccessURL)
		// Extract the original session token from the request
		authenticatedClient.SetSessionToken(httpmw.APITokenFromRequest(r))

		// Register tools with authenticated client
		if err := mcpServer.RegisterTools(authenticatedClient); err != nil {
			api.Logger.Warn(r.Context(), "failed to register MCP tools", slog.Error(err))
		}

		// Handle the MCP request
		mcpServer.ServeHTTP(w, r)
	})
}
