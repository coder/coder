package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
	"github.com/spf13/cobra"
)

func workspaceStart() *cobra.Command {
	return &cobra.Command{
		Use:               "start <workspace>",
		ValidArgsFunction: validArgsWorkspaceName,
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := client.WorkspaceByName(cmd.Context(), "", args[0])
			if err != nil {
				return err
			}
			before := time.Now()
			build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: database.WorkspaceTransitionStart,
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
