// package crontab provides utilities for parsing and deserializing
// cron-style expressions.
package crontab

import (
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/xerrors"
)

// For the purposes of this library, we only need minute, hour, and
//day-of-week.
const parserFormat = cron.Minute | cron.Hour | cron.Dow

var defaultParser = cron.NewParser(parserFormat)

// Parse parses a WeeklySchedule from spec.
//
// Example Usage:
//  local_sched, _ := cron.Parse("59 23 *")
//  fmt.Println(sched.Next(time.Now().Format(time.RFC3339)))
//  // Output: 2022-04-04T23:59:00Z
//  us_sched, _ := cron.Parse("CRON_TZ=US/Central 30 9 1-5")
//  fmt.Println(sched.Next(time.Now()).Format(time.RFC3339))
//  // Output: 2022-04-04T14:30:00Z
func Parse(spec string) (*WeeklySchedule, error) {
	specSched, err := defaultParser.Parse(spec)
	if err != nil {
		return nil, xerrors.Errorf("parse schedule: %w", err)
	}

	schedule, ok := specSched.(*cron.SpecSchedule)
	if !ok {
		return nil, xerrors.Errorf("expected *cron.SpecSchedule but got %T", specSched)
	}

	cronSched := &WeeklySchedule{
		sched: schedule,
		spec:  spec,
	}
	return cronSched, nil
}

// WeeklySchedule represents a weekly cron schedule.
// It's essentially a thin wrapper for robfig/cron/v3 that implements Stringer.
type WeeklySchedule struct {
	sched *cron.SpecSchedule
	// XXX: there isn't any nice way for robfig/cron to serialize
	spec string
}

// String serializes the schedule to its original human-friendly format.
func (s WeeklySchedule) String() string {
	return s.spec
}

// Next returns the next time in the schedule relative to t.
func (s WeeklySchedule) Next(t time.Time) time.Time {
	return s.sched.Next(t)
}
