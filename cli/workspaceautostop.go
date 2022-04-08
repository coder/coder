package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/coderd/autostart/schedule"
	"github.com/coder/coder/codersdk"
)

const autostopDescriptionLong = `To have your workspace stop automatically at a regular time you can enable autostop.
When enabling autostop, provide a schedule. This schedule is in cron format except only
the following fields are allowed:
- minute
- hour
- day of week

For example, to stop your workspace every weekday at 5.30 pm, provide the schedule '30 17 1-5'.`

func workspaceAutostop() *cobra.Command {
	autostopCmd := &cobra.Command{
		Use:     "autostop enable <workspace> <schedule>",
		Short:   "schedule a workspace to automatically start at a regular time",
		Long:    autostopDescriptionLong,
		Example: "coder workspaces autostop enable my-workspace '30 17 1-5'",
		Hidden:  true, // TODO(cian): un-hide when autostop scheduling implemented
	}

	autostopCmd.AddCommand(workspaceAutostopEnable())
	autostopCmd.AddCommand(workspaceAutostopDisable())

	return autostopCmd
}

func workspaceAutostopEnable() *cobra.Command {
	return &cobra.Command{
		Use:               "enable <workspace_name> <schedule>",
		ValidArgsFunction: validArgsWorkspaceName,
		Args:              cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByName(cmd.Context(), codersdk.Me, args[0])
			if err != nil {
				return err
			}

			validSchedule, err := schedule.Weekly(args[1])
			if err != nil {
				return err
			}

			err = client.UpdateWorkspaceAutostop(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostopRequest{
				Schedule: validSchedule.String(),
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace will automatically stop at %s.\n\n", workspace.Name, validSchedule.Next(time.Now()))

			return nil
		},
	}
}

func workspaceAutostopDisable() *cobra.Command {
	return &cobra.Command{
		Use:               "disable <workspace_name>",
		ValidArgsFunction: validArgsWorkspaceName,
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByName(cmd.Context(), codersdk.Me, args[0])
			if err != nil {
				return err
			}

			err = client.UpdateWorkspaceAutostop(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostopRequest{
				Schedule: "",
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace will no longer automatically stop.\n\n", workspace.Name)

			return nil
		},
	}
}
