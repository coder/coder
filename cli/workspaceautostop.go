package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/coderd/autostart/schedule"
	"github.com/coder/coder/codersdk"
)

const autostopDescriptionLong = `To have your workspace stop automatically at a regular time you can enable autostop.
When enabling autostop, provide the minute, hour, and day(s) of week.
The default autostop schedule is at 18:00 in your local timezone (TZ env, UTC by default).
`

func workspaceAutostop() *cobra.Command {
	autostopCmd := &cobra.Command{
		Use:     "autostop enable <workspace>",
		Short:   "schedule a workspace to automatically stop at a regular time",
		Long:    autostopDescriptionLong,
		Example: "coder workspaces autostop enable my-workspace --minute 0 --hour 18 --days 1-5 -tz Europe/Dublin",
		Hidden:  true,
	}

	autostopCmd.AddCommand(workspaceAutostopEnable())
	autostopCmd.AddCommand(workspaceAutostopDisable())

	return autostopCmd
}

func workspaceAutostopEnable() *cobra.Command {
	// yes some of these are technically numbers but the cron library will do that work
	var autostopMinute string
	var autostopHour string
	var autostopDayOfWeek string
	var autostopTimezone string
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

			spec := fmt.Sprintf("CRON_TZ=%s %s %s * * %s", autostopTimezone, autostopMinute, autostopHour, autostopDayOfWeek)
			validSchedule, err := schedule.Weekly(spec)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
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

	cmd.Flags().StringVar(&autostopMinute, "minute", "0", "autostop minute")
	cmd.Flags().StringVar(&autostopHour, "hour", "18", "autostop hour")
	cmd.Flags().StringVar(&autostopDayOfWeek, "days", "1-5", "autostop day(s) of week")
	tzEnv := os.Getenv("TZ")
	if tzEnv == "" {
		tzEnv = "UTC"
	}
	cmd.Flags().StringVar(&autostopTimezone, "tz", tzEnv, "autostop timezone")
	return cmd
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
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
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
