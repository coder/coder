package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) tasksCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "task",
		Aliases: []string{"tasks"},
		Short:   "Manage tasks",
		// Coder Tasks is hidden from the product. Hiding the command keeps
		// it out of `coder --help` and out of the generated CLI reference
		// docs, while leaving it usable on deployments that set
		// CODER_ENABLE_AI_TASKS.
		Hidden: true,
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.taskCreate(),
			r.taskDelete(),
			r.taskList(),
			r.taskLogs(),
			r.taskPause(),
			r.taskResume(),
			r.taskSend(),
			r.taskStatus(),
		},
	}
	return cmd
}
