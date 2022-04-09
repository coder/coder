package cliui

import (
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
	"github.com/spf13/cobra"
	"github.com/xlab/treeprint"
)

func WorkspaceResources(cmd *cobra.Command, resources []codersdk.WorkspaceResource) error {
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

	writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 4, ' ', 0)
	_, _ = fmt.Fprintf(writer, "Type\tName\tGood\n")
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

		tree := treeprint.NewWithRoot(resource.Type + "." + resource.Name)

		// _, _ = fmt.Fprintln(cmd.OutOrStdout(), resource.Type+"."+resource.Name)
		_, existsOnStop := addressOnStop[resource.Address]
		if existsOnStop {
			// _, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+Styles.Warn.Render("~ persistent"))
		} else {
			// _, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+Styles.Keyword.Render("+ start")+Styles.Placeholder.Render(" (deletes on stop)"))
		}

		for _, agent := range resource.Agents {
			tree.AddNode(agent.Name + " " + agent.OperatingSystem)
		}

		fmt.Fprintln(cmd.OutOrStdout(), tree.String())

		// if resource.Agent != nil {
		// _, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+Styles.Fuschia.Render("▲ allows ssh"))
		// }
		// _, _ = fmt.Fprintln(cmd.OutOrStdout())
	}
	return writer.Flush()
}
