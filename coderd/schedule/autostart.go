package schedule

import (
	"time"

	"github.com/coder/coder/v2/coderd/schedule/cron"
)

// NextAutostart takes the workspace and template schedule and returns the next autostart schedule
// after "at". The boolean returned is if the autostart should be allowed to start based on the template
// schedule.
func NextAutostart(at time.Time, wsSchedule string, templateSchedule TemplateScheduleOptions) (time.Time, bool) {
	sched, err := cron.Weekly(wsSchedule)
	if err != nil {
		return time.Time{}, false
	}

	// Round down to the nearest minute, as this is the finest granularity cron supports.
	// Truncate is probably not necessary here, but doing it anyway to be sure.
	nextTransition := sched.Next(at).Truncate(time.Minute)

	// The nextTransition is when the auto start should kick off. If it lands on a
	// forbidden day, do not allow the auto start. We use the time location of the
	// schedule to determine the weekday. So if "Saturday" is disallowed, the
	// definition of "Saturday" depends on the location of the schedule.
	zonedTransition := nextTransition.In(sched.Location())
	allowed := templateSchedule.AutostartRequirement.DaysMap()[zonedTransition.Weekday()]

	return zonedTransition, allowed
}
