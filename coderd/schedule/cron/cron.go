// package schedule provides utilities for managing template and workspace
// autostart and autostop schedules. This includes utilities for parsing and
// deserializing cron-style expressions.
package cron

import (
	"fmt"
	"strings"
	"time"

	rbcron "github.com/robfig/cron/v3"
	"golang.org/x/xerrors"
)

// For the purposes of this library, we only need minute, hour, and
// day-of-week. However to ensure interoperability we will use the standard
// five-valued cron format. Descriptors are not supported.
const parserFormat = rbcron.Minute | rbcron.Hour | rbcron.Dom | rbcron.Month | rbcron.Dow

var defaultParser = rbcron.NewParser(parserFormat)

// Weekly parses a Schedule from spec scoped to a recurring weekly event.
// Spec consists of the following space-delimited fields, in the following order:
// - timezone e.g. CRON_TZ=US/Central (optional)
// - minutes of hour e.g. 30 (required)
// - hour of day e.g. 9 (required)
// - day of month (must be *)
// - month (must be *)
// - day of week e.g. 1 (required)
//
// Example Usage:
//
//	local_sched, _ := cron.Weekly("59 23 *")
//	fmt.Println(sched.Next(time.Now().Format(time.RFC3339)))
//	// Output: 2022-04-04T23:59:00Z
//
//	us_sched, _ := cron.Weekly("CRON_TZ=US/Central 30 9 1-5")
//	fmt.Println(sched.Next(time.Now()).Format(time.RFC3339))
//	// Output: 2022-04-04T14:30:00Z
func Weekly(raw string) (*Schedule, error) {
	if err := validateWeeklySpec(raw); err != nil {
		return nil, xerrors.Errorf("validate weekly schedule: %w", err)
	}

	return parse(raw)
}

// Daily parses a Schedule from spec scoped to a recurring daily event.
// Spec consists of the following space-delimited fields, in the following order:
// - timezone e.g. CRON_TZ=US/Central (optional)
// - minutes of hour e.g. 30 (required)
// - hour of day e.g. 9 (required)
// - day of month (must be *)
// - month (must be *)
// - day of week (must be *)
//
// Example Usage:
//
//	local_sched, _ := cron.Weekly("59 23 * * *")
//	fmt.Println(sched.Next(time.Now().Format(time.RFC3339)))
//	// Output: 2022-04-04T23:59:00Z
//
//	us_sched, _ := cron.Weekly("CRON_TZ=US/Central 30 9 * * *")
//	fmt.Println(sched.Next(time.Now()).Format(time.RFC3339))
//	// Output: 2022-04-04T14:30:00Z
func Daily(raw string) (*Schedule, error) {
	if err := validateDailySpec(raw); err != nil {
		return nil, xerrors.Errorf("validate daily schedule: %w", err)
	}

	return parse(raw)
}

func parse(raw string) (*Schedule, error) {
	// If schedule does not specify a timezone, default to UTC. Otherwise,
	// the library will default to time.Local which we want to avoid.
	if !strings.HasPrefix(raw, "CRON_TZ=") {
		raw = "CRON_TZ=UTC " + raw
	}

	specSched, err := defaultParser.Parse(raw)
	if err != nil {
		return nil, xerrors.Errorf("parse schedule: %w", err)
	}

	schedule, ok := specSched.(*rbcron.SpecSchedule)
	if !ok {
		return nil, xerrors.Errorf("expected *cron.SpecSchedule but got %T", specSched)
	}

	if schedule.Location == time.Local {
		return nil, xerrors.Errorf("schedules scoped to time.Local are not supported")
	}

	// Strip the leading CRON_TZ prefix so we just store the cron string.
	// The timezone info is available in SpecSchedule.
	cronStr := raw
	if strings.HasPrefix(raw, "CRON_TZ=") {
		cronStr = strings.Join(strings.Fields(raw)[1:], " ")
	}

	cronSched := &Schedule{
		sched:   schedule,
		cronStr: cronStr,
	}
	return cronSched, nil
}

// Schedule represents a cron schedule.
// It's essentially a wrapper for robfig/cron/v3 that has additional
// convenience methods.
type Schedule struct {
	sched *rbcron.SpecSchedule
	// XXX: there isn't any nice way for robfig/cron to serialize
	cronStr string
}

// String serializes the schedule to its original human-friendly format.
// The leading CRON_TZ is maintained.
func (s Schedule) String() string {
	var sb strings.Builder
	_, _ = sb.WriteString("CRON_TZ=")
	_, _ = sb.WriteString(s.sched.Location.String())
	_, _ = sb.WriteString(" ")
	_, _ = sb.WriteString(s.cronStr)
	return sb.String()
}

// Location returns the IANA location for the schedule.
func (s Schedule) Location() *time.Location {
	return s.sched.Location
}

// Cron returns the cron spec for the schedule with the leading CRON_TZ
// stripped, if present.
func (s Schedule) Cron() string {
	return s.cronStr
}

// Next returns the next time in the schedule relative to t.
func (s Schedule) Next(t time.Time) time.Time {
	return s.sched.Next(t)
}

var (
	t0   = time.Date(1970, 1, 1, 1, 1, 1, 0, time.UTC)
	tMax = t0.Add(168 * time.Hour)
)

// Min returns the minimum duration of the schedule.
// This is calculated as follows:
//   - Let t(0) be a given point in time (1970-01-01T01:01:01Z00:00)
//   - Let t(max) be 168 hours after t(0).
//   - Let t(1) be the next scheduled time after t(0).
//   - Let t(n) be the next scheduled time after t(n-1).
//   - Then, the minimum duration of s d(min)
//     = min( t(n) - t(n-1) ∀ n ∈ N, t(n) < t(max) )
func (s Schedule) Min() time.Duration {
	durMin := tMax.Sub(t0)
	tPrev := s.Next(t0)
	tCurr := s.Next(tPrev)
	for {
		dur := tCurr.Sub(tPrev)
		if dur < durMin {
			durMin = dur
		}
		tPrev = tCurr
		tCurr = s.Next(tCurr)
		if tCurr.After(tMax) {
			break
		}
	}
	return durMin
}

// TimeParsed returns the parsed time.Time of the minute and hour fields. If the
// time cannot be represented in a valid time.Time, a zero time is returned.
func (s Schedule) TimeParsed() time.Time {
	minute := strings.Fields(s.cronStr)[0]
	hour := strings.Fields(s.cronStr)[1]
	maybeTime := fmt.Sprintf("%s:%s", hour, minute)
	t, err := time.ParseInLocation("15:4", maybeTime, s.sched.Location)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Time returns a humanized form of the minute and hour fields.
func (s Schedule) Time() string {
	minute := strings.Fields(s.cronStr)[0]
	hour := strings.Fields(s.cronStr)[1]
	maybeTime := fmt.Sprintf("%s:%s", hour, minute)
	t, err := time.ParseInLocation("15:4", maybeTime, s.sched.Location)
	if err != nil {
		// return the original cronspec for minute and hour, who knows what's in there!
		return fmt.Sprintf("cron(%s %s)", minute, hour)
	}
	return t.Format(time.Kitchen)
}

// DaysOfWeek returns a humanized form of the day-of-week field.
func (s Schedule) DaysOfWeek() string {
	dow := strings.Fields(s.cronStr)[4]
	if dow == "*" {
		return "daily"
	}
	for _, weekday := range []time.Weekday{
		time.Sunday,
		time.Monday,
		time.Tuesday,
		time.Wednesday,
		time.Thursday,
		time.Friday,
		time.Saturday,
	} {
		dow = strings.Replace(dow, fmt.Sprintf("%d", weekday), weekday.String()[:3], 1)
	}
	return dow
}

// validateWeeklySpec ensures that the day-of-month and month options of
// spec are both set to *
func validateWeeklySpec(spec string) error {
	parts := strings.Fields(spec)
	if len(parts) < 5 {
		return xerrors.Errorf("expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix")
	}
	if len(parts) == 6 {
		parts = parts[1:]
	}
	if parts[2] != "*" || parts[3] != "*" {
		return xerrors.Errorf("expected day-of-month and month to be *")
	}
	return nil
}

// validateDailySpec ensures that the day-of-month, month and day-of-week
// options of spec are all set to *
func validateDailySpec(spec string) error {
	parts := strings.Fields(spec)
	if len(parts) < 5 {
		return xerrors.Errorf("expected schedule to consist of 5 fields with an optional CRON_TZ=<timezone> prefix")
	}
	if len(parts) == 6 {
		parts = parts[1:]
	}
	if parts[2] != "*" || parts[3] != "*" || parts[4] != "*" {
		return xerrors.Errorf("expected day-of-month, month and day-of-week to be *")
	}
	return nil
}
