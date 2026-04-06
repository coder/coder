package agentmcp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API exposes MCP tool discovery and call proxying through the
// agent.
type API struct {
	logger  slog.Logger
	manager *Manager
}

// NewAPI creates a new MCP API handler backed by the given
// manager.
func NewAPI(logger slog.Logger, manager *Manager) *API {
	return &API{
		logger:  logger,
		manager: manager,
	}
}

// Routes returns the HTTP handler for MCP-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/tools", api.handleListTools)
	r.Post("/call-tool", api.handleCallTool)
	return r
}

// handleListTools returns the cached MCP tool definitions,
// optionally refreshing them first if ?refresh=true is set.
func (api *API) handleListTools(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Allow callers to force a tool re-scan before listing.
	if r.URL.Query().Get("refresh") == "true" {
		if err := api.manager.RefreshTools(ctx); err != nil {
			api.logger.Warn(ctx, "failed to refresh MCP tools", slog.Error(err))
		}
	}

	tools := api.manager.Tools()
	// Ensure non-nil so JSON serialization returns [] not null.
	if tools == nil {
		tools = []workspacesdk.MCPToolInfo{}
	}

	httpapi.Write(ctx, rw, http.StatusOK, workspacesdk.ListMCPToolsResponse{
		Tools: tools,
	})
}

// handleCallTool proxies a tool invocation to the appropriate
// MCP server based on the tool name prefix.
func (api *API) handleCallTool(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req workspacesdk.CallMCPToolRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	resp, err := api.manager.CallTool(ctx, req)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(err, ErrInvalidToolName) {
			status = http.StatusBadRequest
		} else if errors.Is(err, ErrUnknownServer) {
			status = http.StatusNotFound
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: "MCP tool call failed.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}
