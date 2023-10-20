package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/tz"
	"github.com/coder/coder/v2/codersdk"
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
	scheduleOverrideDescriptionLong = `
  * The new stop time is calculated from *now*.
  * The new stop time must be at least 30 minutes in the future.
  * The workspace template may restrict the maximum workspace runtime.
`
)

func (r *RootCmd) schedules() *clibase.Cmd {
	scheduleCmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "schedule { show | start | stop | override } <workspace>",
		Short:       "Schedule automated start and stop times for workspaces",
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.scheduleShow(),
			r.scheduleStart(),
			r.scheduleStop(),
			r.scheduleOverride(),
		},
	}

	return scheduleCmd
}

func (r *RootCmd) scheduleShow() *clibase.Cmd {
	client := new(codersdk.Client)
	showCmd := &clibase.Cmd{
		Use:   "show <workspace-name>",
		Short: "Show workspace schedule",
		Long:  scheduleShowDescriptionLong,
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			return displaySchedule(workspace, inv.Stdout)
		},
	}
	return showCmd
}

func (r *RootCmd) scheduleStart() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use: "start <workspace-name> { <start-time> [day-of-week] [location] | manual }",
		Long: scheduleStartDescriptionLong + "\n" + formatExamples(
			example{
				Description: "Set the workspace to start at 9:30am (in Dublin) from Monday to Friday",
				Command:     "coder schedule start my-workspace 9:30AM Mon-Fri Europe/Dublin",
			},
		),
		Short: "Edit workspace start schedule",
		Middleware: clibase.Chain(
			clibase.RequireRangeArgs(2, 4),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			var schedStr *string
			if inv.Args[1] != "manual" {
				sched, err := parseCLISchedule(inv.Args[1:]...)
				if err != nil {
					return err
				}

				schedStr = ptr.Ref(sched.String())
			}

			err = client.UpdateWorkspaceAutostart(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: schedStr,
			})
			if err != nil {
				return err
			}

			updated, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			return displaySchedule(updated, inv.Stdout)
		},
	}

	return cmd
}

func (r *RootCmd) scheduleStop() *clibase.Cmd {
	client := new(codersdk.Client)
	return &clibase.Cmd{
		Use: "stop <workspace-name> { <duration> | manual }",
		Long: scheduleStopDescriptionLong + "\n" + formatExamples(
			example{
				Command: "coder schedule stop my-workspace 2h30m",
			},
		),
		Short: "Edit workspace stop schedule",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			var durMillis *int64
			if inv.Args[1] != "manual" {
				dur, err := parseDuration(inv.Args[1])
				if err != nil {
					return err
				}
				durMillis = ptr.Ref(dur.Milliseconds())
			}

			if err := client.UpdateWorkspaceTTL(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceTTLRequest{
				TTLMillis: durMillis,
			}); err != nil {
				return err
			}

			updated, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			return displaySchedule(updated, inv.Stdout)
		},
	}
}

func (r *RootCmd) scheduleOverride() *clibase.Cmd {
	client := new(codersdk.Client)
	overrideCmd := &clibase.Cmd{
		Use:   "override-stop <workspace-name> <duration from now>",
		Short: "Override the stop time of a currently running workspace instance.",
		Long: scheduleOverrideDescriptionLong + "\n" + formatExamples(
			example{
				Command: "coder schedule override-stop my-workspace 90m",
			},
		),
		Middleware: clibase.Chain(
			clibase.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			overrideDuration, err := parseDuration(inv.Args[1])
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			loc, err := tz.TimezoneIANA()
			if err != nil {
				loc = time.UTC // best effort
			}

			if overrideDuration < 29*time.Minute {
				_, _ = fmt.Fprintf(
					inv.Stdout,
					"Please specify a duration of at least 30 minutes.\n",
				)
				return nil
			}

			newDeadline := time.Now().In(loc).Add(overrideDuration)
			if err := client.PutExtendWorkspace(inv.Context(), workspace.ID, codersdk.PutExtendWorkspaceRequest{
				Deadline: newDeadline,
			}); err != nil {
				return err
			}

			updated, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			return displaySchedule(updated, inv.Stdout)
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
		sched, err := cron.Weekly(ptr.NilToEmpty(workspace.AutostartSchedule))
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
