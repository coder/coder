package codermcp

import (
	"context"
	"io"
	"log"
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
	in                  io.Reader
	out                 io.Writer
	instructions        string
	logger              *slog.Logger
	allowedTools        []string
	allowedExecCommands []string
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

// WithStdin sets the input reader for the MCP server.
func WithStdin(in io.Reader) Option {
	return func(o *mcpOptions) {
		o.in = in
	}
}

// WithStdout sets the output writer for the MCP server.
func WithStdout(out io.Writer) Option {
	return func(o *mcpOptions) {
		o.out = out
	}
}

// WithAllowedTools sets the allowed tools for the MCP server.
func WithAllowedTools(tools []string) Option {
	return func(o *mcpOptions) {
		o.allowedTools = tools
	}
}

// WithAllowedExecCommands sets the allowed commands for workspace execution.
func WithAllowedExecCommands(commands []string) Option {
	return func(o *mcpOptions) {
		o.allowedExecCommands = commands
	}
}

// New creates a new MCP server with the given client and options.
func New(ctx context.Context, client *codersdk.Client, opts ...Option) io.Closer {
	options := &mcpOptions{
		in:           os.Stdin,
		instructions: ``,
		logger:       ptr.Ref(slog.Make(sloghuman.Sink(os.Stdout))),
		out:          os.Stdout,
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
		Client:              client,
		Logger:              &logger,
		AllowedExecCommands: options.allowedExecCommands,
	})

	srv := server.NewStdioServer(mcpSrv)
	srv.SetErrorLogger(log.New(options.out, "", log.LstdFlags))
	done := make(chan error)
	go func() {
		defer close(done)
		srvErr := srv.Listen(ctx, options.in, options.out)
		done <- srvErr
	}()

	return closeFunc(func() error {
		return <-done
	})
}

type closeFunc func() error

func (f closeFunc) Close() error {
	return f()
}

var _ io.Closer = closeFunc(nil)
