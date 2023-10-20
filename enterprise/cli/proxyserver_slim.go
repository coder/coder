//go:build slim

package cli

import (
	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
)

func (r *RootCmd) proxyServer() *clibase.Cmd {
	root := &clibase.Cmd{
		Use:     "server",
		Short:   "Start a workspace proxy server",
		Aliases: []string{},
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			agplcli.SlimUnsupported(inv.Stderr, "workspace-proxy server")
			return nil
		},
	}

	return root
}
