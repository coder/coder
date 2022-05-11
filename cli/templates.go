package cli

import (
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func templates() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
		Short:   "Create, manage, and deploy templates",
		Aliases: []string{"template"},
		Example: `
  - Create a template for developers to create workspaces

    ` + color.New(color.FgHiMagenta).Sprint("$ coder templates create") + `

  - Make changes to your template, and plan the changes
 
    ` + color.New(color.FgHiMagenta).Sprint("$ coder templates plan <name>") + `

  - Update the template. Your developers can update their workspaces

    ` + color.New(color.FgHiMagenta).Sprint("$ coder templates update <name>"),
	}
	cmd.AddCommand(
		templateCreate(),
		templateInit(),
		templateList(),
		templatePlan(),
		templateUpdate(),
		templateVersions(),
	)

	return cmd
}
