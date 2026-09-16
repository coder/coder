package chatd

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/x/chatd/chatprovider"
	"github.com/coder/coder/v2/coderd/x/chatd/mcpclient"
)

// maxMCPAppProxyResultBytes bounds the serialized MCP result returned
// to a rendered app.
const maxMCPAppProxyResultBytes = 4 << 20

// MCPAppRequestError is a request-level failure of an app-initiated
// MCP call: the tool or resource is unknown, not callable by apps, or
// the server answered with an error. The message is safe to return to
// the caller.
type MCPAppRequestError struct {
	Message string
}

func (e *MCPAppRequestError) Error() string { return e.Message }

// IsMCPAppRequestError reports whether err is an MCPAppRequestError.
func IsMCPAppRequestError(err error) bool {
	var target *MCPAppRequestError
	return errors.As(err, &target)
}

// CallMCPAppTool invokes a tool on one of the chat's MCP servers on
// behalf of an app rendered from that server. Only tools the server
// currently lists, that pass the config's allow and deny lists, and
// whose UI visibility includes "app" may be called. The raw
// CallToolResult is returned as JSON.
func (p *Server) CallMCPAppTool(
	ctx context.Context,
	chat database.Chat,
	cfg database.MCPServerConfig,
	toolName string,
	arguments map[string]any,
) (json.RawMessage, error) {
	if strings.TrimSpace(toolName) == "" {
		return nil, &MCPAppRequestError{Message: "Tool name is required."}
	}
	session, err := p.dialMCPAppSession(ctx, chat, cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	var tool *mcp.Tool
	for _, candidate := range session.Tools() {
		if candidate.Name == toolName {
			tool = candidate
			break
		}
	}
	if tool == nil {
		return nil, &MCPAppRequestError{Message: "The MCP server does not expose a tool with that name."}
	}
	if !mcpclient.IsToolAllowed(tool.Name, cfg.ToolAllowList, cfg.ToolDenyList) {
		return nil, &MCPAppRequestError{Message: "The tool is not allowed by the MCP server configuration."}
	}
	if !mcpclient.ParseToolUIMeta(tool.Meta).AppVisible() {
		return nil, &MCPAppRequestError{Message: "The tool is not callable by apps."}
	}

	result, err := session.CallTool(ctx, tool.Name, arguments)
	if err != nil {
		return nil, &MCPAppRequestError{Message: "The MCP server rejected the tool call: " + mcpclient.RedactErrorURL(err)}
	}
	return marshalMCPAppResult(result)
}

// ReadMCPAppResource reads a resource from one of the chat's MCP
// servers on behalf of an app rendered from that server. Apps may
// read ui:// resources and any resource the server lists; other URIs
// are rejected. The raw ReadResourceResult is returned as JSON.
func (p *Server) ReadMCPAppResource(
	ctx context.Context,
	chat database.Chat,
	cfg database.MCPServerConfig,
	uri string,
) (json.RawMessage, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, &MCPAppRequestError{Message: "Resource URI is required."}
	}
	session, err := p.dialMCPAppSession(ctx, chat, cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	if !strings.HasPrefix(uri, mcpclient.UIResourceScheme) {
		listed, listErr := session.ListResources(ctx)
		if listErr != nil {
			return nil, &MCPAppRequestError{Message: "The MCP server does not list that resource."}
		}
		if !slices.ContainsFunc(listed, func(r *mcp.Resource) bool { return r.URI == uri }) {
			return nil, &MCPAppRequestError{Message: "The MCP server does not list that resource."}
		}
	}

	result, err := session.ReadResource(ctx, uri)
	if err != nil {
		return nil, &MCPAppRequestError{Message: "The MCP server rejected the resource read: " + mcpclient.RedactErrorURL(err)}
	}
	return marshalMCPAppResult(result)
}

// dialMCPAppSession opens a short-lived session to cfg using the chat
// owner's credentials, refreshing an expired OAuth token first. The
// caller must Close the session.
func (p *Server) dialMCPAppSession(
	ctx context.Context,
	chat database.Chat,
	cfg database.MCPServerConfig,
) (*mcpclient.Session, error) {
	if !slices.Contains(chat.MCPServerIDs, cfg.ID) {
		return nil, &MCPAppRequestError{Message: "The MCP server is not attached to this chat."}
	}
	if !cfg.Enabled || cfg.OrganizationID != chat.OrganizationID {
		return nil, &MCPAppRequestError{Message: "The MCP server is not available to this chat."}
	}
	logger := p.logger.With(
		slog.F("chat_id", chat.ID),
		slog.F("server_slug", cfg.Slug),
	)

	tokens, err := p.db.GetMCPServerUserTokensByUserID(ctx, chat.OwnerID)
	if err != nil {
		return nil, xerrors.Errorf("load MCP user tokens: %w", err)
	}
	tokens = slices.DeleteFunc(tokens, func(tok database.MCPServerUserToken) bool {
		return tok.MCPServerConfigID != cfg.ID
	})
	tokens = p.refreshExpiredMCPTokens(ctx, logger, []database.MCPServerConfig{cfg}, tokens)

	session, err := mcpclient.DialSession(
		ctx,
		logger,
		cfg,
		tokens,
		chat.OwnerID,
		p.oidcTokenSource,
		chatprovider.CoderHeaders(chat),
		p.mcpHTTPClient,
		mcpclient.ConnectOptions{MCPApps: true},
	)
	if err != nil {
		return nil, &MCPAppRequestError{Message: "Failed to connect to the MCP server: " + mcpclient.RedactErrorURL(err)}
	}
	return session, nil
}

func marshalMCPAppResult(result any) (json.RawMessage, error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, xerrors.Errorf("marshal MCP result: %w", err)
	}
	if len(data) > maxMCPAppProxyResultBytes {
		return nil, &MCPAppRequestError{Message: "The MCP server result is too large."}
	}
	return data, nil
}
