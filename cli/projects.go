package cli

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func projects() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "projects",
		Long: "Testing something",
		Example: `
  - Create a project for developers to create workspaces

    ` + color.New(color.FgHiMagenta).Sprint("$ coder projects create") + `

  - Make changes to your project, and plan the changes
 
    ` + color.New(color.FgHiMagenta).Sprint("$ coder projects plan <name>") + `

  - Update the project. Your developers can update their workspaces

    ` + color.New(color.FgHiMagenta).Sprint("$ coder projects update <name>"),
	}
	cmd.AddCommand(projectCreate())
	cmd.AddCommand(projectPlan())
	cmd.AddCommand(projectUpdate())

	return cmd
}
