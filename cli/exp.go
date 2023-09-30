package cli

import "github.com/coder/coder/v2/cli/clibase"

func (r *RootCmd) expCmd() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "exp",
		Short: "Internal commands for testing and experimentation. These are prone to breaking changes with no notice.",
		Handler: func(i *clibase.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Hidden: true,
		Children: []*clibase.Cmd{
			r.scaletestCmd(),
			r.errorExample(),
		},
	}
	return cmd
}
