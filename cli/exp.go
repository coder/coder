package cli

import "github.com/coder/serpent"

func (r *RootCmd) expCmd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "exp",
		Short: "Internal commands for testing and experimentation. These are prone to breaking changes with no notice.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Hidden: true,
		Children: []*serpent.Command{
			r.scaletestCmd(),
			r.errorExample(),
			r.mcpCommand(),
			r.promptExample(),
			r.rptyCommand(),
		},
	}
	return cmd
}
