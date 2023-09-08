//go:build slim

package cli

import (
	"github.com/coder/coder/v2/cli/clibase"
)

func (r *RootCmd) Server(_ func()) *clibase.Cmd {
	root := &clibase.Cmd{
		Use:   "server",
		Short: "Start a Coder server",
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			SlimUnsupported(inv.Stderr, "server")
			return nil
		},
	}

	return root
}
