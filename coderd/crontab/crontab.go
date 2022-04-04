package crontab

import (
	"time"

	"github.com/robfig/cron/v3"
	"golang.org/x/xerrors"
)

const parserFormat = cron.Minute | cron.Hour | cron.Dow

var defaultParser = cron.NewParser(parserFormat)

// WeeklySchedule represents a weekly cron schedule serializable to and from a string.
//
// Example Usage:
//  local_sched, _ := cron.Parse("59 23 *")
//  fmt.Println(sched.Next(time.Now().Format(time.RFC3339)))
//  // Output: 2022-04-04T23:59:00Z
//  us_sched, _ := cron.Parse("CRON_TZ=US/Central 30 9 1-5")
//  fmt.Println(sched.Next(time.Now()).Format(time.RFC3339))
//  // Output: 2022-04-04T14:30:00Z
type WeeklySchedule interface {
	String() string
	Next(time.Time) time.Time
}

// cronSchedule is a thin wrapper for cron.SpecSchedule that implements Stringer.
type cronSchedule struct {
	sched *cron.SpecSchedule
	// XXX: there isn't any nice way for robfig/cron to serialize
	spec string
}

var _ WeeklySchedule = (*cronSchedule)(nil)

// String serializes the schedule to its original human-friendly format.
func (s cronSchedule) String() string {
	return s.spec
}

// Next returns the next time in the schedule relative to t.
func (s cronSchedule) Next(t time.Time) time.Time {
	return s.sched.Next(t)
}

func Parse(spec string) (*cronSchedule, error) {
	specSched, err := defaultParser.Parse(spec)
	if err != nil {
		return nil, xerrors.Errorf("parse schedule: %w", err)
	}

	schedule, ok := specSched.(*cron.SpecSchedule)
	if !ok {
		return nil, xerrors.Errorf("expected *cron.SpecSchedule but got %T", specSched)
	}

	cronSched := &cronSchedule{
		sched: schedule,
		spec:  spec,
	}
	return cronSched, nil

}
