package cli

import (
	"fmt"
	"sort"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func templates() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
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
		templateEdit(),
		templateInit(),
		templateList(),
		templatePlan(),
		templateUpdate(),
		templateVersions(),
	)

	return cmd
}

func displayTemplateVersionInfo(cmd *cobra.Command, resources []codersdk.WorkspaceResource) error {
	sort.Slice(resources, func(i, j int) bool {
		return fmt.Sprintf("%s.%s", resources[i].Type, resources[i].Name) < fmt.Sprintf("%s.%s", resources[j].Type, resources[j].Name)
	})

	addressOnStop := map[string]codersdk.WorkspaceResource{}
	for _, resource := range resources {
		if resource.Transition != database.WorkspaceTransitionStop {
			continue
		}
		addressOnStop[resource.Address] = resource
	}

	displayed := map[string]struct{}{}
	for _, resource := range resources {
		if resource.Type == "random_string" {
			// Hide resources that aren't substantial to a user!
			continue
		}
		_, alreadyShown := displayed[resource.Address]
		if alreadyShown {
			continue
		}
		displayed[resource.Address] = struct{}{}

		_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Bold.Render("resource."+resource.Type+"."+resource.Name))
		_, existsOnStop := addressOnStop[resource.Address]
		if existsOnStop {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Warn.Render("~ persistent"))
		} else {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Keyword.Render("+ start")+cliui.Styles.Placeholder.Render(" (deletes on stop)"))
		}
		if resource.Agent != nil {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Fuschia.Render("▲ allows ssh"))
		}
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}
	return nil
}
