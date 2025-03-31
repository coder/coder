package codermcp

import (
	"io"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/codersdk"
	mcptools "github.com/coder/coder/v2/mcp/tools"
)

type mcpOptions struct {
	instructions string
	logger       *slog.Logger
	allowedTools []string
}

// Option is a function that configures the MCP server.
type Option func(*mcpOptions)

// WithInstructions sets the instructions for the MCP server.
func WithInstructions(instructions string) Option {
	return func(o *mcpOptions) {
		o.instructions = instructions
	}
}

// WithLogger sets the logger for the MCP server.
func WithLogger(logger *slog.Logger) Option {
	return func(o *mcpOptions) {
		o.logger = logger
	}
}

// WithAllowedTools sets the allowed tools for the MCP server.
func WithAllowedTools(tools []string) Option {
	return func(o *mcpOptions) {
		o.allowedTools = tools
	}
}

// NewStdio creates a new MCP stdio server with the given client and options.
// It is the responsibility of the caller to start and stop the server.
func NewStdio(client *codersdk.Client, opts ...Option) *server.StdioServer {
	options := &mcpOptions{
		instructions: ``,
		logger:       ptr.Ref(slog.Make(sloghuman.Sink(os.Stdout))),
	}
	for _, opt := range opts {
		opt(options)
	}

	mcpSrv := server.NewMCPServer(
		"Coder Agent",
		buildinfo.Version(),
		server.WithInstructions(options.instructions),
	)

	logger := slog.Make(sloghuman.Sink(os.Stdout))

	// Register tools based on the allowed list (if specified)
	reg := mcptools.AllTools()
	if len(options.allowedTools) > 0 {
		reg = reg.WithOnlyAllowed(options.allowedTools...)
	}
	reg.Register(mcpSrv, mcptools.ToolDeps{
		Client: client,
		Logger: &logger,
	})

	srv := server.NewStdioServer(mcpSrv)
	return srv
}

type closeFunc func() error

func (f closeFunc) Close() error {
	return f()
}

var _ io.Closer = closeFunc(nil)
