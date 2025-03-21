package cli

import (
	"context"
	"errors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	codermcp "github.com/coder/coder/v2/mcp"
	"github.com/coder/serpent"
)

func (r *RootCmd) mcpCommand() *serpent.Command {
	var (
		client              = new(codersdk.Client)
		instructions        string
		allowedTools        []string
		allowedExecCommands []string
	)
	return &serpent.Command{
		Use: "mcp",
		Handler: func(inv *serpent.Invocation) error {
			return mcpHandler(inv, client, instructions, allowedTools, allowedExecCommands)
		},
		Short: "Start an MCP server that can be used to interact with a Coder depoyment.",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Options: []serpent.Option{
			{
				Name:        "instructions",
				Description: "The instructions to pass to the MCP server.",
				Flag:        "instructions",
				Value:       serpent.StringOf(&instructions),
			},
			{
				Name:        "allowed-tools",
				Description: "Comma-separated list of allowed tools. If not specified, all tools are allowed.",
				Flag:        "allowed-tools",
				Value:       serpent.StringArrayOf(&allowedTools),
			},
			{
				Name:        "allowed-exec-commands",
				Description: "Comma-separated list of allowed commands for workspace execution. If not specified, all commands are allowed.",
				Flag:        "allowed-exec-commands",
				Value:       serpent.StringArrayOf(&allowedExecCommands),
			},
		},
	}
}

func mcpHandler(inv *serpent.Invocation, client *codersdk.Client, instructions string, allowedTools []string, allowedExecCommands []string) error {
	ctx, cancel := context.WithCancel(inv.Context())
	defer cancel()

	logger := slog.Make(sloghuman.Sink(inv.Stdout))

	me, err := client.User(ctx, codersdk.Me)
	if err != nil {
		cliui.Errorf(inv.Stderr, "Failed to log in to the Coder deployment.")
		cliui.Errorf(inv.Stderr, "Please check your URL and credentials.")
		cliui.Errorf(inv.Stderr, "Tip: Run `coder whoami` to check your credentials.")
		return err
	}
	cliui.Infof(inv.Stderr, "Starting MCP server")
	cliui.Infof(inv.Stderr, "User          : %s", me.Username)
	cliui.Infof(inv.Stderr, "URL           : %s", client.URL)
	cliui.Infof(inv.Stderr, "Instructions  : %q", instructions)
	if len(allowedTools) > 0 {
		cliui.Infof(inv.Stderr, "Allowed Tools : %v", allowedTools)
	}
	if len(allowedExecCommands) > 0 {
		cliui.Infof(inv.Stderr, "Allowed Exec Commands : %v", allowedExecCommands)
	}
	cliui.Infof(inv.Stderr, "Press Ctrl+C to stop the server")

	// Capture the original stdin, stdout, and stderr.
	invStdin := inv.Stdin
	invStdout := inv.Stdout
	invStderr := inv.Stderr
	defer func() {
		inv.Stdin = invStdin
		inv.Stdout = invStdout
		inv.Stderr = invStderr
	}()

	options := []codermcp.Option{
		codermcp.WithInstructions(instructions),
		codermcp.WithLogger(&logger),
		codermcp.WithStdin(invStdin),
		codermcp.WithStdout(invStdout),
	}

	// Add allowed tools option if specified
	if len(allowedTools) > 0 {
		options = append(options, codermcp.WithAllowedTools(allowedTools))
	}

	// Add allowed exec commands option if specified
	if len(allowedExecCommands) > 0 {
		options = append(options, codermcp.WithAllowedExecCommands(allowedExecCommands))
	}

	closer := codermcp.New(ctx, client, options...)

	<-ctx.Done()
	if err := closer.Close(); err != nil {
		if !errors.Is(err, context.Canceled) {
			cliui.Errorf(inv.Stderr, "Failed to stop the MCP server: %s", err)
			return err
		}
	}
	return nil
}
