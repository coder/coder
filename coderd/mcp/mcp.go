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

	"cdr.dev/slog"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/toolsdk"
)

const (
	// MCPServerName is the name used for the MCP server.
	MCPServerName = "Coder"
	// MCPServerInstructions is the instructions text for the MCP server.
	MCPServerInstructions = "Coder MCP Server providing workspace and template management tools"
)

// Server represents an MCP HTTP server instance
type Server struct {
	Logger slog.Logger

	// mcpServer is the underlying MCP server
	mcpServer *server.MCPServer

	// streamableServer handles HTTP transport
	streamableServer *server.StreamableHTTPServer
}

// NewServer creates a new MCP HTTP server
func NewServer(logger slog.Logger) (*Server, error) {
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

	return &Server{
		Logger:           logger,
		mcpServer:        mcpSrv,
		streamableServer: streamableServer,
	}, nil
}

// ServeHTTP implements http.Handler interface
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.streamableServer.ServeHTTP(w, r)
}

// RegisterTools registers all available MCP tools with the server
func (s *Server) RegisterTools(client *codersdk.Client) error {
	if client == nil {
		return xerrors.New("client cannot be nil: MCP HTTP server requires authenticated client")
	}

	// Create tool dependencies
	toolDeps, err := toolsdk.NewDeps(client)
	if err != nil {
		return xerrors.Errorf("failed to initialize tool dependencies: %w", err)
	}

	// Register all available tools
	for _, tool := range toolsdk.All {
		s.mcpServer.AddTools(mcpFromSDK(tool, toolDeps))
	}

	return nil
}

// mcpFromSDK adapts a toolsdk.Tool to go-mcp's server.ServerTool
func mcpFromSDK(sdkTool toolsdk.GenericTool, tb toolsdk.Deps) server.ServerTool {
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
		},
		Handler: func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			var buf bytes.Buffer
			if err := json.NewEncoder(&buf).Encode(request.Params.Arguments); err != nil {
				return nil, xerrors.Errorf("failed to encode request arguments: %w", err)
			}
			result, err := sdkTool.Handler(ctx, tb, buf.Bytes())
			if err != nil {
				return nil, err
			}
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
