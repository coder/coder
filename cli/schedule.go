package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/coderd/util/tz"
	"github.com/coder/coder/codersdk"
)

const (
	scheduleShowDescriptionLong = `Shows the following information for the given workspace:
  * The automatic start schedule
  * The next scheduled start time
  * The duration after which it will stop
  * The next scheduled stop time
`
	scheduleStartDescriptionLong = `Schedules a workspace to regularly start at a specific time.
Schedule format: <start-time> [day-of-week] [location].
  * Start-time (required) is accepted either in 12-hour (hh:mm{am|pm}) format, or 24-hour format hh:mm.
  * Day-of-week (optional) allows specifying in the cron format, e.g. 1,3,5 or Mon-Fri.
    Aliases such as @daily are not supported.
    Default: * (every day)
  * Location (optional) must be a valid location in the IANA timezone database.
    If omitted, we will fall back to either the TZ environment variable or /etc/localtime.
    You can check your corresponding location by visiting https://ipinfo.io - it shows in the demo widget on the right.
`
	scheduleStopDescriptionLong = `Schedules a workspace to stop after a given duration has elapsed.
  * Workspace runtime is measured from the time that the workspace build completed.
  * The minimum scheduled stop time is 1 minute.
  * The workspace template may place restrictions on the maximum shutdown time.
  * Changes to workspace schedules only take effect upon the next build of the workspace,
    and do not affect a running instance of a workspace.

When enabling scheduled stop, enter a duration in one of the following formats:
  * 3h2m (3 hours and two minutes)
  * 3h   (3 hours)
  * 2m   (2 minutes)
  * 2    (2 minutes)
`
	scheduleOverrideDescriptionLong = `Override the stop time of a currently running workspace instance.
  * The new stop time is calculated from *now*.
  * The new stop time must be at least 30 minutes in the future.
  * The workspace template may restrict the maximum workspace runtime.
`
)

func schedules() *cobra.Command {
	scheduleCmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "schedule { show | start | stop | override } <workspace>",
		Short:       "Schedule automated start and stop times for workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	scheduleCmd.AddCommand(
		scheduleShow(),
		scheduleStart(),
		scheduleStop(),
		scheduleOverride(),
	)

	return scheduleCmd
}

func scheduleShow() *cobra.Command {
	showCmd := &cobra.Command{
		Use:   "show <workspace-name>",
		Short: "Show workspace schedule",
		Long:  scheduleShowDescriptionLong,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}

			return displaySchedule(workspace, cmd.OutOrStdout())
		},
	}
	return showCmd
}

func scheduleStart() *cobra.Command {
	cmd := &cobra.Command{
		Use: "start <workspace-name> { <start-time> [day-of-week] [location] | manual }",
		Example: formatExamples(
			example{
				Description: "Set the workspace to start at 9:30am (in Dublin) from Monday to Friday",
				Command:     "coder schedule start my-workspace 9:30AM Mon-Fri Europe/Dublin",
			},
		),
		Short: "Edit workspace start schedule",
		Long:  scheduleStartDescriptionLong,
		Args:  cobra.RangeArgs(2, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}

			var schedStr *string
			if args[1] != "manual" {
				sched, err := parseCLISchedule(args[1:]...)
				if err != nil {
					return err
				}

				schedStr = ptr.Ref(sched.String())
			}

			err = client.UpdateWorkspaceAutostart(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: schedStr,
			})
			if err != nil {
				return err
			}

			updated, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}
			return displaySchedule(updated, cmd.OutOrStdout())
		},
	}

	return cmd
}

func scheduleStop() *cobra.Command {
	return &cobra.Command{
		Args: cobra.ExactArgs(2),
		Use:  "stop <workspace-name> { <duration> | manual }",
		Example: formatExamples(
			example{
				Command: "coder schedule stop my-workspace 2h30m",
			},
		),
		Short: "Edit workspace stop schedule",
		Long:  scheduleStopDescriptionLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}

			var durMillis *int64
			if args[1] != "manual" {
				dur, err := parseDuration(args[1])
				if err != nil {
					return err
				}
				durMillis = ptr.Ref(dur.Milliseconds())
			}

			if err := client.UpdateWorkspaceTTL(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: durMillis,
			}); err != nil {
				return err
			}

			updated, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}
			return displaySchedule(updated, cmd.OutOrStdout())
		},
	}
}

func scheduleOverride() *cobra.Command {
	overrideCmd := &cobra.Command{
		Args: cobra.ExactArgs(2),
		Use:  "override-stop <workspace-name> <duration from now>",
		Example: formatExamples(
			example{
				Command: "coder schedule override-stop my-workspace 90m",
			},
		),
		Short: "Edit stop time of active workspace",
		Long:  scheduleOverrideDescriptionLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			overrideDuration, err := parseDuration(args[1])
			if err != nil {
				return err
			}

			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			loc, err := tz.TimezoneIANA()
			if err != nil {
				loc = time.UTC // best effort
			}

			if overrideDuration < 29*time.Minute {
				_, _ = fmt.Fprintf(
					cmd.OutOrStdout(),
					"Please specify a duration of at least 30 minutes.\n",
				)
				return nil
			}

			newDeadline := time.Now().In(loc).Add(overrideDuration)
			if err := client.PutExtendWorkspace(cmd.Context(), workspace.ID, codersdk.PutExtendWorkspaceRequest{
				Deadline: newDeadline,
			}); err != nil {
				return err
			}

			updated, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}
			return displaySchedule(updated, cmd.OutOrStdout())
		},
	}
	return overrideCmd
}

func displaySchedule(workspace codersdk.Workspace, out io.Writer) error {
	loc, err := tz.TimezoneIANA()
	if err != nil {
		loc = time.UTC // best effort
	}

	var (
		schedStart     = "manual"
		schedStop      = "manual"
		schedNextStart = "-"
		schedNextStop  = "-"
	)
	if !ptr.NilOrEmpty(workspace.AutostartSchedule) {
		sched, err := schedule.Weekly(ptr.NilToEmpty(workspace.AutostartSchedule))
		if err != nil {
			// This should never happen.
			_, _ = fmt.Fprintf(out, "Invalid autostart schedule %q for workspace %s: %s\n", *workspace.AutostartSchedule, workspace.Name, err.Error())
			return nil
		}
		schedNext := sched.Next(time.Now()).In(sched.Location())
		schedStart = fmt.Sprintf("%s %s (%s)", sched.Time(), sched.DaysOfWeek(), sched.Location())
		schedNextStart = schedNext.Format(timeFormat + " on " + dateFormat)
	}

	if !ptr.NilOrZero(workspace.TTLMillis) {
		d := time.Duration(*workspace.TTLMillis) * time.Millisecond
		schedStop = durationDisplay(d) + " after start"
	}

	if !workspace.LatestBuild.Deadline.IsZero() {
		if workspace.LatestBuild.Transition != "start" {
			schedNextStop = "-"
		} else {
			schedNextStop = workspace.LatestBuild.Deadline.Time.In(loc).Format(timeFormat + " on " + dateFormat)
			schedNextStop = fmt.Sprintf("%s (in %s)", schedNextStop, durationDisplay(time.Until(workspace.LatestBuild.Deadline.Time)))
		}
	}

	tw := cliui.Table()
	tw.AppendRow(table.Row{"Starts at", schedStart})
	tw.AppendRow(table.Row{"Starts next", schedNextStart})
	tw.AppendRow(table.Row{"Stops at", schedStop})
	tw.AppendRow(table.Row{"Stops next", schedNextStop})

	_, _ = fmt.Fprintln(out, tw.Render())
	return nil
}
