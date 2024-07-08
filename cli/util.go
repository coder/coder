package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/tz"
	"github.com/coder/serpent"
)

var (
	errInvalidScheduleFormat = xerrors.New("Schedule must be in the format Mon-Fri 09:00AM America/Chicago")
	errInvalidTimeFormat     = xerrors.New("Start time must be in the format hh:mm[am|pm] or HH:MM")
	errUnsupportedTimezone   = xerrors.New("The location you provided looks like a timezone. Check https://ipinfo.io for your location.")
)

// userSetOption returns true if the option was set by the user.
// This is helpful if the zero value of a flag is meaningful, and you need
// to distinguish between the user setting the flag to the zero value and
// the user not setting the flag at all.
func userSetOption(inv *serpent.Invocation, flagName string) bool {
	for _, opt := range inv.Command.Options {
		if opt.Name == flagName {
			return !(opt.ValueSource == serpent.ValueSourceNone || opt.ValueSource == serpent.ValueSourceDefault)
		}
	}
	return false
}

// durationDisplay formats a duration for easier display:
//   - Durations of 24 hours or greater are displays as Xd
//   - Durations less than 1 minute are displayed as <1m
//   - Duration is truncated to the nearest minute
//   - Empty minutes and seconds are truncated
//   - The returned string is the absolute value. Use sign()
//     if you need to indicate if the duration is positive or
//     negative.
func durationDisplay(d time.Duration) string {
	duration := d
	sign := ""
	if duration == 0 {
		return "0s"
	}
	if duration < 0 {
		duration *= -1
	}
	// duration > 0 now
	if duration < time.Minute {
		return sign + "<1m"
	}
	if duration > 24*time.Hour {
		duration = duration.Truncate(time.Hour)
	}
	if duration > time.Minute {
		duration = duration.Truncate(time.Minute)
	}
	days := 0
	for duration.Hours() >= 24 {
		days++
		duration -= 24 * time.Hour
	}
	durationDisplay := duration.String()
	if days > 0 {
		durationDisplay = fmt.Sprintf("%dd%s", days, durationDisplay)
	}
	for _, suffix := range []string{"m0s", "h0m", "d0s", "d0h"} {
		if strings.HasSuffix(durationDisplay, suffix) {
			durationDisplay = durationDisplay[:len(durationDisplay)-2]
		}
	}
	return sign + durationDisplay
}

// timeDisplay formats a time in the local timezone
// in RFC3339 format.
func timeDisplay(t time.Time) string {
	localTz, err := tz.TimezoneIANA()
	if err != nil {
		localTz = time.UTC
	}

	return t.In(localTz).Format(time.RFC3339)
}

// relative relativizes a duration with the prefix "ago" or "in"
func relative(d time.Duration) string {
	if d > 0 {
		return "in " + durationDisplay(d)
	}
	if d < 0 {
		return durationDisplay(d) + " ago"
	}
	return "now"
}

// parseCLISchedule parses a schedule in the format HH:MM{AM|PM} [DOW] [LOCATION]
func parseCLISchedule(parts ...string) (*cron.Schedule, error) {
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
			loc = time.UTC
		}
	}

	sched, err := cron.Weekly(fmt.Sprintf(
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

// parseDuration parses a duration from a string.
// If units are omitted, minutes are assumed.
func parseDuration(raw string) (time.Duration, error) {
	// If the user input a raw number, assume minutes
	if isDigit(raw) {
		raw = raw + "m"
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	return d, nil
}

func isDigit(s string) bool {
	return strings.IndexFunc(s, func(c rune) bool {
		return c < '0' || c > '9'
	}) == -1
}

// parseTime attempts to parse a time (no date) from the given string using a number of layouts.
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
		"3PM",
		"3pm",
	} {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t, nil
		}
	}
	return time.Time{}, errInvalidTimeFormat
}

func formatActiveDevelopers(n int) string {
	developerText := "developer"
	if n != 1 {
		developerText = "developers"
	}

	var nStr string
	if n < 0 {
		nStr = "-"
	} else {
		nStr = strconv.Itoa(n)
	}

	return fmt.Sprintf("%s active %s", nStr, developerText)
}
