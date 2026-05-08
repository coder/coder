package agentmcp

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/agentchat"
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

// NewAPI creates a new MCP API handler. mcpConfigFiles returns
// the resolved .mcp.json paths and is called on every tool-list
// request to detect config changes.
func NewAPI(logger slog.Logger, m *Manager, mcpConfigFiles func() []string) *API {
	return &API{logger: logger, manager: m, mcpConfigFiles: mcpConfigFiles}
}

// Routes returns the HTTP handler for MCP-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Get("/tools", api.handleListTools)
	r.Post("/call-tool", api.handleCallTool)
	return r
}

// handleListTools returns the current MCP tool cache after the
// manager performs startup-safe config synchronization.
func (api *API) handleListTools(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logger := api.logger.With(agentchat.Fields(ctx)...)

	tools, err := api.manager.ListTools(ctx, api.mcpConfigFiles())
	if err != nil {
		switch {
		case errors.Is(err, context.Canceled):
			logger.Warn(ctx, "mcp tool list canceled by caller", slog.Error(err))
		case errors.Is(err, context.DeadlineExceeded):
			logger.Warn(ctx, "mcp tool list timed out", slog.Error(err))
		default:
			logger.Warn(ctx, "mcp tool list failed", slog.Error(err))
		}
	}
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
