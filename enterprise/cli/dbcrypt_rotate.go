//go:build !slim

package cli

import (
	"github.com/coder/coder/v2/cli/clibase"
)

func (r *RootCmd) dbcryptRotate() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "dbcrypt-rotate",
		Short: "Rotate database encryption keys",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
		),
		Handler: func(inv *clibase.Invocation) error {
			// TODO: implement
			return nil
		},
	}
	return cmd
}
