package coderd

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/mcp"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

type MCPToolset string

const (
	MCPToolsetStandard MCPToolset = "standard"
	MCPToolsetChatGPT  MCPToolset = "chatgpt"
)

type sharedTailnetAgentDialer struct {
	provider workspaceapps.AgentProvider
}

func (d sharedTailnetAgentDialer) DialAgent(ctx context.Context, agentID uuid.UUID, _ *workspacesdk.DialAgentOptions) (workspacesdk.AgentConn, error) {
	if d.provider == nil {
		return nil, xerrors.New("agent provider is unavailable")
	}

	conn, release, err := d.provider.AgentConn(ctx, agentID)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, xerrors.New("agent provider returned nil connection")
	}
	if release == nil {
		return nil, xerrors.New("agent provider returned nil release function")
	}

	return workspacesdk.WrapAgentConn(conn, func() error {
		release()
		return nil
	}), nil
}

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
		dialer := sharedTailnetAgentDialer{provider: api.agentProvider}
		toolOpt := toolsdk.WithAgentDialer(dialer)
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
