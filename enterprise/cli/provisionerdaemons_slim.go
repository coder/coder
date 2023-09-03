//go:build slim

package cli

import (
	"github.com/coder/coder/v2/cli/clibase"
)

func (r *RootCmd) provisionerDaemons() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "provisionerd",
		Short: "Manage provisioner daemons",
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			slimUnsupported(inv.Stderr, "coder provisionerd")
			return nil
		},
	}

	return cmd
}
