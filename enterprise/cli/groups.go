package cli

import (
	"github.com/coder/coder/cli/clibase"
	"gvisor.dev/gvisor/runsc/cmd"
)

func (r *RootCmd) groups() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "groups",
		Short:   "Manage groups",
		Aliases: []string{"group"},
		Handler: func(inv *clibase.Invokation) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		groupCreate(),
		groupList(),
		groupEdit(),
		groupDelete(),
	)

	return cmd
}
