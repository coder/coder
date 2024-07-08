//go:build slim

package cli

import (
	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/serpent"
)

func (r *RootCmd) provisionerDaemons() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "provisionerd",
		Short: "Manage provisioner daemons",
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *serpent.Invocation) error {
			agplcli.SlimUnsupported(inv.Stderr, "provisionerd")
			return nil
		},
	}

	return cmd
}
