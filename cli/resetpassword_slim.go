//go:build slim

package cli

import (
	"github.com/coder/coder/v2/cli/clibase"
)

func (*RootCmd) resetPassword() *clibase.Cmd {
	root := &clibase.Cmd{
		Use:   "reset-password <username>",
		Short: "Directly connect to the database to reset a user's password",
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *clibase.Invocation) error {
			SlimUnsupported(inv.Stderr, "reset-password")
			return nil
		},
	}

	return root
}
