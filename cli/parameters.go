package cli

import (
	"github.com/spf13/cobra"
)

func parameters() *cobra.Command {
	cmd := &cobra.Command{
		Short: "List parameters for a given scope",
		Example: formatExamples(
			example{
				Command: "coder parameters list workspace my-workspace",
			},
		),
		Use: "parameters",
		// Currently hidden as this shows parameter values, not parameter
		// schemes. Until we have a good way to distinguish the two, it's better
		// not to add confusion or lock ourselves into a certain api.
		// This cmd is still valuable debugging tool for devs to avoid
		// constructing curl requests.
		Hidden:  true,
		Aliases: []string{"params"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		parameterList(),
	)
	return cmd
}
