package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

// Option configures a Server at construction time.
type Option func(*Server)

// WithMetrics attaches a Metrics sink to the Server. Nil is accepted and
// disables metric recording.
func WithMetrics(m *Metrics) Option {
	return func(s *Server) { s.metrics = m }
}

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
	mcpServer *server.MCPServer

	// streamableServer handles HTTP transport
	streamableServer *server.StreamableHTTPServer

	metrics *Metrics
}

// NewServer creates a new MCP HTTP server.
func NewServer(logger slog.Logger, opts ...Option) (*Server, error) {
	// Create the core MCP server
	mcpSrv := server.NewMCPServer(
		MCPServerName,
		buildinfo.Version(),
		server.WithInstructions(MCPServerInstructions),
	)

	// Create logger adapter for mcp-go
	mcpLogger := &mcpLoggerAdapter{logger: logger}

	// Create streamable HTTP server with configuration
	streamableServer := server.NewStreamableHTTPServer(mcpSrv,
		server.WithHeartbeatInterval(30*time.Second),
		server.WithLogger(mcpLogger),
	)

	s := &Server{
		Logger:           logger,
		mcpServer:        mcpSrv,
		streamableServer: streamableServer,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// ServeHTTP implements http.Handler interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.metrics.sessionInc()
	defer s.metrics.sessionDec()
	s.streamableServer.ServeHTTP(w, r)
}

// toolDepsOpts returns the toolsdk.NewDeps option set derived from this
// Server's configuration.
func (s *Server) toolDepsOpts() []func(*toolsdk.Deps) {
	var opts []func(*toolsdk.Deps)
	if obs := s.metrics.AgentDialObserver(); obs != nil {
		opts = append(opts, toolsdk.WithAgentConnObserver(obs))
	}
	return opts
}

// Register all available MCP tools with the server excluding:
// - ReportTask - which requires dependencies not available in the remote MCP context
// - ChatGPT search and fetch tools, which are redundant with the standard tools.
func (s *Server) RegisterTools(client *codersdk.Client) error {
	if client == nil {
		return xerrors.New("client cannot be nil: MCP HTTP server requires authenticated client")
	}

	// Create tool dependencies
	toolDeps, err := toolsdk.NewDeps(client, s.toolDepsOpts()...)
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

		s.mcpServer.AddTools(s.mcpFromSDK(tool, toolDeps))
	}
	return nil
}

// ChatGPT tools are the search and fetch tools as defined in https://platform.openai.com/docs/mcp.
// We do not expose any extra ones because ChatGPT has an undocumented "Safety Scan" feature.
// In my experiments, if I included extra tools in the MCP server, ChatGPT would often - but not always -
// refuse to add Coder as a connector.
func (s *Server) RegisterChatGPTTools(client *codersdk.Client) error {
	if client == nil {
		return xerrors.New("client cannot be nil: MCP HTTP server requires authenticated client")
	}

	// Create tool dependencies
	toolDeps, err := toolsdk.NewDeps(client, s.toolDepsOpts()...)
	if err != nil {
		return xerrors.Errorf("failed to initialize tool dependencies: %w", err)
	}

	for _, tool := range toolsdk.All {
		if tool.Name != toolsdk.ToolNameChatGPTSearch && tool.Name != toolsdk.ToolNameChatGPTFetch {
			continue
		}

		s.mcpServer.AddTools(s.mcpFromSDK(tool, toolDeps))
	}
	return nil
}

// mcpFromSDK adapts a toolsdk.Tool to go-mcp's server.ServerTool. The
// returned handler records duration and outcome to s.metrics and emits a
// structured log line for every invocation.
func (s *Server) mcpFromSDK(sdkTool toolsdk.GenericTool, tb toolsdk.Deps) server.ServerTool {
	if sdkTool.Schema.Properties == nil {
		panic("developer error: schema properties cannot be nil")
	}

	return server.ServerTool{
		Tool: mcp.Tool{
			Name:        sdkTool.Name,
			Description: sdkTool.Description,
			InputSchema: mcp.ToolInputSchema{
				Type:       "object",
				Properties: sdkTool.Schema.Properties,
				Required:   sdkTool.Schema.Required,
			},
			Annotations: mcp.ToolAnnotation{
				ReadOnlyHint:    mcp.ToBoolPtr(sdkTool.MCPAnnotations.ReadOnlyHint),
				DestructiveHint: mcp.ToBoolPtr(sdkTool.MCPAnnotations.DestructiveHint),
				IdempotentHint:  mcp.ToBoolPtr(sdkTool.MCPAnnotations.IdempotentHint),
				OpenWorldHint:   mcp.ToBoolPtr(sdkTool.MCPAnnotations.OpenWorldHint),
			},
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(request.Params.Arguments); err != nil {
				return nil, xerrors.Errorf("failed to encode request arguments: %w", err)
			}

			start := time.Now()
			result, err := sdkTool.Handler(ctx, tb, buf.Bytes())
			elapsed := time.Since(start)

			outcome := "success"
			if err != nil {
				outcome = "error"
			}
			s.metrics.observeTool(sdkTool.Name, outcome, elapsed.Seconds())

			fields := []slog.Field{
				slog.F("tool", sdkTool.Name),
				slog.F("outcome", outcome),
				slog.F("duration_ms", elapsed.Milliseconds()),
				slog.F("arg_bytes", buf.Len()),
				slog.F("result_bytes", len(result)),
			}
			if r, ok := RequestorFromContext(ctx); ok {
				fields = append(fields,
					slog.F("requestor_id", r.UserID),
					slog.F("requestor_name", r.Username),
					slog.F("requestor_email", r.Email),
					slog.F("api_key_id", r.APIKeyID),
					slog.F("request_id", r.RequestID),
					slog.F("user_agent", r.UserAgent),
				)
			}
			if err != nil {
				s.Logger.Warn(ctx, "mcp tool call failed", append(fields, slog.Error(err))...)
				return nil, err
			}
			s.Logger.Debug(ctx, "mcp tool call", fields...)

			return &mcp.CallToolResult{
				Content: []mcp.Content{
					mcp.NewTextContent(string(result)),
				},
			}, nil
		},
	}
}

// mcpLoggerAdapter adapts slog.Logger to the mcp-go util.Logger interface
type mcpLoggerAdapter struct {
	logger slog.Logger
}

func (l *mcpLoggerAdapter) Infof(format string, v ...any) {
	l.logger.Info(context.Background(), fmt.Sprintf(format, v...))
}

func (l *mcpLoggerAdapter) Errorf(format string, v ...any) {
	l.logger.Error(context.Background(), fmt.Sprintf(format, v...))
}
