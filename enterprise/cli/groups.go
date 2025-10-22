package cli

import "github.com/coder/serpent"

func (r *RootCmd) groups() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "groups",
		Short:   "Manage groups",
		Aliases: []string{"group"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.groupCreate(),
			r.groupList(),
			r.groupEdit(),
			r.groupDelete(),
		},
	}

	return cmd
}
