package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/codersdk"
	"github.com/spf13/cobra"
)

func workspaceUpdate() *cobra.Command {
	return &cobra.Command{
		Use: "update",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := client.WorkspaceByName(cmd.Context(), "", args[0])
			if err != nil {
				return err
			}
			if !workspace.Outdated {
				fmt.Printf("Workspace isn't outdated!\n")
				return nil
			}
			project, err := client.Project(cmd.Context(), workspace.ProjectID)
			if err != nil {
				return nil
			}
			before := time.Now()
			build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				ProjectVersionID: project.ActiveVersionID,
				Transition:       workspace.LatestBuild.Transition,
			})
			if err != nil {
				return err
			}
			logs, err := client.WorkspaceBuildLogsAfter(cmd.Context(), build.ID, before)
			if err != nil {
				return err
			}
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				fmt.Printf("Output: %s\n", log.Output)
			}
			return nil
		},
	}
}
