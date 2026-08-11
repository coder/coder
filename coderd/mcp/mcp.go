package mcp

import (
	"context"
	stdslog "log/slog"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

const (
	// MCPServerName is the name used for the MCP server.
	MCPServerName = "Coder"
	// MCPServerInstructions is the instructions text for the MCP server.
	MCPServerInstructions = "Coder MCP Server providing workspace and template management tools"

	// Used in tests and aibridge.
	MCPEndpoint = "/api/experimental/mcp/http"
)

// Server represents an MCP HTTP server instance
type Server struct {
	Logger slog.Logger

	// mcpServer is the underlying MCP server
	mcpServer *mcp.Server

	handler http.Handler
}

// NewServer creates a new MCP HTTP server
func NewServer(logger slog.Logger) (*Server, error) {
	mcpSrv := mcp.NewServer(&mcp.Implementation{
		Name:    MCPServerName,
		Version: buildinfo.Version(),
	}, &mcp.ServerOptions{
		Instructions: MCPServerInstructions,
		Logger:       stdslog.New(&slogHandler{logger: logger}),
	})

	// Stateless mode runs each request as its own short-lived session,
	// omits Mcp-Session-Id, and answers GET and DELETE with 405, all
	// permitted by the Streamable HTTP transport spec.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return mcpSrv
	}, &mcp.StreamableHTTPOptions{
		Stateless: true,
		// Use application/json instead of SSE framing because Coder
		// tools emit no notifications and do not need a stream.
		JSONResponse: true,
		// coderd often listens on loopback behind a reverse proxy,
		// which trips the SDK's localhost DNS-rebinding check (loopback
		// local address with a public Host header). The endpoint's
		// bearer authentication is the relevant access control.
		DisableLocalhostProtection: true,
	})

	return &Server{
		Logger:    logger,
		mcpServer: mcpSrv,
		handler:   handler,
	}, nil
}

// ServeHTTP implements http.Handler interface
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

// Register all available MCP tools with the server excluding:
// - ReportTask - which requires dependencies not available in the remote MCP context
// - ChatGPT search and fetch tools, which are redundant with the standard tools.
func (s *Server) RegisterTools(client *codersdk.Client, opts ...func(*toolsdk.Deps)) error {
	if client == nil {
		return xerrors.New("client cannot be nil: MCP HTTP server requires authenticated client")
	}

	// Create tool dependencies
	toolDeps, err := toolsdk.NewDeps(client, opts...)
	if err != nil {
		return xerrors.Errorf("failed to initialize tool dependencies: %w", err)
	}

	for _, tool := range toolsdk.All {
		// the ReportTask tool requires dependencies not available in the remote MCP context
		// the ChatGPT search and fetch tools are redundant with the standard tools.
		if tool.Name == toolsdk.ToolNameReportTask ||
			tool.Name == toolsdk.ToolNameChatGPTSearch || tool.Name == toolsdk.ToolNameChatGPTFetch {
			continue
		}

		RegisterSDKTool(s.mcpServer, tool, toolDeps)
	}
	return nil
}

// RegisterPrompts registers all MCP prompt templates with the server.
func (s *Server) RegisterPrompts() {
	for _, prompt := range toolsdk.AllPrompts {
		RegisterSDKPrompt(s.mcpServer, prompt)
	}
}

// ChatGPT tools are the search and fetch tools as defined in https://platform.openai.com/docs/mcp.
// We do not expose any extra ones because ChatGPT has an undocumented "Safety Scan" feature.
// In my experiments, if I included extra tools in the MCP server, ChatGPT would often - but not always -
// refuse to add Coder as a connector.
func (s *Server) RegisterChatGPTTools(client *codersdk.Client, opts ...func(*toolsdk.Deps)) error {
	if client == nil {
		return xerrors.New("client cannot be nil: MCP HTTP server requires authenticated client")
	}

	// Create tool dependencies
	toolDeps, err := toolsdk.NewDeps(client, opts...)
	if err != nil {
		return xerrors.Errorf("failed to initialize tool dependencies: %w", err)
	}

	for _, tool := range toolsdk.All {
		if tool.Name != toolsdk.ToolNameChatGPTSearch && tool.Name != toolsdk.ToolNameChatGPTFetch {
			continue
		}

		RegisterSDKTool(s.mcpServer, tool, toolDeps)
	}
	return nil
}

// RegisterSDKTool registers a [toolsdk.GenericTool] with an MCP server.
func RegisterSDKTool(srv *mcp.Server, sdkTool toolsdk.GenericTool, tb toolsdk.Deps) {
	if sdkTool.Schema.Properties == nil {
		panic("developer error: schema properties cannot be nil")
	}

	inputSchema := map[string]any{
		"type":       "object",
		"properties": sdkTool.Schema.Properties,
	}
	if len(sdkTool.Schema.Required) > 0 {
		inputSchema["required"] = sdkTool.Schema.Required
	}

	srv.AddTool(&mcp.Tool{
		Name:        sdkTool.Name,
		Description: sdkTool.Description,
		InputSchema: inputSchema,
		// Set pointer-valued hints even when false so every hint
		// remains explicit on the wire.
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    sdkTool.MCPAnnotations.ReadOnlyHint,
			DestructiveHint: ptr.Ref(sdkTool.MCPAnnotations.DestructiveHint),
			IdempotentHint:  sdkTool.MCPAnnotations.IdempotentHint,
			OpenWorldHint:   ptr.Ref(sdkTool.MCPAnnotations.OpenWorldHint),
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := sdkTool.Handler(ctx, tb, req.Params.Arguments)
		if err != nil {
			return nil, err
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: string(result)},
			},
		}, nil
	})
}

// RegisterSDKPrompt registers a [toolsdk.Prompt] with an MCP server.
func RegisterSDKPrompt(srv *mcp.Server, sdkPrompt toolsdk.Prompt) {
	args := make([]*mcp.PromptArgument, 0, len(sdkPrompt.Arguments))
	for _, arg := range sdkPrompt.Arguments {
		args = append(args, &mcp.PromptArgument{
			Name:        arg.Name,
			Description: arg.Description,
			Required:    arg.Required,
		})
	}
	srv.AddPrompt(&mcp.Prompt{
		Name:        sdkPrompt.Name,
		Description: sdkPrompt.Description,
		Arguments:   args,
	}, func(_ context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		text, err := sdkPrompt.Render(req.Params.Arguments)
		if err != nil {
			return nil, err
		}
		return &mcp.GetPromptResult{
			Description: sdkPrompt.Description,
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: text}},
			},
		}, nil
	})
}

type slogHandler struct {
	logger slog.Logger
}

// The SDK logs several INFO lines per stateless request (session
// connect/disconnect), so only warnings and errors are forwarded.
func (*slogHandler) Enabled(_ context.Context, level stdslog.Level) bool {
	return level >= stdslog.LevelWarn
}

func (h *slogHandler) Handle(ctx context.Context, record stdslog.Record) error {
	fields := make([]slog.Field, 0, record.NumAttrs())
	record.Attrs(func(attr stdslog.Attr) bool {
		fields = append(fields, slog.F(attr.Key, attr.Value.Any()))
		return true
	})
	switch {
	case record.Level >= stdslog.LevelError:
		h.logger.Error(ctx, record.Message, fields...)
	case record.Level >= stdslog.LevelWarn:
		h.logger.Warn(ctx, record.Message, fields...)
	default:
		h.logger.Info(ctx, record.Message, fields...)
	}
	return nil
}

func (h *slogHandler) WithAttrs(attrs []stdslog.Attr) stdslog.Handler {
	fields := make([]slog.Field, 0, len(attrs))
	for _, attr := range attrs {
		fields = append(fields, slog.F(attr.Key, attr.Value.Any()))
	}
	return &slogHandler{logger: h.logger.With(fields...)}
}

func (h *slogHandler) WithGroup(string) stdslog.Handler {
	return h
}
