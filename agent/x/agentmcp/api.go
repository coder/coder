package agentmcp

import (
	"context"
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
	logger         slog.Logger
	manager        *Manager
	mcpConfigFiles func() []string
}

// NewAPI creates a new MCP API handler backed by the given
// manager. The mcpConfigFiles callback returns the current
// resolved config file paths; it is called on every tool-list
// request to detect config changes.
func NewAPI(
	logger slog.Logger,
	manager *Manager,
	mcpConfigFiles func() []string,
) *API {
	return &API{
		logger:         logger,
		manager:        manager,
		mcpConfigFiles: mcpConfigFiles,
	}
}

// Routes returns the HTTP handler for MCP-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/tools", api.handleListTools)
	r.Post("/call-tool", api.handleCallTool)
	return r
}

// handleListTools checks whether any .mcp.json config file
// has changed since the last reload, triggering a differential
// reload if so, then returns the cached MCP tool definitions.
// The ?refresh=true query parameter forces a tool re-scan
// independent of config changes.
func (api *API) handleListTools(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check config freshness and reload if changed.
	var reloaded bool
	paths := api.mcpConfigFiles()
	if api.manager.SnapshotChanged(paths) {
		if err := api.manager.Reload(ctx, paths); err != nil {
			// Categorize the error for operator debugging.
			switch {
			case errors.Is(err, context.Canceled):
				api.logger.Warn(ctx, "mcp reload canceled by caller", slog.Error(err))
			case errors.Is(err, context.DeadlineExceeded):
				api.logger.Warn(ctx, "mcp reload timed out", slog.Error(err))
			default:
				api.logger.Warn(ctx, "mcp reload failed", slog.Error(err))
			}
			// Fall through to return whatever tools we have.
		} else {
			reloaded = true
		}
	}

	// Allow callers to force a tool re-scan before listing.
	// Skip if a config reload ran above, since it already
	// refreshes tools as part of the reload.
	if r.URL.Query().Get("refresh") == "true" && !reloaded {
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
