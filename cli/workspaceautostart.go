package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/coderd/autostart/schedule"
	"github.com/coder/coder/codersdk"
)

const autostartDescriptionLong = `To have your workspace build automatically at a regular time you can enable autostart.
When enabling autostart, provide a schedule. This schedule is in cron format except only
the following fields are allowed:
- minute
- hour
- day of week

For example, to start your workspace every weekday at 9.30 am, provide the schedule '30 9 1-5'.`

func workspaceAutostart() *cobra.Command {
	autostartCmd := &cobra.Command{
		Use:     "autostart enable <workspace> <schedule>",
		Short:   "schedule a workspace to automatically start at a regular time",
		Long:    autostartDescriptionLong,
		Example: "coder workspaces autostart enable my-workspace '30 9 1-5'",
		Hidden:  true, // TODO(cian): un-hide when autostart scheduling implemented
	}

	autostartCmd.AddCommand(workspaceAutostartEnable())
	autostartCmd.AddCommand(workspaceAutostartDisable())

	return autostartCmd
}

func workspaceAutostartEnable() *cobra.Command {
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

			err = client.UpdateWorkspaceAutostart(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: validSchedule.String(),
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace will automatically start at %s.\n\n", workspace.Name, validSchedule.Next(time.Now()))

			return nil
		},
	}
}

func workspaceAutostartDisable() *cobra.Command {
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

			err = client.UpdateWorkspaceAutostart(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: "",
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace will no longer automatically start.\n\n", workspace.Name)

			return nil
		},
	}
}
