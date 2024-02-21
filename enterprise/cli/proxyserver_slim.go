//go:build slim

package cli

import (
	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/serpent"
)

func (r *RootCmd) proxyServer() *serpent.Cmd {
	root := &serpent.Cmd{
		Use:     "server",
		Short:   "Start a workspace proxy server",
		Aliases: []string{},
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *serpent.Invocation) error {
			agplcli.SlimUnsupported(inv.Stderr, "workspace-proxy server")
			return nil
		},
	}

	return root
}
