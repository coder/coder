//go:build slim

package cli

import "github.com/coder/coder/v2/cli/clibase"

func (r *RootCmd) scaletestCmd() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "scaletest",
		Short: "Run a scale test against the Coder API",
		Handler: func(inv *clibase.Invocation) error {
			SlimUnsupported(inv.Stderr, "exp scaletest")
			return nil
		},
	}

	return cmd
}
