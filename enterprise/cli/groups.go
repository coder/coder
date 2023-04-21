package cli

import (
	"github.com/coder/coder/cli/clibase"
)

func (r *RootCmd) groups() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "groups",
		Short:   "Manage groups",
		Aliases: []string{"group"},
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.groupCreate(),
			r.groupList(),
			r.groupEdit(),
			r.groupDelete(),
		},
	}

	return cmd
}
