package schedule

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/database"
)

const (
	// restartRequirementLeeway is the duration of time before a restart
	// requirement where we skip the requirement and fall back to the next
	// scheduled restart. This avoids workspaces being restarted too soon.
	restartRequirementLeeway = 1 * time.Hour

	// restartRequirementBuffer is the duration of time we subtract from the
	// time when calculating the next scheduled restart time. This avoids issues
	// where autostart happens on the hour and the scheduled quiet hours are
	// also on the hour.
	restartRequirementBuffer = -15 * time.Minute
)

type CalculateAutostopParams struct {
	Database                    database.Store
	TemplateScheduleStore       TemplateScheduleStore
	UserQuietHoursScheduleStore UserQuietHoursScheduleStore

	Now       time.Time
	Workspace database.Workspace
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
// must be stopped by. MaxDeadline is calculated using the template's "restart
// requirement" settings and the user's "quiet hours" settings to pick a time
// outside of working hours.
//
// Deadline is a cost saving measure, while max deadline is a
// compliance/updating measure.
func CalculateAutostop(ctx context.Context, params CalculateAutostopParams) (AutostopTime, error) {
	var (
		db        = params.Database
		workspace = params.Workspace
		now       = params.Now

		autostop AutostopTime
	)

	if workspace.Ttl.Valid {
		// When the workspace is made it copies the template's TTL, and the user
		// can unset it to disable it (unless the template has
		// UserAutoStopEnabled set to false, see below).
		autostop.Deadline = now.Add(time.Duration(workspace.Ttl.Int64))
	}

	templateSchedule, err := params.TemplateScheduleStore.GetTemplateScheduleOptions(ctx, db, workspace.TemplateID)
	if err != nil {
		return autostop, xerrors.Errorf("get template schedule options: %w", err)
	}
	if !templateSchedule.UserAutostopEnabled {
		// The user is not permitted to set their own TTL, so use the template
		// default.
		autostop.Deadline = time.Time{}
		if templateSchedule.DefaultTTL > 0 {
			autostop.Deadline = now.Add(templateSchedule.DefaultTTL)
		}
	}

	// Use the old algorithm for calculating max_deadline if the instance isn't
	// configured or entitled to use the new feature flag yet.
	// TODO(@dean): remove this once the feature flag is enabled for all
	if !templateSchedule.UseRestartRequirement && templateSchedule.MaxTTL > 0 {
		autostop.MaxDeadline = now.Add(templateSchedule.MaxTTL)
	}

	// TODO(@dean): remove extra conditional
	if templateSchedule.UseRestartRequirement && templateSchedule.RestartRequirement.DaysOfWeek != 0 {
		// The template has a restart requirement, so determine the max deadline
		// of this workspace build.

		// First, get the user's quiet hours schedule (this will return the
		// default if the user has not set their own schedule).
		userQuietHoursSchedule, err := params.UserQuietHoursScheduleStore.GetUserQuietHoursScheduleOptions(ctx, db, workspace.OwnerID)
		if err != nil {
			return autostop, xerrors.Errorf("get user quiet hours schedule options: %w", err)
		}

		// If the schedule is nil, that means the deployment isn't entitled to
		// use quiet hours or the default schedule has not been set. In this
		// case, do not set a max deadline on the workspace.
		if userQuietHoursSchedule.Schedule != nil {
			loc := userQuietHoursSchedule.Schedule.Location()
			now := now.In(loc)
			// Add the leeway here so we avoid checking today's quiet hours if
			// the workspace was started <1h before midnight.
			startOfStopDay := truncateMidnight(now.Add(restartRequirementLeeway))

			// If the template schedule wants to only restart on n-th weeks then
			// change the startOfDay to be the Monday of the next applicable
			// week.
			if templateSchedule.RestartRequirement.Weeks > 1 {
				epoch := TemplateRestartRequirementEpoch(loc)
				if startOfStopDay.Before(epoch) {
					return autostop, xerrors.New("coder server system clock is incorrect, cannot calculate template restart requirement")
				}
				since := startOfStopDay.Sub(epoch)
				weeksSinceEpoch := int64(since.Hours() / (24 * 7))
				requiredWeeks := templateSchedule.RestartRequirement.Weeks
				weeksRemainder := weeksSinceEpoch % requiredWeeks
				if weeksRemainder != 0 {
					// Add (requiredWeeks - weeksSince) * 7 days to the current
					// startOfStopDay, then truncate to Monday midnight.
					//
					// This sets startOfStopDay to Monday at midnight of the
					// next applicable week.
					y, mo, d := startOfStopDay.Date()
					d += int(requiredWeeks-weeksRemainder) * 7
					startOfStopDay = time.Date(y, mo, d, 0, 0, 0, 0, loc)
					startOfStopDay = truncateMondayMidnight(startOfStopDay)
				}
			}

			// Determine if we should skip the first day because the schedule is
			// too near or has already passed.
			//
			// Allow an hour of leeway (i.e. any workspaces started within an
			// hour of the scheduled stop time will always bounce to the next
			// stop window).
			checkSchedule := userQuietHoursSchedule.Schedule.Next(startOfStopDay.Add(restartRequirementBuffer))
			if checkSchedule.Before(now.Add(restartRequirementLeeway)) {
				// Set the first stop day we try to tomorrow because today's
				// schedule is too close to now or has already passed.
				startOfStopDay = nextDayMidnight(startOfStopDay)
			}

			// Iterate from 0 to 7, check if the current startOfDay is in the
			// restart requirement. If it isn't then add a day and try again.
			requirementDays := templateSchedule.RestartRequirement.DaysMap()
			for i := 0; i < len(DaysOfWeek)+1; i++ {
				if i == len(DaysOfWeek) {
					// We've wrapped, so somehow we couldn't find a day in the
					// restart requirement in the next week.
					//
					// This shouldn't be able to happen, as we've already
					// checked that there is a day in the restart requirement
					// above with the
					// `if templateSchedule.RestartRequirement.DaysOfWeek != 0`
					// check.
					//
					// The eighth bit shouldn't be set, as we validate the
					// bitmap in the enterprise TemplateScheduleStore.
					return autostop, xerrors.New("could not find suitable day for template restart requirement in the next 7 days")
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
				checkTime = checkTime.Add(restartRequirementBuffer)
			}

			// Get the next occurrence of the restart schedule.
			autostop.MaxDeadline = userQuietHoursSchedule.Schedule.Next(checkTime)
			if autostop.MaxDeadline.IsZero() {
				return autostop, xerrors.New("could not find next occurrence of template restart requirement in user quiet hours schedule")
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
		return autostop, xerrors.Errorf("deadline calculation error, computed deadline or max deadline is in the past for workspace build: deadline=%q maxDeadline=%q now=%q", autostop.Deadline, autostop.MaxDeadline, now)
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

// truncateMondayMidnight truncates a time to the previous Monday at midnight in
// the time object's timezone.
func truncateMondayMidnight(t time.Time) time.Time {
	// time.Date will correctly normalize the date if it's past the end of the
	// month. E.g. October 32nd will be November 1st.
	yy, mm, dd := t.Date()
	dd -= int(t.Weekday() - 1)
	t = time.Date(yy, mm, dd, 0, 0, 0, 0, t.Location())
	return truncateMidnight(t)
}
