// package schedule provides utilities for parsing and deserializing
// cron-style expressions.
package schedule

import (
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/xerrors"
)

// For the purposes of this library, we only need minute, hour, and
// day-of-week.
const parserFormatWeekly = cron.Minute | cron.Hour | cron.Dow

var defaultParser = cron.NewParser(parserFormatWeekly)

// Weekly parses a Schedule from spec scoped to a recurring weekly event.
// Spec consists of the following space-delimited fields, in the following order:
// - timezone e.g. CRON_TZ=US/Central (optional)
// - minutes of hour e.g. 30 (required)
// - hour of day e.g. 9 (required)
// - day of week e.g. 1 (required)
//
// Example Usage:
//  local_sched, _ := schedule.Weekly("59 23 *")
//  fmt.Println(sched.Next(time.Now().Format(time.RFC3339)))
//  // Output: 2022-04-04T23:59:00Z
//  us_sched, _ := schedule.Weekly("CRON_TZ=US/Central 30 9 1-5")
//  fmt.Println(sched.Next(time.Now()).Format(time.RFC3339))
//  // Output: 2022-04-04T14:30:00Z
func Weekly(spec string) (*Schedule, error) {
	specSched, err := defaultParser.Parse(spec)
	if err != nil {
		return nil, xerrors.Errorf("parse schedule: %w", err)
	}

	schedule, ok := specSched.(*cron.SpecSchedule)
	if !ok {
		return nil, xerrors.Errorf("expected *cron.SpecSchedule but got %T", specSched)
	}

	cronSched := &Schedule{
		sched: schedule,
		spec:  spec,
	}
	return cronSched, nil
}

// Schedule represents a cron schedule.
// It's essentially a thin wrapper for robfig/cron/v3 that implements Stringer.
type Schedule struct {
	sched *cron.SpecSchedule
	// XXX: there isn't any nice way for robfig/cron to serialize
	spec string
}

// String serializes the schedule to its original human-friendly format.
func (s Schedule) String() string {
	return s.spec
}

// Next returns the next time in the schedule relative to t.
func (s Schedule) Next(t time.Time) time.Time {
	return s.sched.Next(t)
}
