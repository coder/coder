package agentmcp

import (
	"context"
	"io"
	"log"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/agent/agentmcp/mcptools"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/coderd/util/ptr"
)

type mcpOptions struct {
	in           io.Reader
	out          io.Writer
	instructions string
	logger       *slog.Logger
}

type Option func(*mcpOptions)

func WithInstructions(instructions string) Option {
	return func(o *mcpOptions) {
		o.instructions = instructions
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(o *mcpOptions) {
		o.logger = logger
	}
}

func WithStdin(in io.Reader) Option {
	return func(o *mcpOptions) {
		o.in = in
	}
}

func WithStdout(out io.Writer) Option {
	return func(o *mcpOptions) {
		o.out = out
	}
}

func New(ctx context.Context, opts ...Option) io.Closer {
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

	mcptools.RegisterCoderReportTask(mcpSrv, logger)
	mcptools.RegisterCoderWhoami(mcpSrv)
	mcptools.RegisterCoderListWorkspaces(mcpSrv)
	mcptools.RegisterCoderWorkspaceExec(mcpSrv)

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
