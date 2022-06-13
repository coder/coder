package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/coderd/util/tz"
	"github.com/coder/coder/codersdk"
)

const autostartDescriptionLong = `To have your workspace build automatically at a regular time you can enable autostart.
When enabling autostart, enter a schedule in the format: <start-time> [day-of-week] [location].
  * Start-time (required) is accepted either in 12-hour (hh:mm{am|pm}) format, or 24-hour format hh:mm.
  * Day-of-week (optional) allows specifying in the cron format, e.g. 1,3,5 or Mon-Fri.
    Aliases such as @daily are not supported.
    Default: * (every day)
  * Location (optional) must be a valid location in the IANA timezone database.
    If omitted, we will fall back to either the TZ environment variable or /etc/localtime.
    You can check your corresponding location by visiting https://ipinfo.io - it shows in the demo widget on the right.
`

func autostart() *cobra.Command {
	autostartCmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "autostart set <workspace> <start-time> [day-of-week] [location]",
		Short:       "schedule a workspace to automatically start at a regular time",
		Long:        autostartDescriptionLong,
		Example:     "coder autostart set my-workspace 9:30AM Mon-Fri Europe/Dublin",
	}

	autostartCmd.AddCommand(autostartShow())
	autostartCmd.AddCommand(autostartSet())
	autostartCmd.AddCommand(autostartUnset())

	return autostartCmd
}

func autostartShow() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "show <workspace_name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}

			if workspace.AutostartSchedule == nil || *workspace.AutostartSchedule == "" {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "not enabled\n")
				return nil
			}

			validSchedule, err := schedule.Weekly(*workspace.AutostartSchedule)
			if err != nil {
				// This should never happen.
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Invalid autostart schedule %q for workspace %s: %s\n", *workspace.AutostartSchedule, workspace.Name, err.Error())
				return nil
			}

			next := validSchedule.Next(time.Now())

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"schedule: %s\ntimezone: %s\nnext:     %s\n",
				validSchedule.Cron(),
				validSchedule.Location(),
				next.In(validSchedule.Location()),
			)

			return nil
		},
	}
	return cmd
}

func autostartSet() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "set <workspace_name> <start-time> [day-of-week] [location]",
		Args: cobra.RangeArgs(2, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			sched, err := parseCLISchedule(args[1:]...)
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}

			err = client.UpdateWorkspaceAutostart(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: ptr.Ref(sched.String()),
			})
			if err != nil {
				return err
			}

			schedNext := sched.Next(time.Now())
			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"%s will automatically start at %s %s (%s)\n",
				workspace.Name,
				schedNext.In(sched.Location()).Format(time.Kitchen),
				sched.DaysOfWeek(),
				sched.Location().String(),
			)
			return nil
		},
	}

	return cmd
}

func autostartUnset() *cobra.Command {
	return &cobra.Command{
		Use:  "unset <workspace_name>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}

			err = client.UpdateWorkspaceAutostart(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceAutostartRequest{
				Schedule: nil,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s will no longer automatically start.\n", workspace.Name)

			return nil
		},
	}
}

var errInvalidScheduleFormat = xerrors.New("Schedule must be in the format Mon-Fri 09:00AM America/Chicago")
var errInvalidTimeFormat = xerrors.New("Start time must be in the format hh:mm[am|pm] or HH:MM")
var errUnsupportedTimezone = xerrors.New("The location you provided looks like a timezone. Check https://ipinfo.io for your location.")

// parseCLISchedule parses a schedule in the format HH:MM{AM|PM} [DOW] [LOCATION]
func parseCLISchedule(parts ...string) (*schedule.Schedule, error) {
	// If the user was careful and quoted the schedule, un-quote it.
	// In the case that only time was specified, this will be a no-op.
	if len(parts) == 1 {
		parts = strings.Fields(parts[0])
	}
	var loc *time.Location
	dayOfWeek := "*"
	t, err := parseTime(parts[0])
	if err != nil {
		return nil, err
	}
	hour, minute := t.Hour(), t.Minute()

	// Any additional parts get ignored.
	switch len(parts) {
	case 3:
		dayOfWeek = parts[1]
		loc, err = time.LoadLocation(parts[2])
		if err != nil {
			_, err = time.Parse("MST", parts[2])
			if err == nil {
				return nil, errUnsupportedTimezone
			}
			return nil, xerrors.Errorf("Invalid timezone %q specified: a valid IANA timezone is required", parts[2])
		}
	case 2:
		// Did they provide day-of-week or location?
		if maybeLoc, err := time.LoadLocation(parts[1]); err != nil {
			// Assume day-of-week.
			dayOfWeek = parts[1]
		} else {
			loc = maybeLoc
		}
	case 1: // already handled
	default:
		return nil, errInvalidScheduleFormat
	}

	// If location was not specified, attempt to automatically determine it as a last resort.
	if loc == nil {
		loc, err = tz.TimezoneIANA()
		if err != nil {
			return nil, xerrors.Errorf("Could not automatically determine your timezone")
		}
	}

	sched, err := schedule.Weekly(fmt.Sprintf(
		"CRON_TZ=%s %d %d * * %s",
		loc.String(),
		minute,
		hour,
		dayOfWeek,
	))
	if err != nil {
		// This will either be an invalid dayOfWeek or an invalid timezone.
		return nil, xerrors.Errorf("Invalid schedule: %w", err)
	}

	return sched, nil
}

func parseTime(s string) (time.Time, error) {
	// Try a number of possible layouts.
	for _, layout := range []string{
		time.Kitchen, // 03:04PM
		"03:04pm",
		"3:04PM",
		"3:04pm",
		"15:04",
		"1504",
		"03PM",
		"03pm",
	} {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, errInvalidTimeFormat
}
