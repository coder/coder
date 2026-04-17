package coderd

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

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
		// Create MCP server instance for each request. Metrics is a shared,
		// process-wide sink; a nil value is safe and disables recording.
		mcpServer, err := mcp.NewServer(
			api.Logger.Named("mcp"),
			mcp.WithMetrics(api.mcpMetrics),
		)
		if err != nil {
			api.Logger.Error(r.Context(), "failed to create MCP server", slog.Error(err))
			httpapi.Write(r.Context(), w, http.StatusInternalServerError, codersdk.Response{
				Message: "MCP server initialization failed",
			})
			return
		}
		// Stash requestor info on the context so individual tool handlers can
		// emit structured logs that identify who made the call.
		r = r.WithContext(mcp.WithRequestor(r.Context(), requestorFromRequest(r)))

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

// requestorFromRequest gathers identity information already extracted by
// upstream middleware (auth, logging) into a compact value we can stash on
// the context for tool-level structured logs. All fields are best-effort;
// unauthenticated requests yield a zero-valued Requestor.
func requestorFromRequest(r *http.Request) mcp.Requestor {
	req := mcp.Requestor{
		RequestID: middleware.GetReqID(r.Context()),
		UserAgent: r.UserAgent(),
	}
	if key, ok := httpmw.APIKeyOptional(r); ok {
		req.UserID = key.UserID.String()
		req.APIKeyID = key.ID
	}
	if subj, ok := httpmw.UserAuthorizationOptional(r.Context()); ok {
		req.Username = subj.FriendlyName
		req.Email = subj.Email
	}
	return req
}
