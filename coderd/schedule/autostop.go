package schedule

import (
	"cmp"
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/tracing"
)

const (
	// autostopRequirementLeeway is the duration of time before a autostop
	// requirement where we skip the requirement and fall back to the next
	// scheduled stop. This avoids workspaces being stopped too soon.
	//
	// E.g. If the workspace is started shortly before the quiet hours, we
	//      will skip the autostop requirement and use the next scheduled
	//      stop time instead.
	autostopRequirementLeeway = 2 * time.Hour
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

	// WorkspaceBuildCompletedAt is the time when the workspace build was
	// completed.
	//
	// We always want to calculate startup leeway using the build completion
	// time to avoid forcing a workspace build's max_deadline to be pushed to
	// the next potential cron instance.
	//
	// E.g. if this function is called for an existing workspace build, which
	//      currently has a max_deadline within the next 2 hours (see leeway
	//      above), and the current time is passed in as the completion time,
	//      the max_deadline will be updated to be much later than expected.
	WorkspaceBuildCompletedAt time.Time
	Workspace                 database.WorkspaceTable

	// AutostopRequirementNotBefore is the earliest required stop when
	// recalculating an existing build. Build startup leeway still applies.
	//
	// E.g. if a build completes on Friday and an admin adds a weekly Sunday
	//      requirement the following Wednesday, pass the template save time
	//      to select the upcoming Sunday's quiet hours. A zero value starts
	//      from build completion plus startup leeway and selects the previous
	//      Sunday's quiet hours. That past deadline can cause an unexpected
	//      stop on Wednesday, outside quiet hours.
	AutostopRequirementNotBefore time.Time
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
// when activity is detected and more than 5% of the TTL has passed to save
// database queries, see the ActivityBumpWorkspace query).
//
// MaxDeadline is the maximum value for deadline. The deadline cannot be bumped
// past this value, so it denotes the absolute deadline that the workspace build
// must be stopped by. MaxDeadline is calculated using the template's "autostop
// requirement" settings and the user's "quiet hours" settings to pick a time
// outside of working hours.
//
// Note that the deadline is checked at the database level:
//
//	(deadline IS NOT zero AND deadline <= max_deadline) UNLESS max_deadline is zero.
//
// Deadline is intended as a cost saving measure, not as a hard policy. It is
// derived from either the workspace's TTL or the template's TTL, depending on
// the template's policy, to ensure workspaces are stopped when they are idle.
//
// MaxDeadline is intended as a compliance policy. It is derived from the
// template's autostop requirement to cap workspace uptime and effectively force
// people to update often.
//
// Note that only the build's CURRENT deadline property influences automation in
// the autobuild package. As stated above, the MaxDeadline property is only used
// to cap the value of a build's deadline.
func CalculateAutostop(ctx context.Context, params CalculateAutostopParams) (AutostopTime, error) {
	ctx, span := tracing.StartSpan(ctx,
		trace.WithAttributes(attribute.String("coder.workspace_id", params.Workspace.ID.String())),
		trace.WithAttributes(attribute.String("coder.template_id", params.Workspace.TemplateID.String())),
	)
	defer span.End()

	var (
		db               = params.Database
		workspace        = params.Workspace
		buildCompletedAt = params.WorkspaceBuildCompletedAt

		autostop AutostopTime
	)

	templateSchedule, err := params.TemplateScheduleStore.Get(ctx, db, workspace.TemplateID)
	if err != nil {
		return autostop, xerrors.Errorf("get template schedule options: %w", err)
	}

	ttl := workspaceTTL(workspace, templateSchedule)
	if ttl > 0 {
		// Only apply non-zero TTLs.
		autostop.Deadline = buildCompletedAt.Add(ttl)
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
			nextAutostart, ok := NextAutostart(params.WorkspaceBuildCompletedAt, params.WorkspaceAutostart, templateSchedule)
			if ok && autostop.Deadline.After(nextAutostart) {
				autostop.Deadline = nextAutostart.Add(ttl)
			}
		}
	}

	autostop.MaxDeadline, err = calculateMaxDeadline(ctx, params, templateSchedule.AutostopRequirement)
	if err != nil {
		return autostop, err
	}

	// If the workspace doesn't have a deadline or the max deadline is sooner
	// than the workspace deadline, use the max deadline as the actual deadline.
	if !autostop.MaxDeadline.IsZero() && (autostop.Deadline.IsZero() || autostop.MaxDeadline.Before(autostop.Deadline)) {
		autostop.Deadline = autostop.MaxDeadline
	}

	if (!autostop.Deadline.IsZero() && autostop.Deadline.Before(buildCompletedAt)) || (!autostop.MaxDeadline.IsZero() && autostop.MaxDeadline.Before(buildCompletedAt)) {
		// Something went wrong with the deadline calculation, so we should
		// bail.
		return autostop, xerrors.Errorf("deadline calculation error, computed deadline or max deadline is in the past for workspace build: deadline=%q maxDeadline=%q now=%q", autostop.Deadline, autostop.MaxDeadline, buildCompletedAt)
	}

	return autostop, nil
}

func calculateMaxDeadline(ctx context.Context, params CalculateAutostopParams, requirement TemplateAutostopRequirement) (time.Time, error) {
	if requirement.DaysOfWeek == 0 {
		return time.Time{}, nil
	}

	// Get the user's quiet hours schedule (this will return the default if
	// the user has not set their own schedule).
	userQuietHoursSchedule, err := params.UserQuietHoursScheduleStore.Get(ctx, params.Database, params.Workspace.OwnerID)
	if err != nil {
		return time.Time{}, xerrors.Errorf("get user quiet hours schedule options: %w", err)
	}

	// If the schedule is nil, that means the deployment isn't entitled to
	// use quiet hours. In this case, do not set a max deadline on the
	// workspace.
	if userQuietHoursSchedule.Schedule == nil {
		return time.Time{}, nil
	}

	loc := userQuietHoursSchedule.Schedule.Location()
	buildCompletedAtInLoc := params.WorkspaceBuildCompletedAt.In(loc)
	// Add the leeway here so we avoid checking today's quiet hours if
	// the workspace was started shortly before midnight.
	notBefore := buildCompletedAtInLoc.Add(autostopRequirementLeeway)
	if params.AutostopRequirementNotBefore.After(notBefore) {
		notBefore = params.AutostopRequirementNotBefore.In(loc)
	}
	// A bitmap with only the unused bit set would never match a date.
	if requirement.DaysOfWeek&0b01111111 == 0 {
		return time.Time{}, xerrors.New("autostop requirement has no valid weekdays")
	}
	requirementDays := requirement.DaysMap()

	// A rollback across midnight can bring back the previous date after
	// the cutoff. Include it if a quiet-hours start still qualifies.
	searchStart := notBefore
	previousDate := notBefore.AddDate(0, 0, -1)
	if requirementDays[previousDate.Weekday()] &&
		(requirement.Weeks <= 1 || !previousDate.Before(TemplateAutostopRequirementEpoch(loc))) {
		if quietStart, _ := userQuietHoursSchedule.Schedule.FirstOnDate(previousDate, notBefore); !quietStart.IsZero() {
			searchStart = previousDate
		}
	}

	var prevStopDay time.Time
	useNextQuietHours := false
	for stopDay := searchStart; ; stopDay = stopDay.AddDate(0, 0, 1) {
		// If a clock change returns the cursor to an earlier date,
		// recover by finding the next quiet-hours start on a later
		// date (if a stop is required, a late one is better than
		// none).
		for !prevStopDay.IsZero() && cmp.Or(
			cmp.Compare(stopDay.Year(), prevStopDay.Year()),
			cmp.Compare(stopDay.YearDay(), prevStopDay.YearDay()),
		) <= 0 {
			// The daily cron search advances through elapsed hours,
			// so it can cross a skipped date.
			next := userQuietHoursSchedule.Schedule.Next(stopDay)
			if !next.After(stopDay) {
				return time.Time{}, xerrors.Errorf("could not find next quiet hours after %q", stopDay)
			}
			stopDay = next
		}
		// Recovery can move the cursor into a different week. Apply
		// the cadence to the recovered date before checking its weekday.
		if !useNextQuietHours && requirement.Weeks > 1 {
			stopDay, err = GetNextApplicableMondayOfNWeeks(stopDay, requirement.Weeks)
			if err != nil {
				return time.Time{}, xerrors.Errorf("determine start of stop week: %w", err)
			}
		}
		// Advance calendar days from noon, since a timezone transition
		// such as a daylight saving time change is more likely to skip
		// midnight than noon.
		year, month, day := stopDay.Date()
		noon := time.Date(year, month, day, 12, 0, 0, 0, loc)
		// Keep the original cursor if a skipped noon resolves to a
		// different local date.
		if noon.Year() == year && noon.Month() == month && noon.Day() == day {
			stopDay = noon
		}
		if !prevStopDay.IsZero() && !stopDay.After(prevStopDay) {
			return time.Time{}, xerrors.Errorf("required stop date search did not advance past %q", prevStopDay)
		}
		prevStopDay = stopDay
		if !useNextQuietHours && !requirementDays[stopDay.Weekday()] {
			continue
		}

		quietStart, skippedQuietHours := userQuietHoursSchedule.Schedule.FirstOnDate(stopDay, notBefore)
		if !quietStart.IsZero() {
			return quietStart, nil
		}

		// Use the next quiet-hours start on any date when a clock jump
		// at or after the cutoff skips the selected date's quiet hours.
		if skippedQuietHours {
			useNextQuietHours = true
		}
	}
}

// workspaceTTL returns the TTL to use for a workspace.
//
// If the template forbids custom workspace TTLs, then we always use the
// template's configured TTL (or 0 if the template has no TTL configured).
func workspaceTTL(workspace database.WorkspaceTable, templateSchedule TemplateScheduleOptions) time.Duration {
	// If the template forbids custom workspace TTLs, then we always use the
	// template's configured TTL (or 0 if the template has no TTL configured).
	if !templateSchedule.UserAutostopEnabled {
		// This is intentionally a nested if statement because of the else if.
		if templateSchedule.DefaultTTL > 0 {
			return templateSchedule.DefaultTTL
		}
		return 0
	}
	if workspace.Ttl.Valid {
		return time.Duration(workspace.Ttl.Int64)
	}
	return 0
}

// WeeksSinceEpoch gets the weeks since the epoch for a given time. This is a
// 0-indexed number of weeks since the epoch (Monday).
//
// The timezone embedded in the time object is used to determine the epoch.
func WeeksSinceEpoch(now time.Time) (int64, error) {
	epoch := TemplateAutostopRequirementEpoch(now.Location())
	if now.Before(epoch) {
		return 0, xerrors.New("coder server system clock is incorrect, cannot calculate template autostop requirement")
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
			return 0, xerrors.New("overflow calculating weeks since epoch")
		}
		endOfThisYear := startOfNextYear.AddDate(0, 0, -1)
		if endOfThisYear.Year() != i {
			return 0, xerrors.New("overflow calculating weeks since epoch")
		}

		days += endOfThisYear.YearDay()
	}
	// Add this year's days.
	days += now.YearDay()

	// Ensure that the number of days is positive.
	if days < 0 {
		return 0, xerrors.New("overflow calculating weeks since epoch")
	}

	// Divide by 7 to get the number of weeks.
	weeksSinceEpoch := int64(days / 7)
	return weeksSinceEpoch, nil
}

// GetMondayOfWeek gets the Monday (0:00) of the n-th week since epoch.
func GetMondayOfWeek(loc *time.Location, n int64) (time.Time, error) {
	if n < 0 {
		return time.Time{}, xerrors.New("weeks since epoch must be positive")
	}
	epoch := TemplateAutostopRequirementEpoch(loc)
	monday := epoch.AddDate(0, 0, int(n*7))

	y, m, d := monday.Date()
	monday = time.Date(y, m, d, 0, 0, 0, 0, loc)
	if monday.Weekday() != time.Monday {
		// This condition should never be hit, but we have a check for it just
		// in case.
		return time.Time{}, xerrors.Errorf("calculated incorrect Monday for week %v since epoch (actual weekday %q)", n, monday.Weekday())
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
		return time.Time{}, xerrors.Errorf("get current week number: %w", err)
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

	return time.Time{}, xerrors.Errorf("get next applicable Monday of %v weeks: %w", n, lastErr)
}
