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
When enabling autostart, enter a schedule in the format: [day-of-week] start-time [location].
  * Start-time (required) is accepted either in 12-hour (hh:mm{am|pm}) format, or 24-hour format {hh:mm}.
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
		Use:         "autostart enable <workspace>",
		Short:       "schedule a workspace to automatically start at a regular time",
		Long:        autostartDescriptionLong,
		Example:     "coder autostart set my-workspace Mon-Fri 9:30AM Europe/Dublin",
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
			loc, _ := time.LoadLocation(validSchedule.Timezone())

			_, _ = fmt.Fprintf(cmd.OutOrStdout(),
				"schedule: %s\ntimezone: %s\nnext:     %s\n",
				validSchedule.Cron(),
				validSchedule.Timezone(),
				next.In(loc),
			)

			return nil
		},
	}
	return cmd
}

func autostartSet() *cobra.Command {
	cmd := &cobra.Command{
		Use:  "set <workspace_name> ",
		Args: cobra.MinimumNArgs(2),
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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace will automatically start at %s.\n\n", workspace.Name, sched.Next(time.Now()))

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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace will no longer automatically start.\n\n", workspace.Name)

			return nil
		},
	}
}

var errInvalidScheduleFormat = xerrors.New("Schedule must be in the format Mon-Fri 09:00AM America/Chicago")
var errInvalidTimeFormat = xerrors.New("Start time must be in the format hh:mm[am|pm] or HH:MM")

// parseCLISchedule parses a schedule in the format Mon-Fri 09:00AM America/Chicago.
func parseCLISchedule(parts ...string) (*schedule.Schedule, error) {
	// If the user was careful and quoted the schedule, un-quote it.
	// In the case that only time was specified, this will be a no-op.
	if len(parts) == 1 {
		parts = strings.Fields(parts[0])
	}
	timezone := ""
	dayOfWeek := "*"
	var hour, minute int
	switch len(parts) {
	case 1:
		t, err := parseTime(parts[0])
		if err != nil {
			return nil, err
		}
		hour, minute = t.Hour(), t.Minute()
	case 2:
		if !strings.Contains(parts[0], ":") {
			// DOW + Time
			t, err := parseTime(parts[1])
			if err != nil {
				return nil, err
			}
			hour, minute = t.Hour(), t.Minute()
			dayOfWeek = parts[0]
		} else {
			// Time + TZ
			t, err := parseTime(parts[0])
			if err != nil {
				return nil, err
			}
			hour, minute = t.Hour(), t.Minute()
			timezone = parts[1]
		}
	case 3:
		// DOW + Time + TZ
		t, err := parseTime(parts[1])
		if err != nil {
			return nil, err
		}
		hour, minute = t.Hour(), t.Minute()
		dayOfWeek = parts[0]
		timezone = parts[2]
	default:
		return nil, errInvalidScheduleFormat
	}

	// If timezone was not specified, attempt to automatically determine it as a last resort.
	if timezone == "" {
		loc, err := tz.TimezoneIANA()
		if err != nil {
			return nil, xerrors.Errorf("Could not automatically determine your timezone.")
		}
		timezone = loc.String()
	}

	sched, err := schedule.Weekly(fmt.Sprintf(
		"CRON_TZ=%s %d %d * * %s",
		timezone,
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
	// Assume only time provided, HH:MM[AM|PM]
	t, err := time.Parse(time.Kitchen, s)
	if err == nil {
		return t, nil
	}
	// Try 24-hour format without AM/PM suffix.
	t, err = time.Parse("15:04", s)
	if err != nil {
		return time.Time{}, errInvalidTimeFormat
	}
	return t, nil
}
