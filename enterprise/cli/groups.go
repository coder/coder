package cli

import "github.com/spf13/cobra"

func groups() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "groups",
		Short:   "Manage groups",
		Aliases: []string{"group"},
		RunE: func(cmd *cobra.Command, args []string) error {
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
