package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/coderd/autostart/schedule"
	"github.com/coder/coder/codersdk"
)

const autostartDescriptionLong = `To have your workspace build automatically at a regular time you can enable autostart.
When enabling autostart, provide the minute, hour, and day(s) of week.
The default schedule is at 09:00 in your local timezone (TZ env, UTC by default).
`

func workspaceAutostart() *cobra.Command {
	autostartCmd := &cobra.Command{
		Use:     "autostart enable <workspace>",
		Short:   "schedule a workspace to automatically start at a regular time",
		Long:    autostartDescriptionLong,
		Example: "coder workspaces autostart enable my-workspace --minute 30 --hour 9 --days 1-5 --tz Europe/Dublin",
		Hidden:  true,
	}

	autostartCmd.AddCommand(workspaceAutostartEnable())
	autostartCmd.AddCommand(workspaceAutostartDisable())

	return autostartCmd
}

func workspaceAutostartEnable() *cobra.Command {
	// yes some of these are technically numbers but the cron library will do that work
	var autostartMinute string
	var autostartHour string
	var autostartDayOfWeek string
	var autostartTimezone string
	cmd := &cobra.Command{
		Use:               "enable <workspace_name> <schedule>",
		ValidArgsFunction: validArgsWorkspaceName,
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			spec := fmt.Sprintf("CRON_TZ=%s %s %s * * %s", autostartTimezone, autostartMinute, autostartHour, autostartDayOfWeek)
			validSchedule, err := schedule.Weekly(spec)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
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

	cmd.Flags().StringVar(&autostartMinute, "minute", "0", "autostart minute")
	cmd.Flags().StringVar(&autostartHour, "hour", "9", "autostart hour")
	cmd.Flags().StringVar(&autostartDayOfWeek, "days", "1-5", "autostart day(s) of week")
	tzEnv := os.Getenv("TZ")
	if tzEnv == "" {
		tzEnv = "UTC"
	}
	cmd.Flags().StringVar(&autostartTimezone, "tz", tzEnv, "autostart timezone")
	return cmd
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
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
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
