package coderd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/rbac/policy"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
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

		// Wrap the agent connection function to enforce ActionSSH
		// on the workspace. Without this check, a user who can read
		// a workspace but lacks SSH permission could still execute
		// commands through MCP tools.
		toolOpt := toolsdk.WithAgentConnFunc(func(ctx context.Context, agentID uuid.UUID) (workspacesdk.AgentConn, func(), error) {
			if api.Entitlements.Enabled(codersdk.FeatureBrowserOnly) {
				return nil, nil, xerrors.New("non-browser connections are disabled")
			}
			// Use system context for the lookup because the tool
			// handler context does not carry a dbauthz actor. The
			// real authorization happens in the Authorize call below.
			//nolint:gocritic // The system query only fetches the workspace
			// object so we can perform an ActionSSH check against it
			// with the real user's roles via api.Authorize.
			workspace, err := api.Database.GetWorkspaceByAgentID(dbauthz.AsSystemRestricted(ctx), agentID)
			if err != nil {
				return nil, nil, xerrors.Errorf("get workspace by agent ID: %w", err)
			}
			// Enforce the same ActionSSH check that the coordinate
			// endpoint uses (workspaceagents.go:1317).
			if !api.Authorize(r, policy.ActionSSH, workspace) {
				return nil, nil, xerrors.New("unauthorized: you do not have SSH access to this workspace")
			}
			return api.agentProvider.AgentConn(ctx, agentID)
		})

		toolset := MCPToolset(r.URL.Query().Get("toolset"))
		// Default to standard toolset if no toolset is specified.
		if toolset == "" {
			toolset = MCPToolsetStandard
		}

		switch toolset {
		case MCPToolsetStandard:
			if err := mcpServer.RegisterTools(authenticatedClient, toolOpt); err != nil {
				api.Logger.Warn(r.Context(), "failed to register MCP tools", slog.Error(err))
			}
		case MCPToolsetChatGPT:
			if err := mcpServer.RegisterChatGPTTools(authenticatedClient, toolOpt); err != nil {
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
