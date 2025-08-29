package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) tasksCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "task",
		Aliases: []string{"tasks"},
		Short:   "Experimental task commands.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.taskList(),
			r.taskCreate(),
			r.taskStatus(),
			r.taskDelete(),
		},
	}
	return cmd
}
