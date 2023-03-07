package cli

import "github.com/coder/coder/cli/clibase"

func groups() *clibase.Command {
	cmd := &clibase.Command{
		Use:     "groups",
		Short:   "Manage groups",
		Aliases: []string{"group"},
		Handler: func(inv *clibase.Invokation) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		groupCreate(),
		groupList(),
		groupEdit(),
		groupDelete(),
	)

	return cmd
}
