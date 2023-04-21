package cli

import (
	"github.com/coder/coder/cli/clibase"
)

func (r *RootCmd) parameters() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Short: "List parameters for a given scope",
		Long: formatExamples(
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
		Children: []*clibase.Cmd{
			r.parameterList(),
		},
	}
	return cmd
}
