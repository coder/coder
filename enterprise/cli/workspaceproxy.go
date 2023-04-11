package cli

import (
	"context"
	"fmt"

	"github.com/coder/coder/cli"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) workspaceProxy() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "workspace-proxy",
		Short:   "Manage workspace proxies",
		Aliases: []string{"proxy"},
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.proxyServer(),
		},
	}

	return cmd
}

func (r *RootCmd) proxyServer() *clibase.Cmd {
	var (
		// TODO: Remove options that we do not need
		cfg  = new(codersdk.DeploymentValues)
		opts = cfg.Options()
	)
	var _ = opts

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "server",
		Short: "Start a workspace proxy server",
		Middleware: clibase.Chain(
			cli.WriteConfigMW(cfg),
			cli.PrintDeprecatedOptions(),
			clibase.RequireNArgs(0),
			// We need a client to connect with the primary coderd instance.
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			// Main command context for managing cancellation of running
			// services.
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()
			var _ = ctx

			_, _ = fmt.Fprintf(inv.Stdout, "Not yet implemented\n")
			return nil
		},
	}

	return cmd
}
