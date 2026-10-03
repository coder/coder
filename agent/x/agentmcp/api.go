package agentmcp

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// API exposes MCP tool-call proxying through the agent. Tool discovery
// is handled in-process by the agentcontext manager, which reads the
// shared Manager's catalog and pushes it to coderd as pinned context
// resources; this API serves only execution.
type API struct {
	manager *Manager
}

// NewAPI creates a new MCP API handler.
func NewAPI(m *Manager) *API {
	return &API{manager: m}
}

// Routes returns the HTTP handler for MCP-related routes.
func (api *API) Routes() http.Handler {
	r := chi.NewRouter()
	r.Post("/call-tool", api.handleCallTool)
	return r
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
