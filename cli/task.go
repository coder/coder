package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) tasksCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "task",
		Aliases: []string{"tasks"},
		Short:   "Manage tasks",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.taskCreate(),
			r.taskDelete(),
			r.taskList(),
			r.taskLogs(),
			r.taskSend(),
			r.taskStatus(),
		},
	}
	return cmd
}
