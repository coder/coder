package cli

import (
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
)

func templates() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
		Short:   "Create, manage, and deploy templates",
		Aliases: []string{"template"},
		Example: `
  - Create a template for developers to create workspaces

    ` + cliui.Styles.Code.Render("$ coder templates create") + `

  - Make changes to your template, and plan the changes
 
    ` + cliui.Styles.Code.Render("$ coder templates plan <name>") + `

  - Update the template. Your developers can update their workspaces

    ` + cliui.Styles.Code.Render("$ coder templates update <name>"),
	}
	cmd.AddCommand(
		templateCreate(),
		templateInit(),
		templateList(),
		templatePlan(),
		templateUpdate(),
		templateVersions(),
		templateDelete(),
	)

	return cmd
}
