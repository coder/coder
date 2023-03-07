package cli

import (
	"github.com/coder/coder/cli/clibase"
	"github.com/spf13/cobra"
)

func templatePlan() *clibase.Command {
	return &clibase.Command{
		Use:   "plan <directory>",
		Args:  cobra.MinimumNArgs(1),
		Short: "Plan a template push from the current directory",
		Handler: func(inv *clibase.Invokation) error {
			return nil
		},
	}
}
