package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func workspaceShow() *cobra.Command {
	return &cobra.Command{
		Use: "show",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := client.WorkspaceByName(cmd.Context(), "", args[0])
			if err != nil {
				return err
			}
			resources, err := client.WorkspaceResourcesByBuild(cmd.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return err
			}
			for _, resource := range resources {
				if resource.Agent == nil {
					continue
				}

				_, _ = fmt.Printf("Agent: %+v\n", resource.Agent)
			}
			return nil
		},
	}
}
