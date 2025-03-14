package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/schedule/cron"

	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/tz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
const (
	scheduleShowDescriptionLong = `Shows the following information for the given workspace(s):
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
	scheduleExtendDescriptionLong = `
  * The new stop time is calculated from *now*.
  * The new stop time must be at least 30 minutes in the future.
  * The workspace template may restrict the maximum workspace runtime.
`
)
func (r *RootCmd) schedules() *serpent.Command {
	scheduleCmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "schedule { show | start | stop | extend } <workspace>",
		Short:       "Schedule automated start and stop times for workspaces",

		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.scheduleShow(),
			r.scheduleStart(),
			r.scheduleStop(),
			r.scheduleExtend(),
		},
	}
	return scheduleCmd
}
// scheduleShow() is just a wrapper for list() with some different defaults.
func (r *RootCmd) scheduleShow() *serpent.Command {
	var (
		filter    cliui.WorkspaceFilter

		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]scheduleListRow{},

				[]string{
					"workspace",
					"starts at",
					"starts next",
					"stops after",
					"stops next",
				},
			),
			cliui.JSONFormat(),
		)
	)
	client := new(codersdk.Client)
	showCmd := &serpent.Command{
		Use:   "show <workspace | --search <query> | --all>",
		Short: "Show workspace schedules",
		Long:  scheduleShowDescriptionLong,
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			// To preserve existing behavior, if an argument is passed we will
			// only show the schedule for that workspace.
			// This will clobber the search query if one is passed.
			f := filter.Filter()
			if len(inv.Args) == 1 {
				// If the argument contains a slash, we assume it's a full owner/name reference
				if strings.Contains(inv.Args[0], "/") {
					_, workspaceName, err := splitNamedWorkspace(inv.Args[0])
					if err != nil {
						return err
					}
					f.FilterQuery = fmt.Sprintf("name:%s", workspaceName)
				} else {
					// Otherwise, we assume it's a workspace name owned by the current user
					f.FilterQuery = fmt.Sprintf("owner:me name:%s", inv.Args[0])
				}
			}
			res, err := queryConvertWorkspaces(inv.Context(), client, f, scheduleListRowFromWorkspace)
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), res)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	filter.AttachOptions(&showCmd.Options)
	formatter.AttachOptions(&showCmd.Options)
	return showCmd
}
func (r *RootCmd) scheduleStart() *serpent.Command {

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use: "start <workspace-name> { <start-time> [day-of-week] [location] | manual }",
		Long: scheduleStartDescriptionLong + "\n" + FormatExamples(
			Example{
				Description: "Set the workspace to start at 9:30am (in Dublin) from Monday to Friday",
				Command:     "coder schedule start my-workspace 9:30AM Mon-Fri Europe/Dublin",
			},
		),

		Short: "Edit workspace start schedule",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(2, 4),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
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
func (r *RootCmd) scheduleStop() *serpent.Command {
	client := new(codersdk.Client)
	return &serpent.Command{
		Use: "stop <workspace-name> { <duration> | manual }",
		Long: scheduleStopDescriptionLong + "\n" + FormatExamples(

			Example{
				Command: "coder schedule stop my-workspace 2h30m",
			},
		),
		Short: "Edit workspace stop schedule",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(2),
			r.InitClient(client),

		),
		Handler: func(inv *serpent.Invocation) error {
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
func (r *RootCmd) scheduleExtend() *serpent.Command {
	client := new(codersdk.Client)
	extendCmd := &serpent.Command{
		Use:     "extend <workspace-name> <duration from now>",
		Aliases: []string{"override-stop"},

		Short:   "Extend the stop time of a currently running workspace instance.",
		Long: scheduleExtendDescriptionLong + "\n" + FormatExamples(
			Example{
				Command: "coder schedule extend my-workspace 90m",
			},
		),

		Middleware: serpent.Chain(
			serpent.RequireNArgs(2),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			extendDuration, err := parseDuration(inv.Args[1])
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return fmt.Errorf("get workspace: %w", err)
			}
			loc, err := tz.TimezoneIANA()
			if err != nil {
				loc = time.UTC // best effort
			}
			if extendDuration < 29*time.Minute {
				_, _ = fmt.Fprintf(
					inv.Stdout,
					"Please specify a duration of at least 30 minutes.\n",
				)
				return nil
			}
			newDeadline := time.Now().In(loc).Add(extendDuration)
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
	return extendCmd
}
func displaySchedule(ws codersdk.Workspace, out io.Writer) error {

	rows := []workspaceListRow{workspaceListRowFromWorkspace(time.Now(), ws)}
	rendered, err := cliui.DisplayTable(rows, "workspace", []string{
		"workspace", "starts at", "starts next", "stops after", "stops next",
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, rendered)

	return err
}
// scheduleListRow is a row in the schedule list.
// this is required for proper JSON output.
type scheduleListRow struct {
	WorkspaceName string `json:"workspace" table:"workspace,default_sort"`
	StartsAt      string `json:"starts_at" table:"starts at"`

	StartsNext    string `json:"starts_next" table:"starts next"`
	StopsAfter    string `json:"stops_after" table:"stops after"`
	StopsNext     string `json:"stops_next" table:"stops next"`
}
func scheduleListRowFromWorkspace(now time.Time, workspace codersdk.Workspace) scheduleListRow {
	autostartDisplay := ""
	nextStartDisplay := ""
	if !ptr.NilOrEmpty(workspace.AutostartSchedule) {
		if sched, err := cron.Weekly(*workspace.AutostartSchedule); err == nil {
			autostartDisplay = sched.Humanize()

			nextStartDisplay = timeDisplay(sched.Next(now))
		}
	}
	autostopDisplay := ""
	nextStopDisplay := ""
	if !ptr.NilOrZero(workspace.TTLMillis) {
		dur := time.Duration(*workspace.TTLMillis) * time.Millisecond
		autostopDisplay = durationDisplay(dur)
		if !workspace.LatestBuild.Deadline.IsZero() && workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionStart {
			nextStopDisplay = timeDisplay(workspace.LatestBuild.Deadline.Time)
		}
	}

	return scheduleListRow{
		WorkspaceName: workspace.OwnerName + "/" + workspace.Name,
		StartsAt:      autostartDisplay,
		StartsNext:    nextStartDisplay,
		StopsAfter:    autostopDisplay,
		StopsNext:     nextStopDisplay,
	}
}
