package chattool

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"charm.land/fantasy"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/database"
)

// mcpToolNameSeparator is the double-underscore separator used to
// namespace MCP tool names: mcp__{server_slug}__{tool_name}.
const mcpToolNameSeparator = "__"

// MCPServerClient wraps an mcp-go client and adapts its tools
// to fantasy.AgentTool instances.
type MCPServerClient struct {
	client      *mcpclient.Client
	server      database.ChatMCPServer
	logger      slog.Logger
	mcpTools    []mcp.Tool
	allowRegex  *regexp.Regexp
	denyRegex   *regexp.Regexp
}

// DiscoverMCPServerTools connects to a remote MCP server,
// discovers its tools, applies allow/deny filters, and returns
// them as fantasy.AgentTool instances. The returned cleanup
// function closes the underlying MCP client and must be called
// when the tools are no longer needed.
func DiscoverMCPServerTools(
	ctx context.Context,
	server database.ChatMCPServer,
	logger slog.Logger,
) ([]fantasy.AgentTool, func(), error) {
	opts := []transport.StreamableHTTPCOption{}

	// If the server uses header-based authentication, parse the
	// headers and attach them to every outgoing request.
	if server.AuthType == "header" && server.AuthHeaders != "" {
		var headers map[string]string
		if err := json.Unmarshal([]byte(server.AuthHeaders), &headers); err != nil {
			return nil, nil, xerrors.Errorf("parse auth headers for MCP server %q: %w", server.Slug, err)
		}
		if len(headers) > 0 {
			opts = append(opts, transport.WithHTTPHeaders(headers))
		}
	}

	c, err := mcpclient.NewStreamableHttpClient(server.Url, opts...)
	if err != nil {
		return nil, nil, xerrors.Errorf("create MCP client for %q: %w", server.Slug, err)
	}

	if err := c.Start(ctx); err != nil {
		// Best-effort close if Start fails.
		_ = c.Close()
		return nil, nil, xerrors.Errorf("start MCP client for %q: %w", server.Slug, err)
	}

	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{
		Name:    "coder-chat",
		Version: buildinfo.Version(),
	}

	if _, err := c.Initialize(ctx, initReq); err != nil {
		_ = c.Close()
		return nil, nil, xerrors.Errorf("initialize MCP server %q: %w", server.Slug, err)
	}

	toolsResult, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		_ = c.Close()
		return nil, nil, xerrors.Errorf("list tools from MCP server %q: %w", server.Slug, err)
	}

	sc := &MCPServerClient{
		client: c,
		server: server,
		logger: logger,
	}

	// Compile optional allow/deny regexes.
	if server.ToolAllowRegex != "" {
		sc.allowRegex, err = regexp.Compile(server.ToolAllowRegex)
		if err != nil {
			_ = c.Close()
			return nil, nil, xerrors.Errorf(
				"compile tool_allow_regex %q for MCP server %q: %w",
				server.ToolAllowRegex, server.Slug, err,
			)
		}
	}
	if server.ToolDenyRegex != "" {
		sc.denyRegex, err = regexp.Compile(server.ToolDenyRegex)
		if err != nil {
			_ = c.Close()
			return nil, nil, xerrors.Errorf(
				"compile tool_deny_regex %q for MCP server %q: %w",
				server.ToolDenyRegex, server.Slug, err,
			)
		}
	}

	// Filter and adapt the discovered tools.
	var tools []fantasy.AgentTool
	for _, mcpTool := range toolsResult.Tools {
		if !sc.toolAllowed(mcpTool.Name) {
			logger.Debug(ctx, "skipping MCP tool filtered by regex",
				slog.F("server", server.Slug),
				slog.F("tool", mcpTool.Name),
			)
			continue
		}
		tools = append(tools, sc.adaptTool(mcpTool))
	}

	cleanup := func() {
		if closeErr := c.Close(); closeErr != nil {
			logger.Warn(context.Background(), "failed to close MCP client",
				slog.F("server", server.Slug),
				slog.Error(closeErr),
			)
		}
	}

	return tools, cleanup, nil
}

// toolAllowed returns true if the tool name passes both the
// allow and deny regex filters.
func (sc *MCPServerClient) toolAllowed(name string) bool {
	if sc.allowRegex != nil && !sc.allowRegex.MatchString(name) {
		return false
	}
	if sc.denyRegex != nil && sc.denyRegex.MatchString(name) {
		return false
	}
	return true
}

// adaptTool converts an MCP tool definition into a
// fantasy.AgentTool that proxies calls through the MCP client.
func (sc *MCPServerClient) adaptTool(tool mcp.Tool) fantasy.AgentTool {
	qualifiedName := fmt.Sprintf(
		"mcp%s%s%s%s",
		mcpToolNameSeparator,
		sc.server.Slug,
		mcpToolNameSeparator,
		tool.Name,
	)
	description := fmt.Sprintf(
		"[MCP: %s] %s",
		sc.server.DisplayName,
		tool.Description,
	)
	parameters := mcpInputSchemaToParameters(tool.InputSchema)
	required := tool.InputSchema.Required

	// Capture a copy of the tool name for the closure.
	mcpToolName := tool.Name
	client := sc.client
	logger := sc.logger
	serverSlug := sc.server.Slug

	return &mcpAdaptedTool{
		info: fantasy.ToolInfo{
			Name:        qualifiedName,
			Description: description,
			Parameters:  parameters,
			Required:    required,
		},
		run: func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
			var args map[string]any
			if call.Input != "" {
				if err := json.Unmarshal([]byte(call.Input), &args); err != nil {
					return fantasy.NewTextErrorResponse(
						fmt.Sprintf("invalid tool arguments: %s", err),
					), nil
				}
			}

			callReq := mcp.CallToolRequest{}
			callReq.Params.Name = mcpToolName
			callReq.Params.Arguments = args

			result, err := client.CallTool(ctx, callReq)
			if err != nil {
				logger.Warn(ctx, "MCP tool call failed",
					slog.F("server", serverSlug),
					slog.F("tool", mcpToolName),
					slog.Error(err),
				)
				return fantasy.NewTextErrorResponse(
					fmt.Sprintf("MCP tool call failed: %s", err),
				), nil
			}

			return mcpResultToToolResponse(result), nil
		},
	}
}

// mcpInputSchemaToParameters converts an MCP ToolInputSchema to
// the map[string]any parameter format expected by fantasy.ToolInfo.
func mcpInputSchemaToParameters(schema mcp.ToolInputSchema) map[string]any {
	if schema.Properties == nil {
		return make(map[string]any)
	}
	// The MCP input schema properties are already
	// map[string]any, matching the fantasy parameter format.
	return schema.Properties
}

// mcpResultToToolResponse converts an MCP CallToolResult into a
// fantasy.ToolResponse. It handles text and image content parts.
func mcpResultToToolResponse(result *mcp.CallToolResult) fantasy.ToolResponse {
	if result == nil {
		return fantasy.NewTextResponse("")
	}

	var textParts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case mcp.TextContent:
			textParts = append(textParts, c.Text)
		case mcp.ImageContent:
			// Return the first image encountered directly.
			data, err := base64.StdEncoding.DecodeString(c.Data)
			if err != nil {
				textParts = append(textParts,
					fmt.Sprintf("[image decode error: %s]", err),
				)
				continue
			}
			return fantasy.NewImageResponse(data, c.MIMEType)
		}
	}

	text := strings.Join(textParts, "\n")

	if result.IsError {
		return fantasy.NewTextErrorResponse(text)
	}
	return fantasy.NewTextResponse(text)
}

// mcpAdaptedTool implements fantasy.AgentTool for an MCP tool
// proxied through an MCP client.
type mcpAdaptedTool struct {
	info            fantasy.ToolInfo
	run             func(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error)
	providerOptions fantasy.ProviderOptions
}

func (t *mcpAdaptedTool) Info() fantasy.ToolInfo {
	return t.info
}

func (t *mcpAdaptedTool) Run(ctx context.Context, call fantasy.ToolCall) (fantasy.ToolResponse, error) {
	return t.run(ctx, call)
}

func (t *mcpAdaptedTool) ProviderOptions() fantasy.ProviderOptions {
	return t.providerOptions
}

func (t *mcpAdaptedTool) SetProviderOptions(opts fantasy.ProviderOptions) {
	t.providerOptions = opts
}
