//go:build slim

package cli

import "github.com/coder/serpent"

func (r *RootCmd) scaletestCmd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "scaletest",
		Short: "Run a scale test against the Coder API",
		Handler: func(inv *serpent.Invocation) error {
			SlimUnsupported(inv.Stderr, "exp scaletest")
			return nil
		},
	}

	return cmd
}
