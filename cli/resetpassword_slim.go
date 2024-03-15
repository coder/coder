//go:build slim

package cli

import "github.com/coder/serpent"

func (*RootCmd) resetPassword() *serpent.Command {
	root := &serpent.Command{
		Use:   "reset-password <username>",
		Short: "Directly connect to the database to reset a user's password",
		// We accept RawArgs so all commands and flags are accepted.
		RawArgs: true,
		Hidden:  true,
		Handler: func(inv *serpent.Invocation) error {
			SlimUnsupported(inv.Stderr, "reset-password")
			return nil
		},
	}

	return root
}
