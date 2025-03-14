package schedule

import (
	"fmt"
	"errors"
	"context"

	"time"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"github.com/coder/coder/v2/coderd/database"

	"github.com/coder/coder/v2/coderd/tracing"
)
const (
	// autostopRequirementLeeway is the duration of time before a autostop

	// requirement where we skip the requirement and fall back to the next
	// scheduled stop. This avoids workspaces being stopped too soon.
	//
	// E.g. If the workspace is started within two hours of the quiet hours, we
	//      will skip the autostop requirement and use the next scheduled
	//      stop time instead.
	autostopRequirementLeeway = 2 * time.Hour
	// autostopRequirementBuffer is the duration of time we subtract from the
	// time when calculating the next scheduled stop time. This avoids issues
	// where autostart happens on the hour and the scheduled quiet hours are

	// also on the hour.
	//
	// E.g. If the workspace is started at 12am (perhaps due to scheduled
	//      autostart) and the quiet hours is also 12am, the workspace will skip
	//      the day it's supposed to stop and use the next day instead. This is
	//      because getting the next cron schedule time will never include the
	//      time fed to the calculation (i.e. it's not inclusive). This happens
	//      because we always check for the next cron time by rounding down to
	//      midnight.
	//
	//      This resolves that problem by subtracting 15 minutes from midnight
	//      when we check the next cron time.
	autostopRequirementBuffer = -15 * time.Minute
)
type CalculateAutostopParams struct {
	Database                    database.Store
	TemplateScheduleStore       TemplateScheduleStore
	UserQuietHoursScheduleStore UserQuietHoursScheduleStore

	// WorkspaceAutostart can be the empty string if no workspace autostart
	// is configured.
	// If configured, this is expected to be a cron weekly event parsable
	// by autobuild.NextAutostart
	WorkspaceAutostart string
	Now       time.Time
	Workspace database.WorkspaceTable
}
type AutostopTime struct {
	// Deadline is the time when the workspace will be stopped. The value can be

	// bumped by user activity or manually by the user via the UI.
	Deadline time.Time
	// MaxDeadline is the maximum value for deadline.
	MaxDeadline time.Time

}
// CalculateAutostop calculates the deadline and max_deadline for a workspace
// build.
//
// Deadline is the time when the workspace will be stopped, as long as it
// doesn't see any new activity (such as SSH, app requests, etc.). When activity
// is detected the deadline is bumped by the workspace's TTL (this only happens
// when activity is detected and more than 20% of the TTL has passed to save

// database queries).
//
// MaxDeadline is the maximum value for deadline. The deadline cannot be bumped
// past this value, so it denotes the absolute deadline that the workspace build
// must be stopped by. MaxDeadline is calculated using the template's "autostop
// requirement" settings and the user's "quiet hours" settings to pick a time
// outside of working hours.
//
// Deadline is a cost saving measure, while max deadline is a
// compliance/updating measure.
func CalculateAutostop(ctx context.Context, params CalculateAutostopParams) (AutostopTime, error) {
	ctx, span := tracing.StartSpan(ctx,
		trace.WithAttributes(attribute.String("coder.workspace_id", params.Workspace.ID.String())),
		trace.WithAttributes(attribute.String("coder.template_id", params.Workspace.TemplateID.String())),
	)
	defer span.End()
	defer span.End()
	var (
		db        = params.Database
		workspace = params.Workspace
		now       = params.Now
		autostop AutostopTime
	)
	var ttl time.Duration
	if workspace.Ttl.Valid {

		// When the workspace is made it copies the template's TTL, and the user
		// can unset it to disable it (unless the template has
		// UserAutoStopEnabled set to false, see below).
		ttl = time.Duration(workspace.Ttl.Int64)
	}

	if workspace.Ttl.Valid {
		// When the workspace is made it copies the template's TTL, and the user
		// can unset it to disable it (unless the template has

		// UserAutoStopEnabled set to false, see below).
		autostop.Deadline = now.Add(time.Duration(workspace.Ttl.Int64))
	}
	templateSchedule, err := params.TemplateScheduleStore.Get(ctx, db, workspace.TemplateID)
	if err != nil {
		return autostop, fmt.Errorf("get template schedule options: %w", err)
	}
	if !templateSchedule.UserAutostopEnabled {

		// The user is not permitted to set their own TTL, so use the template
		// default.
		ttl = 0
		if templateSchedule.DefaultTTL > 0 {
			ttl = templateSchedule.DefaultTTL
		}
	}

	if ttl > 0 {
		// Only apply non-zero TTLs.
		autostop.Deadline = now.Add(ttl)
		if params.WorkspaceAutostart != "" {
			// If the deadline passes the next autostart, we need to extend the deadline to
			// autostart + deadline. ActivityBumpWorkspace already covers this case
			// when extending the deadline.
			//
			// Situation this is solving.
			// 1. User has workspace with auto-start at 9:00am, 12 hour auto-stop.
			// 2. Coder stops workspace at 9pm
			// 3. User starts workspace at 9:45pm.
			//	- The initial deadline is calculated to be 9:45am

			//	- This crosses the autostart deadline, so the deadline is extended to 9pm
			nextAutostart, ok := NextAutostart(params.Now, params.WorkspaceAutostart, templateSchedule)
			if ok && autostop.Deadline.After(nextAutostart) {
				autostop.Deadline = nextAutostart.Add(ttl)
			}
		}
	}
	// Otherwise, use the autostop_requirement algorithm.
	if templateSchedule.AutostopRequirement.DaysOfWeek != 0 {
		// The template has a autostop requirement, so determine the max deadline
		// of this workspace build.
		// First, get the user's quiet hours schedule (this will return the
		// default if the user has not set their own schedule).
		userQuietHoursSchedule, err := params.UserQuietHoursScheduleStore.Get(ctx, db, workspace.OwnerID)
		if err != nil {
			return autostop, fmt.Errorf("get user quiet hours schedule options: %w", err)
		}
		// If the schedule is nil, that means the deployment isn't entitled to
		// use quiet hours. In this case, do not set a max deadline on the
		// workspace.
		if userQuietHoursSchedule.Schedule != nil {

			loc := userQuietHoursSchedule.Schedule.Location()
			now := now.In(loc)
			// Add the leeway here so we avoid checking today's quiet hours if
			// the workspace was started <1h before midnight.
			startOfStopDay := truncateMidnight(now.Add(autostopRequirementLeeway))

			// If the template schedule wants to only autostop on n-th weeks
			// then change the startOfDay to be the Monday of the next
			// applicable week.
			if templateSchedule.AutostopRequirement.Weeks > 1 {
				startOfStopDay, err = GetNextApplicableMondayOfNWeeks(startOfStopDay, templateSchedule.AutostopRequirement.Weeks)
				if err != nil {
					return autostop, fmt.Errorf("determine start of stop week: %w", err)

				}
			}
			// Determine if we should skip the first day because the schedule is
			// too near or has already passed.
			//
			// Allow an hour of leeway (i.e. any workspaces started within an
			// hour of the scheduled stop time will always bounce to the next
			// stop window).
			checkSchedule := userQuietHoursSchedule.Schedule.Next(startOfStopDay.Add(autostopRequirementBuffer))
			if checkSchedule.Before(now.Add(autostopRequirementLeeway)) {

				// Set the first stop day we try to tomorrow because today's
				// schedule is too close to now or has already passed.
				startOfStopDay = nextDayMidnight(startOfStopDay)
			}
			// Iterate from 0 to 7, check if the current startOfDay is in the
			// autostop requirement. If it isn't then add a day and try again.
			requirementDays := templateSchedule.AutostopRequirement.DaysMap()
			for i := 0; i < len(DaysOfWeek)+1; i++ {
				if i == len(DaysOfWeek) {
					// We've wrapped, so somehow we couldn't find a day in the

					// autostop requirement in the next week.
					//
					// This shouldn't be able to happen, as we've already
					// checked that there is a day in the autostop requirement
					// above with the
					// `if templateSchedule.AutoStopRequirement.DaysOfWeek != 0`
					// check.
					//
					// The eighth bit shouldn't be set, as we validate the
					// bitmap in the enterprise TemplateScheduleStore.
					return autostop, errors.New("could not find suitable day for template autostop requirement in the next 7 days")
				}
				if requirementDays[startOfStopDay.Weekday()] {

					break
				}
				startOfStopDay = nextDayMidnight(startOfStopDay)
			}
			// If the startOfDay is within an hour of now, then we add an hour.
			checkTime := startOfStopDay
			if checkTime.Before(now.Add(time.Hour)) {
				checkTime = now.Add(time.Hour)
			} else {
				// If it's not within an hour of now, subtract 15 minutes to
				// give a little leeway. This prevents skipped stop events
				// because autostart perfectly lines up with autostop.
				checkTime = checkTime.Add(autostopRequirementBuffer)
			}
			// Get the next occurrence of the schedule.
			autostop.MaxDeadline = userQuietHoursSchedule.Schedule.Next(checkTime)
			if autostop.MaxDeadline.IsZero() {
				return autostop, fmt.Errorf("could not find next occurrence of template autostop requirement in user quiet hours schedule, checked from time %q", checkTime)
			}
		}
	}
	// If the workspace doesn't have a deadline or the max deadline is sooner
	// than the workspace deadline, use the max deadline as the actual deadline.
	if !autostop.MaxDeadline.IsZero() && (autostop.Deadline.IsZero() || autostop.MaxDeadline.Before(autostop.Deadline)) {

		autostop.Deadline = autostop.MaxDeadline
	}
	if (!autostop.Deadline.IsZero() && autostop.Deadline.Before(now)) || (!autostop.MaxDeadline.IsZero() && autostop.MaxDeadline.Before(now)) {
		// Something went wrong with the deadline calculation, so we should
		// bail.
		return autostop, fmt.Errorf("deadline calculation error, computed deadline or max deadline is in the past for workspace build: deadline=%q maxDeadline=%q now=%q", autostop.Deadline, autostop.MaxDeadline, now)
	}
	return autostop, nil
}
// truncateMidnight truncates a time to midnight in the time object's timezone.
// t.Truncate(24 * time.Hour) truncates based on the internal time and doesn't

// factor daylight savings properly.
//
// See: https://github.com/golang/go/issues/10894
func truncateMidnight(t time.Time) time.Time {
	yy, mm, dd := t.Date()
	return time.Date(yy, mm, dd, 0, 0, 0, 0, t.Location())
}
// nextDayMidnight returns the next midnight in the time object's timezone.

func nextDayMidnight(t time.Time) time.Time {
	yy, mm, dd := t.Date()
	// time.Date will correctly normalize the date if it's past the end of the
	// month. E.g. October 32nd will be November 1st.
	dd++
	return time.Date(yy, mm, dd, 0, 0, 0, 0, t.Location())

}
// WeeksSinceEpoch gets the weeks since the epoch for a given time. This is a
// 0-indexed number of weeks since the epoch (Monday).
//
// The timezone embedded in the time object is used to determine the epoch.
func WeeksSinceEpoch(now time.Time) (int64, error) {

	epoch := TemplateAutostopRequirementEpoch(now.Location())
	if now.Before(epoch) {
		return 0, errors.New("coder server system clock is incorrect, cannot calculate template autostop requirement")

	}
	// This calculation needs to be done using YearDay, as dividing by the
	// amount of hours is impacted by daylight savings. Even though daylight
	// savings is usually only an hour difference, this calculation is used to
	// get the current week number and could result in an entire week getting
	// skipped if the calculation is off by an hour.
	//
	// Old naive algorithm: weeksSinceEpoch := int64(since.Hours() / (24 * 7))
	// Get days since epoch. Start with a negative number of days, as we want to
	// subtract the YearDay() of the epoch itself.

	days := -epoch.YearDay()
	for i := epoch.Year(); i < now.Year(); i++ {
		startOfNextYear := time.Date(i+1, 1, 1, 0, 0, 0, 0, now.Location())
		if startOfNextYear.Year() != i+1 {
			return 0, errors.New("overflow calculating weeks since epoch")
		}
		endOfThisYear := startOfNextYear.AddDate(0, 0, -1)
		if endOfThisYear.Year() != i {
			return 0, errors.New("overflow calculating weeks since epoch")

		}
		days += endOfThisYear.YearDay()
	}
	// Add this year's days.
	days += now.YearDay()
	// Ensure that the number of days is positive.
	if days < 0 {
		return 0, errors.New("overflow calculating weeks since epoch")
	}
	// Divide by 7 to get the number of weeks.

	weeksSinceEpoch := int64(days / 7)
	return weeksSinceEpoch, nil
}
// GetMondayOfWeek gets the Monday (0:00) of the n-th week since epoch.
func GetMondayOfWeek(loc *time.Location, n int64) (time.Time, error) {
	if n < 0 {
		return time.Time{}, errors.New("weeks since epoch must be positive")
	}

	epoch := TemplateAutostopRequirementEpoch(loc)
	monday := epoch.AddDate(0, 0, int(n*7))
	y, m, d := monday.Date()
	monday = time.Date(y, m, d, 0, 0, 0, 0, loc)
	if monday.Weekday() != time.Monday {
		// This condition should never be hit, but we have a check for it just
		// in case.
		return time.Time{}, fmt.Errorf("calculated incorrect Monday for week %v since epoch (actual weekday %q)", n, monday.Weekday())
	}
	return monday, nil
}
// GetNextApplicableMondayOfNWeeks gets the next Monday (0:00) of the next week
// divisible by n since epoch. If the next applicable week is invalid for any

// reason, the week after will be used instead (up to 2 attempts).
//
// If the current week is divisible by n, then the provided time is returned as
// is.
//

// The timezone embedded in the time object is used to determine the epoch.
func GetNextApplicableMondayOfNWeeks(now time.Time, n int64) (time.Time, error) {
	// Get the current week number.
	weeksSinceEpoch, err := WeeksSinceEpoch(now)
	if err != nil {

		return time.Time{}, fmt.Errorf("get current week number: %w", err)
	}
	// Get the next week divisible by n.
	remainder := weeksSinceEpoch % n
	week := weeksSinceEpoch + (n - remainder)

	if remainder == 0 {
		return now, nil
	}
	// Loop until we find a week that doesn't fail. This should never loop, but
	// we account for failures just in case.
	var lastErr error
	for i := int64(0); i < 3; i++ {
		monday, err := GetMondayOfWeek(now.Location(), week+i)

		if err != nil {
			lastErr = err
			continue
		}
		return monday, nil
	}
	return time.Time{}, fmt.Errorf("get next applicable Monday of %v weeks: %w", n, lastErr)
}
