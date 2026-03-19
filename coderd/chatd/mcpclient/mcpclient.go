package mcpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"charm.land/fantasy"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
)

// toolNameSep separates the server slug from the original tool
// name in prefixed tool names. Double underscore avoids collisions
// with tool names that may contain single underscores.
const toolNameSep = "__"

// connectTimeout bounds how long we wait for a single MCP server
// to start its transport and complete initialization. Servers that
// take longer are skipped so one slow server cannot block the
// entire chat startup.
const connectTimeout = 10 * time.Second

// ConnectAll connects to all configured MCP servers, discovers
// their tools, and returns them as fantasy.AgentTool values. It
// skips servers that fail to connect and logs warnings. The
// returned cleanup function must be called to close all
// connections.
func ConnectAll(
	ctx context.Context,
	logger slog.Logger,
	configs []database.MCPServerConfig,
	tokens []database.MCPServerUserToken,
) (tools []fantasy.AgentTool, cleanup func(), err error) {
	// Index tokens by server config ID so auth header
	// construction is O(1) per server.
	tokensByConfigID := make(
		map[string]database.MCPServerUserToken, len(tokens),
	)
	for _, tok := range tokens {
		tokensByConfigID[tok.MCPServerConfigID.String()] = tok
	}

	var clients []*client.Client

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}

		serverTools, mcpClient, connectErr := connectOne(
			ctx, logger, cfg, tokensByConfigID,
		)
		if connectErr != nil {
			logger.Warn(ctx,
				"skipping MCP server due to connection failure",
				slog.F("server_slug", cfg.Slug),
				slog.F("server_url", cfg.Url),
				slog.Error(connectErr),
			)
			continue
		}
		clients = append(clients, mcpClient)
		tools = append(tools, serverTools...)
	}

	cleanup = func() {
		for _, c := range clients {
			_ = c.Close()
		}
	}

	return tools, cleanup, nil
}

// connectOne establishes a connection to a single MCP server,
// discovers its tools, and wraps each one as an AgentTool with
// the server slug prefix applied.
func connectOne(
	ctx context.Context,
	logger slog.Logger,
	cfg database.MCPServerConfig,
	tokensByConfigID map[string]database.MCPServerUserToken,
) ([]fantasy.AgentTool, *client.Client, error) {
	headers := buildAuthHeaders(cfg, tokensByConfigID)

	tr, err := createTransport(cfg, headers)
	if err != nil {
		return nil, nil, xerrors.Errorf(
			"create transport: %w", err,
		)
	}

	mcpClient := client.NewClient(tr)

	connectCtx, cancel := context.WithTimeout(
		ctx, connectTimeout,
	)
	defer cancel()

	if err := mcpClient.Start(connectCtx); err != nil {
		_ = mcpClient.Close()
		return nil, nil, xerrors.Errorf(
			"start transport: %w", err,
		)
	}

	_, err = mcpClient.Initialize(
		connectCtx,
		mcp.InitializeRequest{
			Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo: mcp.Implementation{
					Name:    "coder",
					Version: buildinfo.Version(),
				},
			},
		},
	)
	if err != nil {
		// Best-effort close so we don't leak the transport.
		_ = mcpClient.Close()
		return nil, nil, xerrors.Errorf("initialize: %w", err)
	}

	toolsResult, err := mcpClient.ListTools(
		connectCtx, mcp.ListToolsRequest{},
	)
	if err != nil {
		_ = mcpClient.Close()
		return nil, nil, xerrors.Errorf("list tools: %w", err)
	}

	var tools []fantasy.AgentTool
	for _, mcpTool := range toolsResult.Tools {
		if !isToolAllowed(
			mcpTool.Name,
			cfg.ToolAllowList,
			cfg.ToolDenyList,
		) {
			logger.Debug(ctx, "skipping denied MCP tool",
				slog.F("server_slug", cfg.Slug),
				slog.F("tool_name", mcpTool.Name),
			)
			continue
		}

		tools = append(
			tools, newMCPTool(cfg.Slug, mcpTool, mcpClient),
		)
	}

	return tools, mcpClient, nil
}

// createTransport builds the appropriate mcp-go transport based
// on the server's configured transport type.
func createTransport(
	cfg database.MCPServerConfig,
	headers map[string]string,
) (transport.Interface, error) {
	switch cfg.Transport {
	case "sse":
		return transport.NewSSE(
			cfg.Url,
			transport.WithHeaders(headers),
		)
	case "streamable_http", "":
		// Default to streamable HTTP, the newer transport.
		return transport.NewStreamableHTTP(
			cfg.Url,
			transport.WithHTTPHeaders(headers),
		)
	default:
		return nil, xerrors.Errorf(
			"unsupported transport %q", cfg.Transport,
		)
	}
}

// buildAuthHeaders constructs HTTP headers for authenticating
// with the MCP server based on the configured auth type.
func buildAuthHeaders(
	cfg database.MCPServerConfig,
	tokensByConfigID map[string]database.MCPServerUserToken,
) map[string]string {
	headers := make(map[string]string)

	switch cfg.AuthType {
	case "oauth2":
		tok, ok := tokensByConfigID[cfg.ID.String()]
		if ok && tok.AccessToken != "" {
			tokenType := tok.TokenType
			if tokenType == "" {
				tokenType = "Bearer"
			}
			headers["Authorization"] =
				tokenType + " " + tok.AccessToken
		}
	case "api_key":
		if cfg.APIKeyHeader != "" && cfg.APIKeyValue != "" {
			headers[cfg.APIKeyHeader] = cfg.APIKeyValue
		}
	case "custom_headers":
		if cfg.CustomHeaders != "" {
			var custom map[string]string
			if err := json.Unmarshal(
				[]byte(cfg.CustomHeaders), &custom,
			); err == nil {
				for k, v := range custom {
					headers[k] = v
				}
			}
		}
	case "none", "":
		// No auth headers needed.
	}

	return headers
}

// isToolAllowed checks a tool name against the allow and deny
// lists. When the allow list is non-empty only tools in it are
// permitted. When the deny list is non-empty tools in it are
// rejected. Both lists use exact string matching against the
// original (non-prefixed) tool name.
func isToolAllowed(
	toolName string,
	allowList []string,
	denyList []string,
) bool {
	if len(allowList) > 0 {
		for _, allowed := range allowList {
			if allowed == toolName {
				return true
			}
		}
		// Allow list is set but the tool isn't in it.
		return false
	}

	for _, denied := range denyList {
		if denied == toolName {
			return false
		}
	}

	return true
}

// mcpToolWrapper adapts a single MCP tool into a
// fantasy.AgentTool. It stores the prefixed name for Info() but
// strips the prefix when forwarding calls to the remote server.
type mcpToolWrapper struct {
	prefixedName    string
	originalName    string
	description     string
	parameters      map[string]any
	required        []string
	client          *client.Client
	providerOptions fantasy.ProviderOptions
}

// newMCPTool creates an mcpToolWrapper from an mcp.Tool
// discovered on a remote server.
func newMCPTool(
	serverSlug string,
	tool mcp.Tool,
	mcpClient *client.Client,
) *mcpToolWrapper {
	return &mcpToolWrapper{
		prefixedName: serverSlug + toolNameSep + tool.Name,
		originalName: tool.Name,
		description:  tool.Description,
		parameters:   tool.InputSchema.Properties,
		required:     tool.InputSchema.Required,
		client:       mcpClient,
	}
}

func (t *mcpToolWrapper) Info() fantasy.ToolInfo {
	return fantasy.ToolInfo{
		Name:        t.prefixedName,
		Description: t.description,
		Parameters:  t.parameters,
		Required:    t.required,
		Parallel:    true,
	}
}

func (t *mcpToolWrapper) Run(
	ctx context.Context,
	params fantasy.ToolCall,
) (fantasy.ToolResponse, error) {
	var args map[string]any
	if params.Input != "" {
		if err := json.Unmarshal(
			[]byte(params.Input), &args,
		); err != nil {
			return fantasy.NewTextErrorResponse(
				"invalid JSON input: " + err.Error(),
			), nil
		}
	}

	result, err := t.client.CallTool(
		ctx,
		mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Name:      t.originalName,
				Arguments: args,
			},
		},
	)
	if err != nil {
		return fantasy.NewTextErrorResponse(err.Error()), nil
	}

	return convertCallResult(result), nil
}

func (t *mcpToolWrapper) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *mcpToolWrapper) SetProviderOptions(
	opts fantasy.ProviderOptions,
) {
	t.providerOptions = opts
}

// convertCallResult translates an MCP CallToolResult into a
// fantasy.ToolResponse. It favours text content; for image or
// audio data it returns the first binary item. When the result
// contains multiple text items they are concatenated with
// newlines so no content is silently dropped.
func convertCallResult(
	result *mcp.CallToolResult,
) fantasy.ToolResponse {
	if result == nil {
		return fantasy.NewTextResponse("")
	}

	var textParts []string
	for _, item := range result.Content {
		switch c := item.(type) {
		case mcp.TextContent:
			textParts = append(textParts, c.Text)
		case mcp.ImageContent:
			data, err := base64.StdEncoding.DecodeString(
				c.Data,
			)
			if err != nil {
				textParts = append(textParts,
					"[image decode error: "+err.Error()+"]",
				)
				continue
			}
			return fantasy.ToolResponse{
				Type:      "image",
				Data:      data,
				MediaType: c.MIMEType,
				IsError:   result.IsError,
			}
		case mcp.AudioContent:
			data, err := base64.StdEncoding.DecodeString(
				c.Data,
			)
			if err != nil {
				textParts = append(textParts,
					"[audio decode error: "+err.Error()+"]",
				)
				continue
			}
			return fantasy.ToolResponse{
				Type:      "media",
				Data:      data,
				MediaType: c.MIMEType,
				IsError:   result.IsError,
			}
		default:
			textParts = append(textParts,
				fmt.Sprintf("[unsupported content type: %T]", c),
			)
		}
	}

	resp := fantasy.NewTextResponse(
		strings.Join(textParts, "\n"),
	)
	resp.IsError = result.IsError
	return resp
}
