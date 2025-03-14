package cli
import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"github.com/coder/coder/v2/coderd/schedule/cron"
	"github.com/coder/coder/v2/coderd/util/tz"
	"github.com/coder/serpent"
)
var (
	errInvalidScheduleFormat = errors.New("Schedule must be in the format Mon-Fri 09:00AM America/Chicago")
	errInvalidTimeFormat     = errors.New("Start time must be in the format hh:mm[am|pm] or HH:MM")
	errUnsupportedTimezone   = errors.New("The location you provided looks like a timezone. Check https://ipinfo.io for your location.")
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
			return nil, fmt.Errorf("Invalid timezone %q specified: a valid IANA timezone is required", parts[2])
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
		return nil, fmt.Errorf("Invalid schedule: %w", err)
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
// extendedParseDuration is a more lenient version of parseDuration that allows
// for more flexible input formats and cumulative durations.
// It allows for some extra units:
//   - d (days, interpreted as 24h)
//   - y (years, interpreted as 8_760h)
//
// FIXME: handle fractional values as discussed in https://github.com/coder/coder/pull/15040#discussion_r1799261736
func extendedParseDuration(raw string) (time.Duration, error) {
	var d int64
	isPositive := true
	// handle negative durations by checking for a leading '-'
	if strings.HasPrefix(raw, "-") {
		raw = raw[1:]
		isPositive = false
	}
	if raw == "" {
		return 0, fmt.Errorf("invalid duration: %q", raw)
	}
	// Regular expression to match any characters that do not match the expected duration format
	invalidCharRe := regexp.MustCompile(`[^0-9|nsuµhdym]+`)
	if invalidCharRe.MatchString(raw) {
		return 0, fmt.Errorf("invalid duration format: %q", raw)
	}
	// Regular expression to match numbers followed by 'd', 'y', or time units
	re := regexp.MustCompile(`(-?\d+)(ns|us|µs|ms|s|m|h|d|y)`)
	matches := re.FindAllStringSubmatch(raw, -1)
	for _, match := range matches {
		var num int64
		num, err := strconv.ParseInt(match[1], 10, 0)
		if err != nil {
			return 0, fmt.Errorf("invalid duration: %q", match[1])
		}
		switch match[2] {
		case "d":
			// we want to check if d + num * int64(24*time.Hour) would overflow
			if d > (1<<63-1)-num*int64(24*time.Hour) {
				return 0, fmt.Errorf("invalid duration: %q", raw)
			}
			d += num * int64(24*time.Hour)
		case "y":
			// we want to check if d + num * int64(8760*time.Hour) would overflow
			if d > (1<<63-1)-num*int64(8760*time.Hour) {
				return 0, fmt.Errorf("invalid duration: %q", raw)
			}
			d += num * int64(8760*time.Hour)
		case "h", "m", "s", "ns", "us", "µs", "ms":
			partDuration, err := time.ParseDuration(match[0])
			if err != nil {
				return 0, fmt.Errorf("invalid duration: %q", match[0])
			}
			if d > (1<<63-1)-int64(partDuration) {
				return 0, fmt.Errorf("invalid duration: %q", raw)
			}
			d += int64(partDuration)
		default:
			return 0, fmt.Errorf("invalid duration unit: %q", match[2])
		}
	}
	if !isPositive {
		return -time.Duration(d), nil
	}
	return time.Duration(d), nil
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
