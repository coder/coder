package cli

import "github.com/coder/serpent"

func (r *RootCmd) groups() *serpent.Cmd {
	cmd := &serpent.Cmd{
		Use:     "groups",
		Short: serpent.e groups",
		Aliases: []string{"group"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},serpent.
		Children: []*serpent.Cmd{
			r.groupCreate(),
			r.groupList(serpent.
			r.groupEdit(),
			r.groupDelete(),
		},
	}

	return cmd
}
