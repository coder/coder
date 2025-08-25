package cli

import "github.com/coder/serpent"

func (r *RootCmd) task() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "task",
		Short:   "Manage tasks",
		Aliases: []string{"tasks"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.taskCreate(),
		},
	}

	return cmd
}
