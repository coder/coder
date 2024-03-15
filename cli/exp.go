package cli

import "github.com/coder/serpent"

func (r *RootCmd) expCmd() *serpent.Cmd {
	cmd := &serpent.Cmd{
		Use:   "exp",
		Short: "Internal commands for testing and experimentation. These are prone to breaking changes with no notice.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Hidden: true,
		Children: []*serpent.Cmd{
			r.scaletestCmd(),
			r.errorExample(),
		},
	}
	return cmd
}
